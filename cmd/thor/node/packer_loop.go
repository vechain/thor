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
)

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
		flow       *packer.Flow
		err        error
		ticker     = time.NewTicker(time.Second)
	)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		best := n.chain.BestBlock()
		now := uint64(time.Now().Unix())

		if flow == nil {
			if flow, err = n.packer.Schedule(best.Header(), now); err != nil {
				if authorized {
					authorized = false
					log.Warn("unable to pack block", "err", err)
				}
				continue
			}
			if !authorized {
				authorized = true
				log.Info("prepared to pack block")
			}
			log.Debug("scheduled to pack block", "after", time.Duration(flow.When()-now)*time.Second)
			continue
		}

		if flow.ParentHeader().ID() != best.Header().ID() {
			flow = nil
			log.Debug("re-schedule packer due to new best block")
			continue
		}

		if now+1 >= flow.When() {
			if err := n.pack(flow); err != nil {
				log.Error("failed to pack block", "err", err)
			}
			flow = nil
		}
	}
}

func (n *Node) pack(flow *packer.Flow) error {
	txs := n.txPool.Executables()
	var txsToRemove []thor.Bytes32
	defer func() {
		for _, id := range txsToRemove {
			n.txPool.Remove(id)
		}
	}()

	startTime := mclock.Now()
	for _, tx := range txs {
		if err := flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			txsToRemove = append(txsToRemove, tx.ID())
		}
	}

	newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
	if err != nil {
		return err
	}
	execElapsed := mclock.Now() - startTime

	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	fork, err := n.commitBlock(newBlock, receipts)
	if err != nil {
		return errors.WithMessage(err, "commit block")
	}
	commitElapsed := mclock.Now() - startTime - execElapsed

	n.processFork(fork)

	if len(fork.Trunk) > 0 {
		n.comm.BroadcastBlock(newBlock)
		log.Info("📦 new block packed",
			"txs", len(receipts),
			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
			"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
			"id", shortID(newBlock.Header().ID()),
		)
	}

	n.packer.SetTargetGasLimit(0)
	if execElapsed > 0 {
		gasUsed := newBlock.Header().GasUsed()
		// calc target gas limit only if gas used above third of gas limit
		if gasUsed > newBlock.Header().GasLimit()/3 {
			targetGasLimit := uint64(thor.TolerableBlockPackingTime) * gasUsed / uint64(execElapsed)
			n.packer.SetTargetGasLimit(targetGasLimit)
			log.Debug("reset target gas limit", "value", targetGasLimit)
		}
	}
	return nil
}
