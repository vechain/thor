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
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
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

		bs  *block.Summary
		eds map[thor.Bytes32]*block.Endorsement
		// ts  *block.TxSet
	)
	defer ticker.Stop()

	n.packer.SetTargetGasLimit(n.targetGasLimit)

	// newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	// newTxSetCh := make(chan *comm.NewTxSetEvent)
	newEndorsementCh := make(chan *comm.NewEndorsementEvent)
	// newHeaderCh := make(chan *comm.NewHeaderEvent)

	round, _ := n.cons.RoundNumber(uint64(time.Now().Unix()))
	epoch, _ := n.cons.EpochNumber(uint64(time.Now().Unix()))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if flow != nil {
				continue
			}

			best := n.chain.BestBlock()
			now := uint64(time.Now().Unix())
			if flow, err = n.packer.Schedule(best.Header(), now); err != nil {
				continue
			}

			r, _ := n.cons.RoundNumber(uint64(time.Now().Unix()))
			e, _ := n.cons.EpochNumber(uint64(time.Now().Unix()))
			if r > round {
				round = r
				if e > epoch {
					epoch = e
				}
			}

		// case bs := <-newBlockSummaryCh:
		case ed := <-newEndorsementCh:
			if bs == nil {
				continue
			}

			if !ed.BlockSummary().IsEqual(bs) {
				continue
			}

			if _, ok := eds[ed.Endorsement.SigningHash()]; ok {
				continue
			}

			// if n.cons.ValidateEndorsement(ed.Endorsement) != nil {
			// 	continue
			// }

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

func (n *Node) packTxSetAndBlockSummary(done chan struct{}, flow *packer.Flow, txs tx.Transactions) error {
	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			n.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()
	for _, tx := range txs {
		select {
		case <-done:
			// log.Debug("Leave tx adopting loop", "Iter", i)
			break
		default:
		}
		// log.Debug("Adopting tx", "txid", tx.ID())
		err := flow.Adopt(tx)
		switch {
		case packer.IsGasLimitReached(err):
			break
		case packer.IsTxNotAdoptableNow(err):
			continue
		default:
			txsToRemove = append(txsToRemove, tx)
		}
	}

	err := flow.PackTxSetAndBlockSummary(n.master.PrivateKey)
	if err != nil {
		return err
	}

	return nil

	// ts := block.NewTxSet(flow.Txs())
	// sig, err := crypto.Sign(ts.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	// if err != nil {
	// 	return nil, nil, err
	// }
	// ts = ts.WithSignature(sig)

	// parent := best.Header().ID()
	// root := flow.Txs().RootHash()
	// time := best.Header().Timestamp() + thor.BlockInterval
	// bs := block.NewBlockSummary(parent, root, time)
	// sig, err = crypto.Sign(bs.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	// if err != nil {
	// 	return nil, nil, err
	// }
	// bs = bs.WithSignature(sig)
	// // log.Debug(fmt.Sprintf("bs sig = 0x%x", bs.Signature()))
	// return bs, ts, nil
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
