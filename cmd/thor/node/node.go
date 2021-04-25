// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/beevik/ntp"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/bandwidth"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var log = log15.New("pkg", "node")

type Node struct {
	goes     co.Goes
	packer   *packer.Packer
	cons     *consensus.Consensus
	consLock sync.Mutex

	master         *Master
	repo           *chain.Repository
	logDB          *logdb.LogDB
	txPool         *txpool.TxPool
	txStashPath    string
	comm           *comm.Communicator
	commitLock     sync.Mutex
	targetGasLimit uint64
	skipLogs       bool
	logDBFailed    bool
	bandwidth      bandwidth.Bandwidth
	forkConfig     thor.ForkConfig
	stater         *state.Stater
	seeder         *poa.Seeder
}

func New(
	master *Master,
	repo *chain.Repository,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	txStashPath string,
	comm *comm.Communicator,
	targetGasLimit uint64,
	skipLogs bool,
	forkConfig thor.ForkConfig,
) *Node {
	return &Node{
		packer:         packer.New(repo, stater, master.Address(), master.Beneficiary, forkConfig),
		cons:           consensus.New(repo, stater, forkConfig),
		master:         master,
		repo:           repo,
		logDB:          logDB,
		txPool:         txPool,
		txStashPath:    txStashPath,
		comm:           comm,
		targetGasLimit: targetGasLimit,
		skipLogs:       skipLogs,
		forkConfig:     forkConfig,
		stater:         stater,
		seeder:         poa.NewSeeder(repo),
	}
}

// Run starts the node process.
func (n *Node) Run(ctx context.Context) error {
	n.comm.Sync(n.handleBlockStream)

	n.goes.Go(func() { n.houseKeeping(ctx) })
	n.goes.Go(func() { n.txStashLoop(ctx) })
	n.goes.Go(func() { n.packerLoop(ctx) })
	n.goes.Go(func() { n.backerLoop(ctx) })

	n.goes.Wait()
	return nil
}

func (n *Node) handleBlockStream(ctx context.Context, stream <-chan *block.Block) (err error) {
	log.Debug("start to process block stream")
	defer log.Debug("process block stream done", "err", err)
	var stats blockStats
	startTime := mclock.Now()

	report := func(block *block.Block) {
		log.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
		stats = blockStats{}
		startTime = mclock.Now()
	}

	var blk *block.Block
	for blk = range stream {
		if _, err := n.processBlock(blk, &stats); err != nil {
			return err
		}

		if stats.processed > 0 &&
			mclock.Now()-startTime > mclock.AbsTime(time.Second*2) {
			report(blk)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if blk != nil && stats.processed > 0 {
		report(blk)
	}
	return nil
}

func (n *Node) houseKeeping(ctx context.Context) {
	log.Debug("enter house keeping")
	defer log.Debug("leave house keeping")

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockCh := make(chan *comm.NewBlockEvent)
	scope.Track(n.comm.SubscribeBlock(newBlockCh))

	futureTicker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer futureTicker.Stop()

	connectivityTicker := time.NewTicker(time.Second)
	defer connectivityTicker.Stop()

	var noPeerTimes int

	futureBlocks := cache.NewRandCache(32)

	for {
		select {
		case <-ctx.Done():
			return
		case newBlock := <-newBlockCh:
			var stats blockStats
			if isTrunk, err := n.processBlock(newBlock.Block, &stats); err != nil {
				if consensus.IsFutureBlock(err) ||
					(consensus.IsParentMissing(err) && futureBlocks.Contains(newBlock.Header().ParentID())) {
					log.Debug("future block added", "id", newBlock.Header().ID())
					futureBlocks.Set(newBlock.Header().ID(), newBlock.Block)
				}
			} else if isTrunk {
				n.comm.BroadcastBlock(newBlock.Block)
				log.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Block.Header())...)
			}
		case <-futureTicker.C:
			// process future blocks
			var blocks []*block.Block
			futureBlocks.ForEach(func(ent *cache.Entry) bool {
				blocks = append(blocks, ent.Value.(*block.Block))
				return true
			})
			sort.Slice(blocks, func(i, j int) bool {
				return blocks[i].Header().Number() < blocks[j].Header().Number()
			})
			var stats blockStats
			for i, block := range blocks {
				if isTrunk, err := n.processBlock(block, &stats); err == nil || consensus.IsKnownBlock(err) {
					log.Debug("future block consumed", "id", block.Header().ID())
					futureBlocks.Remove(block.Header().ID())
					if isTrunk {
						n.comm.BroadcastBlock(block)
					}
				}

				if stats.processed > 0 && i == len(blocks)-1 {
					log.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
				}
			}
		case <-connectivityTicker.C:
			if n.comm.PeerCount() == 0 {
				noPeerTimes++
				if noPeerTimes > 30 {
					noPeerTimes = 0
					go checkClockOffset()
				}
			} else {
				noPeerTimes = 0
			}
		}
	}
}

func (n *Node) txStashLoop(ctx context.Context) {
	log.Debug("enter tx stash loop")
	defer log.Debug("leave tx stash loop")

	db, err := leveldb.OpenFile(n.txStashPath, &opt.Options{})
	if err != nil {
		log.Error("create tx stash", "err", err)
		return
	}
	defer db.Close()

	stash := newTxStash(db, 1000)

	{
		txs := stash.LoadAll()
		n.txPool.Fill(txs, false)
		log.Debug("loaded txs from stash", "count", len(txs))
	}

	var scope event.SubscriptionScope
	defer scope.Close()

	txCh := make(chan *txpool.TxEvent)
	scope.Track(n.txPool.SubscribeTxEvent(txCh))
	for {
		select {
		case <-ctx.Done():
			return
		case txEv := <-txCh:
			// skip executables
			if txEv.Executable != nil && *txEv.Executable {
				continue
			}
			// only stash non-executable txs
			if err := stash.Save(txEv.Tx); err != nil {
				log.Warn("stash tx", "id", txEv.Tx.ID(), "err", err)
			} else {
				log.Debug("stashed tx", "id", txEv.Tx.ID())
			}
		}
	}
}

func (n *Node) processBlock(blk *block.Block, stats *blockStats) (bool, error) {
	// consensus object is not thread-safe
	n.consLock.Lock()
	startTime := mclock.Now()
	stage, receipts, schedElapsed, err := n.cons.Process(blk, uint64(time.Now().Unix()))
	execElapsed := mclock.Now() - startTime
	n.consLock.Unlock()

	if err != nil {
		switch {
		case consensus.IsKnownBlock(err):
			stats.UpdateIgnored(1)
			return false, nil
		case consensus.IsFutureBlock(err) || consensus.IsParentMissing(err):
			stats.UpdateQueued(1)
		case consensus.IsCritical(err):
			msg := fmt.Sprintf(`failed to process block due to consensus failure \n%v\n`, blk.Header())
			log.Error(msg, "err", err)
		default:
			log.Error("failed to process block", "err", err)
		}
		return false, err
	}

	prevTrunk, curTrunk, err := n.commitBlock(stage, blk, receipts)
	if err != nil {
		log.Error("failed to commit block", "err", err)
		return false, err
	}
	commitElapsed := mclock.Now() - startTime - execElapsed

	if v, updated := n.bandwidth.Update(blk.Header(), time.Duration(execElapsed+commitElapsed)); updated {
		log.Debug("bandwidth updated", "gps", v)
	}

	stats.UpdateProcessed(1, len(receipts), schedElapsed, execElapsed-schedElapsed, commitElapsed, blk.Header().GasUsed())
	n.processFork(prevTrunk, curTrunk)
	return prevTrunk.HeadID() != curTrunk.HeadID(), nil
}

func (n *Node) commitBlock(stage *state.Stage, newBlock *block.Block, receipts tx.Receipts) (*chain.Chain, *chain.Chain, error) {
	n.commitLock.Lock()
	defer n.commitLock.Unlock()

	if _, err := stage.Commit(); err != nil {
		return nil, nil, errors.Wrap(err, "commit state")
	}
	if err := n.repo.AddBlock(newBlock, receipts); err != nil {
		return nil, nil, errors.Wrap(err, "add block")
	}

	var prevBest = n.repo.BestBlock()
	becomeNewBest, err := n.compare(newBlock.Header(), prevBest.Header())
	if err != nil {
		return nil, nil, errors.Wrap(err, "compare head")
	}

	if becomeNewBest {
		if !n.skipLogs && !n.logDBFailed {
			diff, err := n.repo.NewChain(newBlock.Header().ParentID()).Exclude(
				n.repo.NewChain(prevBest.Header().ID()))
			if err != nil {
				n.logDBFailed = true
				log.Warn("failed to write logs", "err", err)
			}

			if err := n.writeLogs(diff, newBlock, receipts); err != nil {
				n.logDBFailed = true
				log.Warn("failed to write logs", "err", err)
			}
		}
		if err := n.repo.SetBestBlockID(newBlock.Header().ID()); err != nil {
			return nil, nil, err
		}
	}

	return n.repo.NewChain(prevBest.Header().ID()), n.repo.NewBestChain(), nil
}

// build forks comparing two heads.
func (n *Node) buildFork(b1 *block.Header, b2 *block.Header) (ancestor *block.Header, br1 []thor.Bytes32, br2 []thor.Bytes32, err error) {
	c1 := n.repo.NewChain(b1.ID())
	c2 := n.repo.NewChain(b2.ID())

	br1, err = c1.Exclude(c2)
	if err != nil {
		return
	}
	br2, err = c2.Exclude(c1)
	if err != nil {
		return
	}

	var ancestorNumber uint32
	if len(br1) > 0 {
		ancestorNumber = block.Number(br1[0]) - 1
	} else {
		ancestorNumber = block.Number(br2[0]) - 1
	}

	ancestor, err = c1.GetBlockHeader(ancestorNumber)
	if err != nil {
		return
	}

	return
}

// giving the list of blockID in ascending order, find the latest heavy block.
func (n *Node) findLatestHeavyBlock(ids []thor.Bytes32) (*block.Header, error) {
	for i := len(ids) - 1; i >= 0; i-- {
		sum, err := n.repo.GetBlockSummary(ids[i])
		if err != nil {
			return nil, err
		}

		parent, err := n.repo.GetBlockSummary(sum.Header.ParentID())
		if err != nil {
			return nil, err
		}

		if sum.Header.TotalQuality() > parent.Header.TotalQuality() {
			return sum.Header, nil
		}
	}
	return nil, nil
}

// compare compares two chains, returns true if a>b.
func (n *Node) compare(b1 *block.Header, b2 *block.Header) (bool, error) {
	if b1.ID() == b2.ID() {
		return false, nil
	}

	q1 := b1.TotalQuality()
	q2 := b2.TotalQuality()

	if q1 > q2 {
		return true, nil
	}
	if q1 < q2 {
		return false, nil
	}

	// total quality are equal, find the latest heavy block on both branches
	// later heavy block is better

	if q1 > 0 {
		// Non-zero quality means blocks are at post VIP-193 stage
		ancestor, br1, br2, err := n.buildFork(b1, b2)
		if err != nil {
			return false, errors.Wrap(err, "build fork")
		}
		if len(br2) == 0 {
			// c1 includes c2
			return true, nil
		}
		if len(br1) == 0 {
			// c2 includes c1
			return false, nil
		}

		if q1 > ancestor.TotalQuality() {
			h1, err := n.findLatestHeavyBlock(br1)
			if err != nil {
				return false, err
			}

			h2, err := n.findLatestHeavyBlock(br2)
			if err != nil {
				return false, err
			}

			if h1.Timestamp() > h2.Timestamp() {
				return true, nil
			}

			if h1.Timestamp() < h2.Timestamp() {
				return false, nil
			}
		}
	}

	s1 := b1.TotalScore()
	s2 := b2.TotalScore()
	if s1 > s2 {
		return true, nil
	}
	if s2 < s1 {
		return false, nil
	}
	// total scores are equal

	// smaller ID is preferred, since block with smaller ID usually has larger average score.
	// also, it's a deterministic decision.
	return bytes.Compare(b1.ID().Bytes(), b2.ID().Bytes()) < 0, nil
}

func (n *Node) writeLogs(diff []thor.Bytes32, newBlock *block.Block, newReceipts tx.Receipts) error {
	// write full trunk blocks to prevent logs dropped
	// in rare condition of long fork
	return n.logDB.Log(func(w *logdb.Writer) error {
		for _, id := range diff {
			b, err := n.repo.GetBlock(id)
			if err != nil {
				return err
			}
			receipts, err := n.repo.GetBlockReceipts(id)
			if err != nil {
				return err
			}
			if err := w.Write(b, receipts); err != nil {
				return err
			}
		}
		return w.Write(newBlock, newReceipts)
	})
}

func (n *Node) processFork(prevTrunk, curTrunk *chain.Chain) {
	sideIds, err := prevTrunk.Exclude(curTrunk)
	if err != nil {
		log.Warn("failed to process fork", "err", err)
		return
	}
	if len(sideIds) == 0 {
		return
	}

	if n := len(sideIds); n >= 2 {
		log.Warn(fmt.Sprintf(
			`⑂⑂⑂⑂⑂⑂⑂⑂ FORK HAPPENED ⑂⑂⑂⑂⑂⑂⑂⑂
side-chain:   %v  %v`,
			n, sideIds[n-1]))
	}

	for _, id := range sideIds {
		b, err := n.repo.GetBlock(id)
		if err != nil {
			log.Warn("failed to process fork", "err", err)
			return
		}
		for _, tx := range b.Transactions() {
			if err := n.txPool.Add(tx); err != nil {
				log.Debug("failed to add tx to tx pool", "err", err, "id", tx.ID())
			}
		}
	}
}

func checkClockOffset() {
	resp, err := ntp.Query("pool.ntp.org")
	if err != nil {
		log.Debug("failed to access NTP", "err", err)
		return
	}
	if resp.ClockOffset > time.Duration(thor.BlockInterval)*time.Second/2 {
		log.Warn("clock offset detected", "offset", common.PrettyDuration(resp.ClockOffset))
	}
}
