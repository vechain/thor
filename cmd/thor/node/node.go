// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
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
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var log = log15.New("pkg", "node")

// Node ...
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

	// mu       sync.Mutex
	// beacon   thor.Bytes32
	// seed     thor.Bytes32
	// roundNum uint32
	// epochNum uint32

	// rw  sync.RWMutex
	// bs  *block.Summary
	// eds map[thor.Address]*block.Endorsement
}

// New ...
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
	master.deriveVrfPrivateKey()
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
	}
}

// Run ...
func (n *Node) Run(ctx context.Context, mode int) error {
	if mode != 1 {
		log.Info("test mode on", "mod", mode)
	}

	switch mode {
	case 0: // empty loop
		n.goes.Go(func() { emptyLoop(ctx) })
	case 1: // normal case
		n.comm.Sync(n.handleBlockStream)
		n.goes.Go(func() { n.houseKeeping(ctx) })
		n.goes.Go(func() { n.txStashLoop(ctx) })
		n.goes.Go(func() { n.packerLoop(ctx) })
	case 2:
		/**
		 * To test the low-level broadcasting funcs
		 *
		 * All nodes produce one random instance of each of the
		 * following typtes: block.Summary, block.Endoresement,
		 * block.TxSet and block.Header and then broadcast them to
		 * other nodes.
		 */
		n.goes.Go(func() { n.simpleHouseKeeping(ctx) })
		n.goes.Go(func() { n.testCase2(ctx) })
	case 3:
		/**
		 * To test sync
		 *
		 * It is tested on a three-node local customnet.
		 *
		 * Node1 is made to produce a valid new block and commit
		 * locally. It then broadcasts the block ID.
		 *
		 * Expect: the other two nodes to sync the new block after
		 * receiving the new block id.
		 */
		n.comm.Sync(n.handleBlockStream)
		if n.getNodeID() == 1 {
			n.goes.Go(func() { n.testCase3(ctx) })
		}
		n.goes.Go(func() { emptyLoop(ctx) })
	case 4:
		/**
		 * To test the assembly of an empty block
		 *
		 * It is tested on a three-node customnet.
		 *
		 * Node2 is made to produce and broadcast the tx set and header.
		 *
		 * Expect: the other nodes to assemble the new block after
		 * receiving the block header
		 */
		n.goes.Go(func() { n.houseKeeping(ctx) })
		if n.getNodeID() == 2 {
			n.goes.Go(func() { n.testCase4(ctx) })
		}
	case 5:
		/**
		 * To test the assembly of a block with the tx set arrives
		 * LATER than the header
		 *
		 * It is tested on a three-node customnet.
		 *
		 * Node2 is made to produce and broadcast the tx set and header.
		 *
		 * Expect: the other two nodes receive the tx set and header and
		 * to assemble the new block after receiving the header.
		 */
		n.goes.Go(func() { n.houseKeeping(ctx) })
		if n.getNodeID() == 2 {
			n.goes.Go(func() { n.testCase5(ctx) })
		}
	case 6:
		/**
		 * To test the assembly of a block with the tx set arrives
		 * EARLIER than the block header.
		 *
		 * It is tested on a three-node customnet.
		 *
		 * Node2 is made to produce and broadcast the tx set and hd.
		 *
		 * Expect: the other two nodes receive the tx set and header and
		 * to assemble the new block after receiving the tx set.
		 */
		n.goes.Go(func() { n.houseKeeping(ctx) })
		if n.getNodeID() == 2 {
			n.goes.Go(func() { n.testCase6(ctx) })
		}
	case 7:
		/**
		 * To test the part of packerLoop that make endorsments.
		 *
		 * It is tested on a three-node customnet.
		 *
		 * Node2 is made to produce and broadcast two new block summaries and
		 * expects the other nodes to sent back their endorsement on the
		 * first one and rejects the second.
		 *
		 * The other nodes run endorserLoop that waits any new block summary
		 * and produces and broadcasts a new endorsement
		 *
		 * Expect: the other nodes to endorse and broadcast their endorsements.
		 */
		n.comm.Sync(n.handleBlockStream)
		n.goes.Go(func() { n.houseKeeping(ctx) })
		if n.getNodeID() == 2 {
			n.goes.Go(func() { n.testCase7(ctx) })
		}
		n.goes.Go(func() { n.endorserLoop(ctx) })
	case 8:
		/**
		 * To test the normal procedure of pack a new block.
		 *
		 * It is tested on a three-node customnet.
		 *
		 * Node2 starts with packerLoop while the other two nodes starts
		 * with endorserLoop.
		 *
		 * Expect: Node2 constantly produce and broadcast new blocks.
		 */
		n.comm.Sync(n.handleBlockStream)
		n.goes.Go(func() { n.houseKeeping(ctx) })
		if n.getNodeID() == 2 {
			n.goes.Go(func() { n.packerLoop(ctx) })
		} else {
			n.goes.Go(func() { n.endorserLoop(ctx) })
		}
	default:
		panic("test does not exist")
	}

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

// houseKeeping handles:
// 1. receive & broadcast new block
// 2. receive & broadcast new header
// 3. receive & broadcast new tx set
// 4. assemble new block from header and tx set
// 5. receive & broadcast new bs
// 6. receive & broadcast new bs
func (n *Node) houseKeeping(ctx context.Context) {
	debugLog := func(str string, kv ...interface{}) {
		log.Debug(str, append([]interface{}{"key", "house"}, kv...)...)
	}

	debugLog("enter house keeping")
	defer debugLog("leave house keeping")

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockCh := make(chan *comm.NewBlockEvent)
	newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	newEndorsementCh := make(chan *comm.NewEndorsementEvent)
	newTxSetCh := make(chan *comm.NewTxSetEvent)
	newBlockHeaderCh := make(chan *comm.NewBlockHeaderEvent)

	scope.Track(n.comm.SubscribeBlock(newBlockCh))
	scope.Track(n.comm.SubscribeBlockSummary(newBlockSummaryCh))
	scope.Track(n.comm.SubscribeEndorsement(newEndorsementCh))
	scope.Track(n.comm.SubscribeTxSet(newTxSetCh))
	scope.Track(n.comm.SubscribeBlockHeader(newBlockHeaderCh))

	futureTicker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer futureTicker.Stop()

	connectivityTicker := time.NewTicker(time.Second)
	defer connectivityTicker.Stop()

	var (
		noPeerTimes int
		lbs         *block.Summary // locally stored bs
		lts         *block.TxSet   // locally stored tx set
		lh          *block.Header  // locally stored hd
	)

	futureBlocks := cache.NewRandCache(32)

	knownBs := cache.NewRandCache(32)
	knownTs := cache.NewRandCache(32)
	knownEd := cache.NewRandCache(32)
	knownHd := cache.NewRandCache(32)

	for {
		select {
		case <-ctx.Done():
			return
		case newBlock := <-newBlockCh:
			var stats blockStats
			if isTrunk, err := n.processBlock(newBlock.Block, &stats); err != nil {
				if consensus.IsFutureBlock(err) ||
					(consensus.IsParentMissing(err) && futureBlocks.Contains(newBlock.Header().ParentID())) {
					debugLog("future block added", "id", newBlock.Header().ID())
					futureBlocks.Set(newBlock.Header().ID(), newBlock.Block)
				}
			} else if isTrunk {
				n.comm.BroadcastBlock(newBlock.Block)
				debugLog(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Block.Header())...)
			}

		case ev := <-newBlockSummaryCh:
			bs := ev.Summary

			if _, ok := knownBs.Get(bs.ID()); ok {
				continue
			} else {
				knownBs.Set(bs.ID(), struct{}{})
			}

			debugLog("<== bs", "id", bs.ID())

			now := uint64(time.Now().Unix())
			parentHeader := n.repo.BestBlock().Header()

			// Only receive one bs from the same leader once in the same round
			if lbs != nil {
				if n.cons.ValidateBlockSummary(lbs, parentHeader, now) == nil {
					continue
				}
				debugLog("set lbs=nil")
				lbs = nil
			}

			// validate the new bs
			if err := n.cons.ValidateBlockSummary(bs, parentHeader, now); err != nil {
				debugLog("invalid bs", "err", err, "id", bs.ID())
				continue
			}

			lbs = bs
			debugLog("bs ==>", "id", bs.ID())
			n.comm.BroadcastBlockSummary(bs)

		case ev := <-newTxSetCh:
			ts := ev.TxSet

			if _, ok := knownTs.Get(ts.ID()); ok {
				continue
			} else {
				knownTs.Set(ts.ID(), struct{}{})
			}

			debugLog("<== ts", "id", ts.ID())

			parentHeader := n.repo.BestBlock().Header()
			now := uint64(time.Now().Unix())

			// Only receive one tx set from the same leader once in the same round
			if lts != nil {
				if n.cons.ValidateTxSet(lts, parentHeader, now) == nil {
					continue
				}
				debugLog("set lts=nil")
				lts = nil
			}

			// validate the new tx set
			if err := n.cons.ValidateTxSet(ts, parentHeader, now); err != nil {
				debugLog("invalid ts", "err", err, "id", ts.ID())
				continue
			}

			if lbs != nil {
				// only reject the new tx set if the locally save bs is valid and they do not match
				if n.cons.ValidateBlockSummary(lbs, parentHeader, now) == nil && lbs.TxsRoot() != ts.TxsRoot() {
					debugLog("ts rejected", "id", ts.ID())
					continue
				}
			}

			lts = ts
			debugLog("ts ==>", "id", ts.ID())
			n.comm.BroadcastTxSet(ts)

			// assemble the block if the header has been received
			if lh != nil && n.cons.ValidateBlockHeader(lh, parentHeader, now) == nil && lh.TxsRoot() == ts.TxsRoot() {
				debugLog("assembling new block", "id", lh.ID())
				n.assembleNewBlock(lh, lts, parentHeader, now)
			}

		case ev := <-newEndorsementCh:
			ed := ev.Endorsement

			if _, ok := knownEd.Get(ed.ID()); ok {
				continue
			} else {
				knownEd.Set(ed.ID(), struct{}{})
			}

			debugLog("<== ed", "id", ed.ID())

			parentHeader := n.chain.BestBlock().Header()
			now := uint64(time.Now().Unix())

			if err := n.cons.ValidateEndorsement(ed, parentHeader, now); err != nil {
				debugLog("invalid ed", "err", err, "id", ed.ID())
				continue
			}

			debugLog("ed ==>", "id", ed.ID())
			n.comm.BroadcastEndorsement(ed)

		case ev := <-newBlockHeaderCh:
			header := ev.Header

			if _, ok := knownHd.Get(header.ID()); ok {
				continue
			} else {
				knownHd.Set(header.ID(), struct{}{})
			}

			debugLog("<== hd", "id", header.ID())

			parentHeader := n.chain.BestBlock().Header()
			now := uint64(time.Now().Unix())

			// Only receive one tx set from the same leader once in the same round
			if lh != nil {
				if n.cons.ValidateBlockHeader(lh, parentHeader, now) == nil {
					continue
				}
				debugLog("set lh=nil")
				lh = nil
			}

			if err := n.cons.ValidateBlockHeader(header, parentHeader, now); err != nil {
				debugLog("invalid hd", "id", header.ID(), "err", err)
				continue
			}

			lh = header
			debugLog("hd ==>", "key", "header", "id", header.ID())
			n.comm.BroadcastBlockHeader(header)

			// assemble the block either when there is an empty transaction list or
			// when there has been a tx set received and its tx root matches the one
			// computed from the header
			if (lts == nil && header.TxsRoot() == tx.EmptyRoot) ||
				(lts != nil && lts.TxsRoot() == header.TxsRoot() &&
					n.cons.ValidateTxSet(lts, parentHeader, now) == nil) {
				debugLog("assembling new block", "key", "header", "id", header.ID())
				n.assembleNewBlock(lh, lts, parentHeader, now)
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
					debugLog("future block consumed", "id", block.Header().ID())
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

// assembleNewBlock is not responsible for validating the input header and tx set
func (n *Node) assembleNewBlock(header *block.Header, ts *block.TxSet, parentHeader *block.Header, now uint64) {
	var blk *block.Block
	if ts == nil {
		blk = block.Compose(header, tx.Transactions{})
	} else {
		blk = block.Compose(header, ts.Transactions())
	}

	var stats blockStats
	if isTrunk, err := n.processBlock(blk, &stats); err != nil {
		// reset locally saved header and tx set
		header = nil
		ts = nil
		log.Error("Failed to proccess the assembled new block", "err", err)
	} else if isTrunk {
		// only broadcast the new block id in the current round
		n.comm.BroadcastBlockID(blk.Header().ID())
		log.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(blk.Header())...)
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
		n.txPool.Fill(txs)
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
	stage, receipts, err := n.cons.Process(blk, uint64(time.Now().Unix()))
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
			msg := fmt.Sprintf("failed to process block due to consensus failure \n%v\n", blk.Header())
			log.Error(msg, "err", err)
		default:
			log.Error("failed to process block", "err", err)
		}
		return false, err
	}

	if _, err := stage.Commit(); err != nil {
		log.Error("failed to commit state", "err", err)
		return false, err
	}

	prevTrunk, curTrunk, err := n.commitBlock(blk, receipts)
	if err != nil {
		log.Error("failed to commit block", "err", err)
		return false, err
	}
	commitElapsed := mclock.Now() - startTime - execElapsed

	if v, updated := n.bandwidth.Update(blk.Header(), time.Duration(execElapsed+commitElapsed)); updated {
		log.Debug("bandwidth updated", "gps", v)
	}

	stats.UpdateProcessed(1, len(receipts), execElapsed, commitElapsed, blk.Header().GasUsed())
	n.processFork(prevTrunk, curTrunk)
	return prevTrunk.HeadID() != curTrunk.HeadID(), nil
}

func (n *Node) commitBlock(newBlock *block.Block, receipts tx.Receipts) (*chain.Chain, *chain.Chain, error) {
	n.commitLock.Lock()
	defer n.commitLock.Unlock()

	best := n.repo.BestBlock()
	err := n.repo.AddBlock(newBlock, receipts)
	if err != nil {
		return nil, nil, err
	}
	if newBlock.Header().BetterThan(best.Header()) {
		if err := n.repo.SetBestBlockID(newBlock.Header().ID()); err != nil {
			return nil, nil, err
		}
	}
	prevTrunk := n.repo.NewChain(best.Header().ID())
	curTrunk := n.repo.NewBestChain()

	diff, err := curTrunk.Exclude(prevTrunk)
	if err != nil {
		return nil, nil, err
	}

	if !n.skipLogs {
		if n.logDBFailed {
			log.Warn("!!!log db skipped due to write failure (restart required to recover)")
		} else {
			if err := n.writeLogs(diff); err != nil {
				n.logDBFailed = true
				return nil, nil, errors.Wrap(err, "write logs")
			}
		}
	}
	return prevTrunk, curTrunk, nil
}

func (n *Node) writeLogs(diff []thor.Bytes32) error {
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
		return nil
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
