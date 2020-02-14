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
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
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
	chain          *chain.Chain
	logDB          *logdb.LogDB
	txPool         *txpool.TxPool
	txStashPath    string
	comm           *comm.Communicator
	commitLock     sync.Mutex
	targetGasLimit uint64
	skipLogs       bool
	logDBFailed    bool
}

// New ...
func New(
	master *Master,
	chain *chain.Chain,
	stateCreator *state.Creator,
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
		packer:         packer.New(chain, stateCreator, master.Address(), master.Beneficiary, forkConfig),
		cons:           consensus.New(chain, stateCreator, forkConfig),
		master:         master,
		chain:          chain,
		logDB:          logDB,
		txPool:         txPool,
		txStashPath:    txStashPath,
		comm:           comm,
		targetGasLimit: targetGasLimit,
		skipLogs:       skipLogs,
	}
}

// Run ...
func (n *Node) Run(ctx context.Context) error {
	mode := 4

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
		 * Description: All nodes produce one random instance of
		 * each of the typtes: block.Summary, block.Endoresement,
		 * block.TxSet and block.Header and broadcast them to
		 * other nodes. The test is designed to test the low-level
		 * broadcasting functions.
		 *
		 * Status: PASS
		 */
		n.goes.Go(func() { n.simpleHouseKeeping(ctx) })
		n.goes.Go(func() { n.testBroadcasting(ctx) })
	case 3:
		/**
		 * Description: It is tested on a three-node local customnet.
		 * Node1 is made to produce a valid new block and commit
		 * locally. It then broadcasts the block ID. The test is
		 * designed to test the sync mechanism.
		 *
		 * Status: PASS
		 */
		n.comm.Sync(n.handleBlockStream)
		n.goes.Go(func() { n.testSync(ctx) })
		n.goes.Go(func() { emptyLoop(ctx) })
	case 4:
		/**
		 * Description: The test is designed to test the assembly of
		 * an empty block by the house keeping loop. It is tested on
		 * a three-node local customnet. Node1 is made to produce a
		 * new empty block and broadcast its block header.
		 *
		 * Status: PASS
		 */
		n.goes.Go(func() { n.houseKeeping(ctx) })
		n.goes.Go(func() { n.testEmptyBlockAssembling(ctx) })
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
// 5. receive & broadcast new block summary
// 6. receive & broadcast new endorsement
func (n *Node) houseKeeping(ctx context.Context) {
	log.Debug("enter house keeping")
	defer log.Debug("leave house keeping")

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
		lbs         *block.Summary // locally stored block summary
		lts         *block.TxSet   // locally stored tx set
		lh          *block.Header  // locally stored block header
	)

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

		case ev := <-newBlockSummaryCh:
			bs := ev.Summary

			now := uint64(time.Now().Unix())
			parentHeader := n.chain.BestBlock().Header()

			// Only receive one block summary from the same leader once in the same round
			if lbs != nil {
				if n.cons.ValidateBlockSummary(lbs, parentHeader, now) == nil {
					continue
				}
				lbs = nil
			}

			// validate the new block summary
			if n.cons.ValidateBlockSummary(bs, parentHeader, now) != nil {
				continue
			}

			// log.Debug("Broadcasting block summary", "hash", bs.RLPHash())
			lbs = bs
			n.comm.BroadcastBlockSummary(bs)

		case ev := <-newTxSetCh:
			ts := ev.TxSet
			log.Debug("received new tx set", "id", ts.ID())

			parentHeader := n.chain.BestBlock().Header()
			now := uint64(time.Now().Unix())

			// Only receive one tx set from the same leader once in the same round
			if lts != nil {
				if n.cons.ValidateTxSet(lts, parentHeader, now) == nil {
					continue
				}
				lts = nil
			}

			// validate the new tx set
			if n.cons.ValidateTxSet(ts, parentHeader, now) != nil {
				log.Debug("invalid new tx set", "id", ts.ID())
				continue
			}

			if lbs != nil {
				// only reject the new tx set if the locally save block summary is valid and they do not match
				if n.cons.ValidateBlockSummary(lbs, parentHeader, now) == nil && lbs.TxsRoot() != ts.TxsRoot() {
					log.Debug("new tx set rejected", "txsetid", ts.ID(), "bsid", lbs.ID())
					continue
				}
			}

			lts = ts
			log.Debug("broadcasting tx set", "id", ts.ID())
			n.comm.BroadcastTxSet(ts)

			// assemble the block if the header has been received
			if lh != nil && n.cons.ValidateBlockHeader(lh, parentHeader, now) == nil && lh.TxsRoot() == ts.TxsRoot() {
				log.Debug("assembling new block", "id", lh.ID())
				n.assembleNewBlock(lh, lts, parentHeader, now)
			}

		case ev := <-newEndorsementCh:
			ed := ev.Endorsement

			parentHeader := n.chain.BestBlock().Header()
			now := uint64(time.Now().Unix())

			if n.cons.ValidateEndorsement(ed, parentHeader, now) != nil {
				log.Debug("invalid new endorsement", "id", ed.ID())
				continue
			}

			log.Debug("broadcasting endorsement", "id", ed.ID())
			n.comm.BroadcastEndorsement(ed)

		case ev := <-newBlockHeaderCh:
			header := ev.Header
			log.Info("received new block header", "id", header.ID())

			parentHeader := n.chain.BestBlock().Header()
			now := uint64(time.Now().Unix())

			// Only receive one tx set from the same leader once in the same round
			if lh != nil {
				if n.cons.ValidateBlockHeader(lh, parentHeader, now) == nil {
					continue
				}
				lh = nil
			}

			if err := n.cons.ValidateBlockHeader(header, parentHeader, now); err != nil {
				log.Info("invalid new block header", "id", header.ID(), "err", err)
				continue
			}

			lh = header
			log.Info("broadcasting new block header", "id", header.ID())
			n.comm.BroadcastBlockHeader(header)

			// assemble the block either when there is an empty transaction list or
			// when there has been a tx set received and its tx root matches the one
			// computed from the header
			if (lts == nil && header.TxsRoot() == tx.EmptyRoot) ||
				(lts != nil && lts.TxsRoot() == header.TxsRoot() &&
					n.cons.ValidateTxSet(lts, parentHeader, now) == nil) {
				log.Info("assembling new block", "id", header.ID())
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

	db, err := lvldb.New(n.txStashPath, lvldb.Options{})
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
	startTime := mclock.Now()
	now := uint64(time.Now().Unix())

	// consensus object is not thread-safe
	n.consLock.Lock()
	stage, receipts, err := n.cons.Process(blk, now)
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

	execElapsed := mclock.Now() - startTime

	if _, err := stage.Commit(); err != nil {
		log.Error("failed to commit state", "err", err)
		return false, err
	}

	fork, err := n.commitBlock(blk, receipts)
	if err != nil {
		if !n.chain.IsBlockExist(err) {
			log.Error("failed to commit block", "err", err)
		}
		return false, err
	}
	commitElapsed := mclock.Now() - startTime - execElapsed
	stats.UpdateProcessed(1, len(receipts), execElapsed, commitElapsed, blk.Header().GasUsed())
	n.processFork(fork)
	return len(fork.Trunk) > 0, nil
}

func (n *Node) commitBlock(newBlock *block.Block, receipts tx.Receipts) (*chain.Fork, error) {
	n.commitLock.Lock()
	defer n.commitLock.Unlock()

	fork, err := n.chain.AddBlock(newBlock, receipts)
	if err != nil {
		return nil, err
	}
	if !n.skipLogs {
		if n.logDBFailed {
			log.Warn("!!!log db skipped due to write failure (restart required to recover)")
		} else {
			if err := n.writeLogs(fork.Trunk); err != nil {
				n.logDBFailed = true
				return nil, errors.Wrap(err, "write logs")
			}
		}
	}
	return fork, nil
}

func (n *Node) writeLogs(trunk []*block.Header) error {
	// write full trunk blocks to prevent logs dropped
	// in rare condition of long fork
	task := n.logDB.NewTask()
	for _, header := range trunk {
		body, err := n.chain.GetBlockBody(header.ID())
		if err != nil {
			return err
		}
		receipts, err := n.chain.GetBlockReceipts(header.ID())
		if err != nil {
			return err
		}

		task.ForBlock(header)
		for i, tx := range body.Txs {
			origin, _ := tx.Origin()
			task.Write(tx.ID(), origin, receipts[i].Outputs)
		}
	}
	return task.Commit()
}

func (n *Node) processFork(fork *chain.Fork) {
	if len(fork.Branch) >= 2 {
		trunkLen := len(fork.Trunk)
		branchLen := len(fork.Branch)
		log.Warn(fmt.Sprintf(
			`⑂⑂⑂⑂⑂⑂⑂⑂ FORK HAPPENED ⑂⑂⑂⑂⑂⑂⑂⑂
ancestor: %v
trunk:    %v  %v
branch:   %v  %v`, fork.Ancestor,
			trunkLen, fork.Trunk[trunkLen-1],
			branchLen, fork.Branch[branchLen-1]))
	}
	for _, header := range fork.Branch {
		body, err := n.chain.GetBlockBody(header.ID())
		if err != nil {
			log.Warn("failed to get block body", "err", err, "blockid", header.ID())
			continue
		}
		for _, tx := range body.Txs {
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
