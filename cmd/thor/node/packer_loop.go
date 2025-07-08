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

		bb := n.repo.BestBlockSummary()
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
			if uint64(time.Now().Unix())+thor.BlockInterval/2 > flow.When() {
				// time to pack block
				// blockInterval/2 early to allow more time for processing txs
				if err := n.pack(flow, false); err != nil {
					logger.Error("failed to pack block", "err", err)
				}

				if flow.Number() == 10 {
					flow, pos, err = n.packer.Schedule(bb, now)
					if err != nil {
						logger.Error("failed to initalize second flow", "err", err)
					}
					if err := n.pack(flow, true); err != nil {
						logger.Error("failed to pack block", "err", err)
					}
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

func (n *Node) pack(flow *packer.Flow, duplicate bool) (err error) {
	txs := n.txPool.Executables()
	println("Number of txs ====1", len(txs))
	if flow.Number() == uint32(10) && duplicate {
		txs = make(tx.Transactions, 0)
	}
	println("Number of txs ====2", len(txs))
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

	return n.guardBlockProcessing(flow.Number(), func(conflicts [][]byte) error {
		var (
			startTime  = mclock.Now()
			logEnabled = !n.options.SkipLogs && !n.logDBFailed
			oldBest    = n.repo.BestBlockSummary()
		)

		// adopt txs
		for _, tx := range txs {
			println("adopting tx for block", flow.Number())
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
		println("should vote", shouldVote)
		newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey, uint32(len(conflicts)), false)
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

		println("this is number of conflicts", uint32(len(conflicts)))
		// add the new block into repository
		if err := n.repo.AddBlock(newBlock, receipts, uint32(len(conflicts)), true); err != nil {
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

		if newBlock.Header().Number() == n.forkConfig.GALACTICA {
			fmt.Println(GalacticaASCIIArt)
		}

		n.comm.BroadcastBlock(newBlock)
		logger.Info("📦 new block packed",
			"txs", len(receipts),
			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
			"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
			"id", shortID(newBlock.Header().ID()),
		)
		// TODO: log to be removed when fork is stable
		if newBlock.Header().Number()+1 == n.forkConfig.GALACTICA {
			logger.Info("Last block before Galactica fork activates!")
		} else if newBlock.Header().Number() == n.forkConfig.GALACTICA {
			logger.Info("Galactica fork activated!")
		}

		if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(realElapsed)); updated {
			logger.Trace("bandwidth updated", "gps", v)
		}

		metricBlockProcessedTxs().SetWithLabel(int64(len(receipts)), map[string]string{"type": "proposed"})
		metricBlockProcessedGas().SetWithLabel(int64(newBlock.Header().GasUsed()), map[string]string{"type": "proposed"})
		metricBlockProcessedDuration().Observe(time.Duration(realElapsed).Milliseconds())
		return nil
	})
}
