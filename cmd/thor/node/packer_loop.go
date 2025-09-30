// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// gasLimitSoftLimit is the soft limit of the adaptive block gaslimit.
const gasLimitSoftLimit uint64 = 40_000_000

func (n *Node) packerLoop(ctx context.Context) {
	logger.Debug("enter packer loop")
	defer logger.Debug("leave packer loop")

	logger.Info("waiting for synchronization...")
	select {
	case <-ctx.Done():
		return
	case <-n.comm.Synced():
		n.syncConfig.completingSync = true
		err := n.completeSync()
		n.syncConfig.completingSync = false
		close(n.syncConfig.syncCompleteCh)
		if err != nil {
			<-ctx.Done()
			return
		}
	}
	n.syncConfig.initialSynced = true
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
				if err := n.doPack(flow); err != nil {
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

func (n *Node) doPack(flow *packer.Flow) error {
	err := n.guardBlockProcessing(flow.Number(), func(conflicts uint32) error {
		return n.proposeAndCommit(flow, conflicts)
	})
	updatePackMetrics(err == nil)

	return err
}

func (n *Node) proposeAndCommit(flow *packer.Flow, conflicts uint32) (err error) {
	var txsToRemove []*tx.Transaction
	defer func() {
		if err == nil {
			cleanupTransactions(txsToRemove, n.txPool)
		}
	}()

	ctx := &blockExecContext{
		prevBest:   n.repo.BestBlockSummary().Header,
		conflicts:  conflicts,
		startTime:  mclock.Now(),
		stats:      &blockStats{},
		packing:    true,
		becomeBest: true,
	}

	txs := n.txPool.Executables()
	// adopt txs
	for _, tx := range txs {
		if err := flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			txsToRemove = append(txsToRemove, tx)
		}
	}

	var shouldVote bool
	if flow.Number() >= n.forkConfig.FINALITY {
		shouldVote, err = n.bft.ShouldVote(flow.ParentHeader().ID())
		if err != nil {
			return errors.Wrap(err, "get vote")
		}
	}

	// pack the new block
	ctx.newBlock, ctx.stage, ctx.receipts, err = flow.Pack(n.master.PrivateKey, conflicts, shouldVote)
	if err != nil {
		return errors.Wrap(err, "failed to pack block")
	}

	err = n.commitBlock(ctx)
	if err != nil {
		return err
	}

	n.comm.BroadcastBlock(ctx.newBlock)
	logger.Info("📦 new block packed", ctx.stats.LogContext(ctx.newBlock.Header())...)
	n.postBlockProcessing(ctx.newBlock, ctx.conflicts)

	return nil
}

func cleanupTransactions(txsToRemove []*tx.Transaction, txPool *txpool.TxPool) {
	for _, tx := range txsToRemove {
		txPool.Remove(tx.Hash(), tx.ID())
	}
}

func updatePackMetrics(success bool) {
	successLabel := "false"
	if success {
		successLabel = "true"
	}
	metricBlockProcessedCount().AddWithLabel(1, map[string]string{"type": "proposed", "success": successLabel})
}

func (n *Node) completeSync() error {
	writer := n.logDB.NewWriter()
	logger.Info("Sync done, creating indexes")
	err := writer.ExecuteJournalWalModeSwitch()
	if err != nil {
		logger.Error("Error while switching journal mode", "err", err)
		return err
	}
	err = writer.CreateIndexes()
	if err != nil {
		logger.Error("Error while creating indexes", "err", err)
		return err
	}
	logger.Info("Sync done, indexes created, journal mode switched")
	return nil
}
