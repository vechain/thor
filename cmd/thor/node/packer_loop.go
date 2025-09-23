// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// gasLimitSoftLimit is the soft limit of the adaptive block gaslimit.
const gasLimitSoftLimit uint64 = 40_000_000

type packContext struct {
	flow       *packer.Flow
	conflicts  uint32
	startTime  mclock.AbsTime
	logEnabled bool
	oldBest    *chain.BlockSummary
}

func (n *Node) packerLoop(ctx context.Context) {
	logger.Debug("enter packer loop")
	defer logger.Debug("leave packer loop")

	logger.Info("waiting for synchronization...")
	select {
	case <-ctx.Done():
		return
	case <-n.comm.Synced():
	}
	n.initialSynced = true
	logger.Info("synchronization process done")

	var (
		authorized bool
		ticker     = n.repo.NewTicker()
	)

	n.packer.SetTargetGasLimit(n.options.TargetGasLimit)

	for {
		now := uint64(time.Now().Unix())

		if n.options.TargetGasLimit == 0 {
			// no preset, use suggested
			// apply soft limit in adaptive mode
			suggested := min(n.bandwidth.SuggestGasLimit(), gasLimitSoftLimit)
			n.packer.SetTargetGasLimit(suggested)
		}

		flow, pos, err := n.packer.Schedule(n.repo.BestBlockSummary(), now)
		if err != nil {
			if !packer.IsSchedulingError(err) && authorized {
				authorized = false
				logger.Warn("unable to pack block", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				continue
			}
		}

		if !authorized {
			authorized = true
			logger.Info("prepared to pack block")
		}
		logger.Info("scheduled to pack block", "after", time.Duration(flow.When()-now)*time.Second, "score", flow.TotalScore(), "pos", pos)

		for {
			if uint64(time.Now().Unix())+thor.BlockInterval()/2 > flow.When() {
				// time to pack block
				// blockInterval/2 early to allow more time for processing txs
				if err := n.pack(flow); err != nil {
					logger.Error("failed to pack block", "err", err)
				}
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				best := n.repo.BestBlockSummary().Header
				/*  re-schedule regarding the following two conditions:
				1. parent block needs to update and the new best is not proposed by the same one
				2. best block is better than the block to be proposed
				*/

				s1, _ := best.Signer()
				s2, _ := flow.ParentHeader().Signer()

				if (best.Number() == flow.ParentHeader().Number() && s1 != s2) ||
					best.TotalScore() > flow.TotalScore() {
					logger.Debug("re-schedule packer due to new best block")
					goto RE_SCHEDULE
				}
			}
		}
	RE_SCHEDULE:
	}
}

func (n *Node) pack(flow *packer.Flow) error {
	txs := n.txPool.Executables()
	var txsToRemove []*tx.Transaction

	err := n.processBlockWithGuard(flow, txs, &txsToRemove)

	if err == nil {
		n.cleanupTransactions(txsToRemove)
	}

	n.updatePackMetrics(err == nil)

	return err
}

func (n *Node) processBlockWithGuard(flow *packer.Flow, txs []*tx.Transaction, txsToRemove *[]*tx.Transaction) error {
	return n.guardBlockProcessing(flow.Number(), func(conflicts uint32) (thor.Bytes32, error) {
		return n.processBlockWithConflicts(flow, txs, txsToRemove, conflicts)
	})
}

func (n *Node) processBlockWithConflicts(flow *packer.Flow, txs []*tx.Transaction, txsToRemove *[]*tx.Transaction, conflicts uint32) (thor.Bytes32, error) {
	ctx := &packContext{
		flow:       flow,
		conflicts:  conflicts,
		startTime:  mclock.Now(),
		logEnabled: !n.options.SkipLogs && !n.logDBFailed,
		oldBest:    n.repo.BestBlockSummary(),
	}

	if err := n.processTransactions(ctx, txs, txsToRemove); err != nil {
		return thor.Bytes32{}, err
	}

	shouldVote, err := n.determineVotingRequirement(flow)
	if err != nil {
		return thor.Bytes32{}, errors.Wrap(err, "get vote")
	}

	newBlock, stage, receipts, err := n.packBlock(flow, conflicts, shouldVote)
	if err != nil {
		return thor.Bytes32{}, errors.Wrap(err, "failed to pack block")
	}

	return newBlock.Header().ID(), n.processPackedBlock(ctx, newBlock, stage, receipts)
}

func (n *Node) cleanupTransactions(txsToRemove []*tx.Transaction) {
	for _, tx := range txsToRemove {
		n.txPool.Remove(tx.Hash(), tx.ID())
	}
}

func (n *Node) updatePackMetrics(success bool) {
	successLabel := "false"
	if success {
		successLabel = "true"
	}
	metricBlockProcessedCount().AddWithLabel(1, map[string]string{"type": "proposed", "success": successLabel})
}

func (n *Node) processTransactions(ctx *packContext, txs []*tx.Transaction, txsToRemove *[]*tx.Transaction) error {
	for _, tx := range txs {
		if err := ctx.flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			*txsToRemove = append(*txsToRemove, tx)
		}
	}
	return nil
}

func (n *Node) determineVotingRequirement(flow *packer.Flow) (bool, error) {
	if flow.Number() >= n.forkConfig.FINALITY {
		return n.bft.ShouldVote(flow.ParentHeader().ID())
	}
	return false, nil
}

func (n *Node) packBlock(flow *packer.Flow, conflicts uint32, shouldVote bool) (*block.Block, *state.Stage, tx.Receipts, error) {
	return flow.Pack(n.master.PrivateKey, conflicts, shouldVote)
}

func (n *Node) processPackedBlock(ctx *packContext, newBlock *block.Block, stage *state.Stage, receipts tx.Receipts) error {
	execElapsed := mclock.Now() - ctx.startTime

	if err := n.writeLogsIfEnabled(ctx, newBlock, receipts); err != nil {
		return errors.Wrap(err, "write logs")
	}

	if err := n.commitState(stage); err != nil {
		return errors.Wrap(err, "commit state")
	}

	if err := n.syncLogWorker(ctx); err != nil {
		return err
	}

	if err := n.addBlockToRepository(newBlock, receipts, ctx.conflicts); err != nil {
		return errors.Wrap(err, "add block")
	}

	if err := n.commitToBFT(newBlock); err != nil {
		return errors.Wrap(err, "bft commits")
	}

	n.finalizeAndBroadcast(ctx, newBlock, receipts, execElapsed)

	return nil
}

func (n *Node) writeLogsIfEnabled(ctx *packContext, newBlock *block.Block, receipts tx.Receipts) error {
	if ctx.logEnabled {
		return n.writeLogs(newBlock, receipts, ctx.oldBest.Header.ID())
	}
	return nil
}

func (n *Node) commitState(stage *state.Stage) error {
	_, err := stage.Commit()
	return err
}

func (n *Node) syncLogWorker(ctx *packContext) error {
	if ctx.logEnabled {
		if err := n.logWorker.Sync(); err != nil {
			log.Warn("failed to write logs", "err", err)
			n.logDBFailed = true
		}
	}
	return nil
}

func (n *Node) addBlockToRepository(newBlock *block.Block, receipts tx.Receipts, conflicts uint32) error {
	return n.repo.AddBlock(newBlock, receipts, conflicts, true)
}

func (n *Node) commitToBFT(newBlock *block.Block) error {
	if newBlock.Header().Number() >= n.forkConfig.FINALITY {
		return n.bft.CommitBlock(newBlock.Header(), true)
	}
	return nil
}

func (n *Node) finalizeAndBroadcast(ctx *packContext, newBlock *block.Block, receipts tx.Receipts, execElapsed mclock.AbsTime) {
	realElapsed := mclock.Now() - ctx.startTime
	commitElapsed := realElapsed - execElapsed

	n.processFork(newBlock, ctx.oldBest.Header.ID())
	n.comm.BroadcastBlock(newBlock)

	n.logBlockPacked(newBlock, receipts, execElapsed, commitElapsed)
	n.updateMetrics(newBlock, receipts, realElapsed)
}

func (n *Node) logBlockPacked(newBlock *block.Block, receipts tx.Receipts, execElapsed, commitElapsed mclock.AbsTime) {
	logger.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
		"id", shortID(newBlock.Header().ID()),
	)
}

func (n *Node) updateMetrics(newBlock *block.Block, receipts tx.Receipts, realElapsed mclock.AbsTime) {
	if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(realElapsed)); updated {
		logger.Trace("bandwidth updated", "gps", v)
	}

	metricBlockProcessedTxs().SetWithLabel(int64(len(receipts)), map[string]string{"type": "proposed"})
	metricBlockProcessedGas().SetWithLabel(int64(newBlock.Header().GasUsed()), map[string]string{"type": "proposed"})
	metricBlockProcessedDuration().Observe(time.Duration(realElapsed).Milliseconds())
}
