// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/pkg/errors"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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
		// authorized bool
		flow   *packer.Flow
		err    error
		ticker = time.NewTicker(time.Second)

		// summary      *block.Summary
		// endorsements block.Endorsements
		// txSet        *block.TxSet
	)
	defer ticker.Stop()

	n.packer.SetTargetGasLimit(n.targetGasLimit)

	// newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	// newTxSetCh := make(chan *comm.NewTxSetEvent)
	// newEndorsementCh := make(chan *comm.NewEndorsementEvent)
	// newHeaderCh := make(chan *comm.NewHeaderEvent)

	launchTime := n.chain.GenesisBlock().Header().Timestamp()
	// Starting from 1
	roundNum := (launchTime-uint64(time.Now().Unix()))/thor.BlockInterval + 1
	// Starting from 1
	epochNum := (roundNum-1)/thor.EpochInterval + 1

	_, err = n.cons.Beacon(uint32(epochNum))
	if err != nil {
		panic(struct{}{})
	}
	// roundSeed := n.cons.GetRoundSeed(beacon, uint32(roundNum))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			best := n.chain.BestBlock()
			now := uint64(time.Now().Unix())
			// r := (launchTime-now)/thor.BlockInterval + 1
			// e := (r-1)/thor.EpochInterval + 1

			if flow == nil {
				if flow, err = n.packer.Schedule(best.Header(), now); err != nil {
					continue
				}
			}
			// case bs := <-newBlockSummaryCh:
			// case ed := <-newEndorsementCh:
			// case h := <-newHeaderCh:
			// case ts := <-newTxSetCh:
		}

		// 	if flow.ParentHeader().ID() != best.Header().ID() {
		// 		flow = nil
		// 		log.Debug("re-schedule packer due to new best block")
		// 		continue
		// 	}

		// 	if now+1 >= flow.When() {
		// 		if err := n.pack(flow); err != nil {
		// 			log.Error("failed to pack block", "err", err)
		// 		}
		// 		flow = nil
		// 	}
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

	startTime := mclock.Now()
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

	if n.targetGasLimit == 0 {
		n.packer.SetTargetGasLimit(0)
		if execElapsed > 0 {
			gasUsed := newBlock.Header().GasUsed()
			// calc target gas limit only if gas used above third of gas limit
			if gasUsed > newBlock.Header().GasLimit()/3 {
				targetGasLimit := uint64(math.Log2(float64(newBlock.Header().Number()+1))*float64(thor.TolerableBlockPackingTime)*float64(gasUsed)) / (32 * uint64(execElapsed))
				n.packer.SetTargetGasLimit(targetGasLimit)
				log.Debug("reset target gas limit", "value", targetGasLimit)
			}
		}
	}
	return nil
}
