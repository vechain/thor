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
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/bandwidth"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var logger = log.WithContext("pkg", "node")

var (
	// error when the block larger than known max block number + 1
	errBlockTemporaryUnprocessable = errors.New("block temporary unprocessable")
	errKnownBlock                  = errors.New("block already in the chain")
	errParentMissing               = errors.New("parent block is missing")
	errBFTRejected                 = errors.New("block rejected by BFT engine")
)

type Node struct {
	packer         *packer.Packer
	cons           *consensus.Consensus
	master         *Master
	repo           *chain.Repository
	bft            *bft.Engine
	logDB          *logdb.LogDB
	txPool         *txpool.TxPool
	txStashPath    string
	comm           *comm.Communicator
	targetGasLimit uint64
	skipLogs       bool
	forkConfig     thor.ForkConfig

	logDBFailed bool
	bandwidth   bandwidth.Bandwidth
	maxBlockNum uint32
	processLock sync.Mutex
	logWorker   *worker
}

func New(
	master *Master,
	repo *chain.Repository,
	bft *bft.Engine,
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
		bft:            bft,
		logDB:          logDB,
		txPool:         txPool,
		txStashPath:    txStashPath,
		comm:           comm,
		targetGasLimit: targetGasLimit,
		skipLogs:       skipLogs,
		forkConfig:     forkConfig,
	}
}

func (n *Node) Run(ctx context.Context) error {
	logWorker := newWorker()
	defer logWorker.Close()

	n.logWorker = logWorker

	maxBlockNum, err := n.repo.GetMaxBlockNum()
	if err != nil {
		return err
	}
	n.maxBlockNum = maxBlockNum

	var goes co.Goes
	goes.Go(func() { n.comm.Sync(ctx, n.handleBlockStream) })
	goes.Go(func() { n.houseKeeping(ctx) })
	goes.Go(func() { n.txStashLoop(ctx) })
	goes.Go(func() { n.packerLoop(ctx) })

	goes.Wait()
	return nil
}

func (n *Node) handleBlockStream(ctx context.Context, stream <-chan *block.Block) (err error) {
	logger.Debug("start to process block stream")
	defer logger.Debug("process block stream done", "err", err)
	var stats blockStats
	startTime := mclock.Now()

	report := func(block *block.Block) {
		logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
		stats = blockStats{}
		startTime = mclock.Now()
	}

	var blk *block.Block
	for blk = range stream {
		if blk == nil {
			continue
		}
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
	logger.Debug("enter house keeping")
	defer logger.Debug("leave house keeping")

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
					((err == errParentMissing || err == errBlockTemporaryUnprocessable) && futureBlocks.Contains(newBlock.Header().ParentID())) {
					logger.Debug("future block added", "id", newBlock.Header().ID())
					futureBlocks.Set(newBlock.Header().ID(), newBlock.Block)
				}
			} else if isTrunk {
				n.comm.BroadcastBlock(newBlock.Block)
				logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Block.Header())...)
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
				if isTrunk, err := n.processBlock(block, &stats); err == nil || err == errKnownBlock {
					logger.Debug("future block consumed", "id", block.Header().ID())
					futureBlocks.Remove(block.Header().ID())
					if isTrunk {
						n.comm.BroadcastBlock(block)
					}
				}

				if stats.processed > 0 && i == len(blocks)-1 {
					logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
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
	logger.Debug("enter tx stash loop")
	defer logger.Debug("leave tx stash loop")

	db, err := leveldb.OpenFile(n.txStashPath, nil)
	if err != nil {
		logger.Error("create tx stash", "err", err)
		return
	}
	defer db.Close()

	stash := newTxStash(db, 1000)

	{
		txs := stash.LoadAll()
		n.txPool.Fill(txs)
		logger.Debug("loaded txs from stash", "count", len(txs))
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
				logger.Warn("stash tx", "id", txEv.Tx.ID(), "err", err)
			} else {
				logger.Trace("stashed tx", "id", txEv.Tx.ID())
			}
		}
	}
}

// guardBlockProcessing adds lock on block processing and maintains block conflicts.
func (n *Node) guardBlockProcessing(blockNum uint32, process func(conflicts uint32) error) error {
	n.processLock.Lock()
	defer n.processLock.Unlock()

	if blockNum > n.maxBlockNum {
		if blockNum > n.maxBlockNum+1 {
			// the block is surely unprocessable now
			return errBlockTemporaryUnprocessable
		}
		n.maxBlockNum = blockNum
		return process(0)
	}

	conflicts, err := n.repo.ScanConflicts(blockNum)
	if err != nil {
		return err
	}
	return process(conflicts)
}

func (n *Node) processBlock(newBlock *block.Block, stats *blockStats) (bool, error) {
	var isTrunk *bool

	if err := n.guardBlockProcessing(newBlock.Header().Number(), func(conflicts uint32) error {
		// Check whether the block was already there.
		// It can be skipped if no conflicts.
		if conflicts > 0 {
			if _, err := n.repo.GetBlockSummary(newBlock.Header().ID()); err != nil {
				if !n.repo.IsNotFound(err) {
					return err
				}
			} else {
				return errKnownBlock
			}
		}
		parentSummary, err := n.repo.GetBlockSummary(newBlock.Header().ParentID())
		if err != nil {
			if !n.repo.IsNotFound(err) {
				return err
			}
			return errParentMissing
		}

		var (
			startTime = mclock.Now()
			oldBest   = n.repo.BestBlockSummary()
		)

		if ok, err := n.bft.Accepts(newBlock.Header().ParentID()); err != nil {
			return errors.Wrap(err, "bft accepts")
		} else if !ok {
			return errBFTRejected
		}

		// process the new block
		stage, receipts, err := n.cons.Process(parentSummary, newBlock, uint64(time.Now().Unix()), conflicts)
		if err != nil {
			return err
		}

		var becomeNewBest bool
		// let bft engine decide the best block after fork FINALITY
		if newBlock.Header().Number() >= n.forkConfig.FINALITY && oldBest.Header.Number() >= n.forkConfig.FINALITY {
			becomeNewBest, err = n.bft.Select(newBlock.Header())
			if err != nil {
				return errors.Wrap(err, "bft select")
			}
		} else {
			becomeNewBest = newBlock.Header().BetterThan(oldBest.Header)
		}
		logEnabled := becomeNewBest && !n.skipLogs && !n.logDBFailed
		isTrunk = &becomeNewBest

		execElapsed := mclock.Now() - startTime

		// write logs
		if logEnabled {
			if err := n.writeLogs(newBlock, receipts, oldBest.Header.ID()); err != nil {
				return errors.Wrap(err, "write logs")
			}
		}

		// commit produced states
		if _, err := stage.Commit(); err != nil {
			return errors.Wrap(err, "commit state")
		}

		// sync the log-writing task
		if logEnabled {
			if err := n.logWorker.Sync(); err != nil {
				log.Warn("failed to write logs", "err", err)
				n.logDBFailed = true
			}
		}

		// add the new block into repository
		if err := n.repo.AddBlock(newBlock, receipts, conflicts, becomeNewBest); err != nil {
			return errors.Wrap(err, "add block")
		}

		// commit block in bft engine
		if newBlock.Header().Number() >= n.forkConfig.FINALITY {
			if err := n.bft.CommitBlock(newBlock.Header(), false); err != nil {
				return errors.Wrap(err, "bft commits")
			}
		}

		realElapsed := mclock.Now() - startTime

		if becomeNewBest {
			n.processFork(newBlock, oldBest.Header.ID())
		}

		commitElapsed := mclock.Now() - startTime - execElapsed

		if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(realElapsed)); updated {
			logger.Trace("bandwidth updated", "gps", v)
		}
		stats.UpdateProcessed(1, len(receipts), execElapsed, commitElapsed, realElapsed, newBlock.Header().GasUsed())

		metricBlockProcessedTxs().SetWithLabel(int64(len(receipts)), map[string]string{"type": "received"})
		metricBlockProcessedGas().SetWithLabel(int64(newBlock.Header().GasUsed()), map[string]string{"type": "received"})
		metricBlockProcessedDuration().Observe(time.Duration(realElapsed).Milliseconds())
		return nil
	}); err != nil {
		switch {
		case err == errKnownBlock:
			stats.UpdateIgnored(1)
			return false, nil
		case consensus.IsFutureBlock(err) || err == errParentMissing || err == errBlockTemporaryUnprocessable:
			stats.UpdateQueued(1)
		case err == errBFTRejected:
			// TODO: capture metrics
			logger.Debug(fmt.Sprintf("block rejected by BFT engine\n%v\n", newBlock.Header()))
		case consensus.IsCritical(err):
			msg := fmt.Sprintf(`failed to process block due to consensus failure\n%v\n`, newBlock.Header())
			logger.Error(msg, "err", err)
		default:
			logger.Error("failed to process block", "err", err)
		}
		metricBlockProcessedCount().AddWithLabel(1, map[string]string{"type": "received", "success": "false"})
		return false, err
	}
	metricBlockProcessedCount().AddWithLabel(1, map[string]string{"type": "received", "success": "true"})
	return *isTrunk, nil
}

func (n *Node) writeLogs(newBlock *block.Block, newReceipts tx.Receipts, oldBestBlockID thor.Bytes32) (err error) {
	var w *logdb.Writer
	if int64(newBlock.Header().Timestamp()) < time.Now().Unix()-24*3600 {
		// turn off log sync to quickly catch up
		w = n.logDB.NewWriterSyncOff()
	} else {
		w = n.logDB.NewWriter()
	}
	defer func() {
		if err != nil {
			n.logWorker.Run(w.Rollback)
		}
	}()

	oldTrunk := n.repo.NewChain(oldBestBlockID)
	newTrunk := n.repo.NewChain(newBlock.Header().ParentID())

	oldBranch, err := oldTrunk.Exclude(newTrunk)
	if err != nil {
		return err
	}

	// to clear logs on the old branch.
	if len(oldBranch) > 0 {
		n.logWorker.Run(func() error {
			return w.Truncate(block.Number(oldBranch[0]))
		})
	}

	newBranch, err := newTrunk.Exclude(oldTrunk)
	if err != nil {
		return err
	}
	// write logs on the new branch.
	for _, id := range newBranch {
		block, err := n.repo.GetBlock(id)
		if err != nil {
			return err
		}
		receipts, err := n.repo.GetBlockReceipts(id)
		if err != nil {
			return err
		}
		n.logWorker.Run(func() error {
			return w.Write(block, receipts)
		})
	}

	n.logWorker.Run(func() error {
		if err := w.Write(newBlock, newReceipts); err != nil {
			return err
		}
		return w.Commit()
	})
	return nil
}

func (n *Node) processFork(newBlock *block.Block, oldBestBlockID thor.Bytes32) {
	oldTrunk := n.repo.NewChain(oldBestBlockID)
	newTrunk := n.repo.NewChain(newBlock.Header().ParentID())

	sideIDs, err := oldTrunk.Exclude(newTrunk)
	if err != nil {
		logger.Warn("failed to process fork", "err", err)
		return
	}

	// Set the gauge metric to the size of the fork (0 if there are no forks)
	metricChainForkSize().Set(int64(len(sideIDs)))

	if len(sideIDs) == 0 {
		return
	}

	if n := len(sideIDs); n >= 2 {
		metricChainForkCount().Add(1)
		logger.Warn(fmt.Sprintf(
			`⑂⑂⑂⑂⑂⑂⑂⑂ FORK HAPPENED ⑂⑂⑂⑂⑂⑂⑂⑂
side-chain:   %v  %v`,
			n, sideIDs[n-1]))
	}

	for _, id := range sideIDs {
		b, err := n.repo.GetBlock(id)
		if err != nil {
			logger.Warn("failed to process fork", "err", err)
			return
		}
		for _, tx := range b.Transactions() {
			if err := n.txPool.Add(tx); err != nil {
				logger.Debug("failed to add tx to tx pool", "err", err, "id", tx.ID())
			}
		}
	}
}

func checkClockOffset() {
	resp, err := ntp.Query("pool.ntp.org")
	if err != nil {
		logger.Debug("failed to access NTP", "err", err)
		return
	}
	if resp.ClockOffset > time.Duration(thor.BlockInterval)*time.Second/2 {
		logger.Warn("clock offset detected", "offset", common.PrettyDuration(resp.ClockOffset))
	}
}
