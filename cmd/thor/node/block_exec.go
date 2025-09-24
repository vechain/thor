// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type blockExecContext struct {
	prevBest   *block.Header
	newBlock   *block.Block
	receipts   tx.Receipts
	stage      *state.Stage
	becomeBest bool
	conflicts  uint32
	stats      *blockStats
	packing    bool
	startTime  mclock.AbsTime
}

// guardBlockProcessing adds lock on block processing and maintains block conflicts.
func (n *Node) guardBlockProcessing(blockNum uint32, process func(conflicts uint32) error) (err error) {
	n.processLock.Lock()
	defer func() {
		n.processLock.Unlock()
	}()

	if blockNum > n.maxBlockNum {
		if blockNum > n.maxBlockNum+1 {
			// the block is surely unprocessable now
			return errBlockTemporaryUnprocessable
		}

		// don't increase maxBlockNum if the block is unprocessable
		if e := process(0); e != nil {
			return e
		}

		n.maxBlockNum = blockNum
		return nil
	}

	conflicts, err := n.repo.ScanConflicts(blockNum)
	if err != nil {
		return err
	}
	return process(conflicts)
}

func (n *Node) processBlock(newBlock *block.Block, stats *blockStats) (bool, error) {
	var isTrunk bool

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

		ctx := &blockExecContext{
			prevBest:  n.repo.BestBlockSummary().Header,
			newBlock:  newBlock,
			conflicts: conflicts,
			startTime: mclock.Now(),
			stats:     stats,
		}

		// reject the block if the parent is conflicting with finalized checkpoint
		if ok, err := n.bft.Accepts(newBlock.Header().ParentID()); err != nil {
			return errors.Wrap(err, "bft accepts")
		} else if !ok {
			return errBFTRejected
		}

		// process the new block
		ctx.stage, ctx.receipts, err = n.cons.Process(parentSummary, newBlock, uint64(time.Now().Unix()), conflicts)
		if err != nil {
			return err
		}

		// let bft engine decide the best block after fork FINALITY
		if newBlock.Header().Number() >= n.forkConfig.FINALITY && ctx.prevBest.Number() >= n.forkConfig.FINALITY {
			ctx.becomeBest, err = n.bft.Select(newBlock.Header())
			if err != nil {
				return errors.Wrap(err, "bft select")
			}
		} else {
			ctx.becomeBest = newBlock.Header().BetterThan(ctx.prevBest)
		}

		if err := n.commitBlock(ctx); err != nil {
			return err
		}

		isTrunk = ctx.becomeBest
		// run post block processing hook when no error
		n.postBlockProcessing(ctx.newBlock, ctx.conflicts)

		return nil
	}); err != nil {
		switch {
		case err == errKnownBlock:
			stats.UpdateIgnored(1)
			return false, nil
		case consensus.IsFutureBlock(err) || err == errParentMissing || err == errBlockTemporaryUnprocessable:
			stats.UpdateQueued(1)
		case err == errBFTRejected:
			metricBlockRejectedCount().Add(1)
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
	return isTrunk, nil
}

func (n *Node) commitBlock(ctx *blockExecContext) error {
	execElapsed := mclock.Now() - ctx.startTime

	logEnabled := ctx.becomeBest && !n.options.SkipLogs && !n.logDBFailed
	// write logs
	if logEnabled {
		if err := n.writeLogs(ctx.newBlock, ctx.receipts, ctx.prevBest.ID()); err != nil {
			return errors.Wrap(err, "write logs")
		}
	}

	// commit the states
	if _, err := ctx.stage.Commit(); err != nil {
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
	if err := n.repo.AddBlock(ctx.newBlock, ctx.receipts, ctx.conflicts, ctx.becomeBest); err != nil {
		return errors.Wrap(err, "add block")
	}

	// commit block in bft engine
	if ctx.newBlock.Header().Number() >= n.forkConfig.FINALITY {
		if err := n.bft.CommitBlock(ctx.newBlock.Header(), ctx.packing); err != nil {
			return errors.Wrap(err, "bft commits")
		}
	}

	realElapsed := mclock.Now() - ctx.startTime

	if ctx.becomeBest {
		n.processFork(ctx.newBlock, ctx.prevBest.ID())
	}

	commitElapsed := mclock.Now() - ctx.startTime - execElapsed

	if v, updated := n.bandwidth.Update(ctx.newBlock.Header(), time.Duration(realElapsed)); updated {
		metricNodeGasPerSecond().Set(int64(v))
		logger.Trace("bandwidth updated", "gps", v)
	}

	if ctx.stats != nil {
		ctx.stats.UpdateProcessed(1, len(ctx.receipts), execElapsed, commitElapsed, realElapsed, ctx.newBlock.Header().GasUsed())
	}

	blockType := "received"
	if ctx.packing {
		blockType = "proposed"
	}
	metricBlockProcessedTxs().SetWithLabel(int64(len(ctx.receipts)), map[string]string{"type": blockType})
	metricBlockProcessedGas().SetWithLabel(int64(ctx.newBlock.Header().GasUsed()), map[string]string{"type": blockType})
	metricBlockProcessedDuration().Observe(time.Duration(realElapsed).Milliseconds())

	return nil
}

func (n *Node) postBlockProcessing(newBlock *block.Block, conflicts uint32) {
	if err := func() error {
		// print welcome info
		if n.initialSynced {
			blockNum := newBlock.Header().Number()
			blockID := newBlock.Header().ID()
			if needPrintWelcomeInfo() &&
				blockNum >= n.forkConfig.HAYABUSA+thor.HayabusaTP() &&
				// if transition period are set to 0, transition will attempt to happen on every block
				(thor.HayabusaTP() == 0 || (blockNum-n.forkConfig.HAYABUSA)%thor.HayabusaTP() == 0) {
				summary, err := n.repo.GetBlockSummary(blockID)
				if err != nil {
					return err
				}
				active, err := builtin.Staker.Native(n.stater.NewState(summary.Root())).IsPoSActive()
				if err != nil {
					return nil
				}
				if active {
					printWelcomeInfo()
				}
			}
		}
		// scan for double signing, when exiting block at the same height are more than one
		if conflicts > 0 {
			newSigner, err := newBlock.Header().Signer()
			if err != nil {
				return err
			}

			blockIDs, err := n.repo.GetConflicts(newBlock.Header().Number())
			if err != nil {
				return err
			}

			for _, blockID := range blockIDs {
				// skip the new block
				if blockID == newBlock.Header().ID() {
					continue
				}

				// iter over conflicting blocks
				conflictBlock, err := n.repo.GetBlock(blockID)
				if err != nil {
					return err
				}
				// logic to verify that the blocks are different and from the same signer
				existingSigner, err := conflictBlock.Header().Signer()
				if err != nil {
					return err
				}
				if existingSigner == newSigner {
					log.Warn("Double signing", "block", shortID(newBlock.Header().ID()), "previous", shortID(blockID), "signer", existingSigner)
					metricDoubleSignedBlocks().AddWithLabel(1, map[string]string{"signer": existingSigner.String()})
				}
			}
		}

		return nil
	}(); err != nil {
		logger.Warn("failed to run post process hook", "err", err)
	}
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

	metricChainForkCount().Add(int64(len(sideIDs)))

	if len(sideIDs) == 0 {
		return
	}

	if n := len(sideIDs); n >= 2 {
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
