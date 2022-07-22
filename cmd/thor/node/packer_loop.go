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
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// gasLimitSoftLimit is the soft limit of the adaptive block gaslimit.
const gasLimitSoftLimit uint64 = 30_000_000

func (n *Node) packerLoop(ctx context.Context) {
	log.Debug("enter packer loop")
	defer log.Debug("leave packer loop")

	log.Info("waiting for synchronization...")
	select {
	case <-ctx.Done():
		return
	case <-n.comm.Synced():
	}
	log.Info("synchronization process done")

	var (
		authorized bool
		ticker     = n.repo.NewTicker()
	)

	n.packer.SetTargetGasLimit(n.targetGasLimit)

	for {
		now := uint64(time.Now().Unix())

		if n.targetGasLimit == 0 {
			// no preset, use suggested
			suggested := n.bandwidth.SuggestGasLimit()
			// apply soft limit in adaptive mode
			if suggested > gasLimitSoftLimit {
				suggested = gasLimitSoftLimit
			}
			n.packer.SetTargetGasLimit(suggested)
		}

		flow, err := n.packer.Schedule(n.repo.BestBlockSummary(), now)
		if err != nil {
			if authorized {
				authorized = false
				log.Warn("unable to pack block", "err", err)
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
			log.Info("prepared to pack block")
		}
		log.Debug("scheduled to pack block", "after", time.Duration(flow.When()-now)*time.Second)

		for {
			if uint64(time.Now().Unix())+thor.BlockInterval/2 > flow.When() {
				// time to pack block
				// blockInterval/2 early to allow more time for processing txs
				if err := n.pack(flow); err != nil {
					log.Error("failed to pack block", "err", err)
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
					log.Debug("re-schedule packer due to new best block")
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
	defer func() {
		for _, tx := range txsToRemove {
			n.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	return n.guardBlockProcessing(flow.Number(), func(conflicts uint32) error {
		var (
			startTime  = mclock.Now()
			logEnabled = !n.skipLogs && !n.logDBFailed
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
			if n.writeLogs(newBlock, receipts, oldBest.Header.ID()); err != nil {
				return errors.Wrap(err, "write logs")
			}
		}

		// commit the state
		if _, err := stage.Commit(); err != nil {
			return errors.Wrap(err, "commit state")
		}

		// add the new block into repository
		if err := n.repo.AddBlock(newBlock, receipts, conflicts); err != nil {
			return errors.Wrap(err, "add block")
		}

		// commit block in bft engine
		if newBlock.Header().Number() >= n.forkConfig.FINALITY {
			if err := n.bft.CommitBlock(newBlock.Header(), true); err != nil {
				return errors.Wrap(err, "bft commits")
			}
		}
		realElapsed := mclock.Now() - startTime

		// sync the log-writing task
		if logEnabled {
			if err := n.logWorker.Sync(); err != nil {
				log.Warn("failed to write logs", "err", err)
				n.logDBFailed = true
			}
		}

		if err := n.repo.SetBestBlockID(newBlock.Header().ID()); err != nil {
			return err
		}

		n.processFork(newBlock, oldBest.Header.ID())
		commitElapsed := mclock.Now() - startTime - execElapsed

		n.comm.BroadcastBlock(newBlock)
		log.Info("📦 new block packed",
			"txs", len(receipts),
			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
			"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
			"id", shortID(newBlock.Header().ID()),
		)

		if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(realElapsed)); updated {
			log.Debug("bandwidth updated", "gps", v)
		}
		return nil
	})
}
