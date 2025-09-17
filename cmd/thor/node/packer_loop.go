// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"time"

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

		base := now
		// a block proposer will be given higher priority in the range of (slotTime, slotTime+2*thor.BlockInterval)
		// and here we left at maximum 3 second as buffer for packing and broadcasting the block
		buff := min(thor.BlockInterval/2, uint64(3))
		parentTime := n.repo.BestBlockSummary().Header.Timestamp()
		// if now is in the prioritized window, use the optimal timestamp as base to schedule next time slot
		if now > parentTime && now < parentTime+3*thor.BlockInterval-buff {
			base = parentTime + thor.BlockInterval
		}
		// otherwise, use now as base
		flow, err := n.packer.Schedule(n.repo.BestBlockSummary(), base)
		if err != nil {
			if authorized {
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
		logger.Debug("scheduled to pack block", "after", time.Duration(flow.When()-now)*time.Second)

		for {
			if uint64(time.Now().Unix())+thor.BlockInterval/2 > flow.When() {
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

func (n *Node) pack(flow *packer.Flow) (err error) {
	txs := n.txPool.Executables()
	var txsToRemove []*tx.Transaction
	defer func() {
		if err == nil {
			for _, tx := range txsToRemove {
				n.txPool.Remove(tx.Hash(), tx.ID())
			}
			metricBlockProcessedCount().AddWithLabel(1, map[string]string{"type": "proposed", "success": "true"})
		} else {
			metricBlockProcessedCount().AddWithLabel(1, map[string]string{"type": "proposed", "success": "false"})
		}
	}()

	return n.guardBlockProcessing(flow.Number(), func(conflicts uint32) error {
		var (
			startTime  = mclock.Now()
			logEnabled = !n.options.SkipLogs && !n.logDBFailed
			oldBest    = n.repo.BestBlockSummary()
		)

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
			var err error
			shouldVote, err = n.bft.ShouldVote(flow.ParentHeader().ID())
			if err != nil {
				return errors.Wrap(err, "get vote")
			}
		}

		// pack the new block
		newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey, conflicts, shouldVote)
		if err != nil {
			return errors.Wrap(err, "failed to pack block")
		}
		execElapsed := mclock.Now() - startTime

		// write logs
		if logEnabled {
			if err := n.writeLogs(newBlock, receipts, oldBest.Header.ID()); err != nil {
				return errors.Wrap(err, "write logs")
			}
		}

		// commit the state
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
		if err := n.repo.AddBlock(newBlock, receipts, conflicts, true); err != nil {
			return errors.Wrap(err, "add block")
		}

		// commit block in bft engine
		if newBlock.Header().Number() >= n.forkConfig.FINALITY {
			if err := n.bft.CommitBlock(newBlock.Header(), true); err != nil {
				return errors.Wrap(err, "bft commits")
			}
		}
		realElapsed := mclock.Now() - startTime

		n.processFork(newBlock, oldBest.Header.ID())
		commitElapsed := mclock.Now() - startTime - execElapsed

		n.comm.BroadcastBlock(newBlock)
		logger.Info("📦 new block packed",
			"txs", len(receipts),
			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
			"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
			"id", shortID(newBlock.Header().ID()),
		)

		if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(realElapsed)); updated {
			logger.Trace("bandwidth updated", "gps", v)
		}

		metricBlockProcessedTxs().SetWithLabel(int64(len(receipts)), map[string]string{"type": "proposed"})
		metricBlockProcessedGas().SetWithLabel(int64(newBlock.Header().GasUsed()), map[string]string{"type": "proposed"})
		metricBlockProcessedDuration().Observe(time.Duration(realElapsed).Milliseconds())
		return nil
	})
}
