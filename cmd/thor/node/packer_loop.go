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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// packerLoop is executed by the leader and committee members to
// 1. prepare and broadcast block summary & tx set as leader
// 2. prepare and broadcast endorsements as committee members
// 3. pack and broadcast header as leader
// 4. pack and commit new block as leader
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
		flow      *packer.Flow
		activated = false // flow instance will be activated after it starts to pack things
		err       error
		ticker    = time.NewTicker(time.Second)

		start, txSetBlockSummaryDone, endorsementDone, blockDone, commitDone mclock.AbsTime
	)

	defer ticker.Stop()

	n.packer.SetTargetGasLimit(n.targetGasLimit)

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	scope.Track(n.comm.SubscribeBlockSummary(newBlockSummaryCh))
	newEndorsementCh := make(chan *comm.NewEndorsementEvent)
	scope.Track(n.comm.SubscribeEndorsement(newEndorsementCh))

	var lbs *block.Summary // a local copy of the latest block summary

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if flow != nil && activated { // flow must be either nil or not activated to proceed
				continue
			}

			best := n.chain.BestBlock()
			now := uint64(time.Now().Unix())
			// fmt.Println(now)

			if flow == nil {
				// Schedule a round to be the leader
				flow, err = n.packer.Schedule(best.Header(), now)
				if err != nil {
					log.Error("Schedule", "key", "packer", "err", err)
					continue
				}

				fmt.Printf("now = %v, when = %v\n", now, flow.When())
			}

			// Check whether the best block has changed
			if flow.ParentHeader().ID() != best.Header().ID() {
				flow = nil
				log.Debug("re-schedule packer due to new best block")
				continue
			}

			// Check whether it is the scheduled round for producing a new block
			if now+1 < flow.When() {
				continue
			}

			// Check timeout
			if now > flow.When()+thor.BlockInterval {
				flow = nil
				activated = false
				continue
			}

			start = mclock.Now()

			activated = true // Mark the flow as activated

			maxTxPackingDur := 3 // max number of seconds allowed to prepare transactions
			bs, ts, err := n.packTxSetAndBlockSummary(flow, maxTxPackingDur)
			if err != nil {
				log.Error("packTxSetAndBlockSummary", "key", "packer", "err", err)

				flow = nil
				activated = false
				continue
			}
			txSetBlockSummaryDone = mclock.Now()

			lbs = bs // save a local copy of the latest block summary
			log.Info("bs ==>", "key", "packer", "id", bs.ID())
			n.comm.BroadcastBlockSummary(bs)
			if !ts.IsEmpty() {
				log.Info("ts ==>", "key", "packer", "id", ts.ID())
				n.comm.BroadcastTxSet(ts)
			}

		case ev := <-newBlockSummaryCh:
			now := uint64(time.Now().Unix())
			best := n.chain.BestBlock()

			// Only receive one block summary from the same leader once in the same round
			if lbs != nil {
				if n.cons.ValidateBlockSummary(lbs, best.Header(), now) == nil {
					continue
				}
				lbs = nil
			}

			bs := ev.Summary
			log.Info("<== bs", "key", "packer", "id", bs.ID())

			if err := n.cons.ValidateBlockSummary(bs, best.Header(), now); err != nil {
				log.Error("ValidateBlockSummary", "key", "packer", "err", err)
				continue
			}

			lbs = bs // save the local copy of the latest received block summary

			// Check the committee membership
			ok, proof, err := n.cons.IsCommittee(n.master.VrfPrivateKey, now)
			if err != nil {
				log.Error("IsCommittee", "key", "packer", "err", err)
				continue
			}
			if ok {
				// Endorse the block summary
				ed := block.NewEndorsement(bs, proof)
				sig, err := crypto.Sign(ed.SigningHash().Bytes(), n.master.PrivateKey)
				if err != nil {
					log.Error("Signing endorsement", "key", "packer", "err", err)
					continue
				}
				ed = ed.WithSignature(sig)

				log.Info("ed ==>", "key", "packer", "id", ed.ID())
				n.comm.BroadcastEndorsement(ed)
			}

		case ev := <-newEndorsementCh:
			if flow == nil || !activated { // flow must be activated to proceed
				continue
			}

			best := n.chain.BestBlock()
			now := uint64(time.Now().Unix())

			// Check whether the best block has changed
			if flow.ParentHeader().ID() != best.Header().ID() {
				flow = nil
				activated = false
				log.Debug("re-schedule packer due to new best block")
				continue
			}

			// Check whether it is the scheduled round for producing a new block
			if now+1 < flow.When() {
				continue
			}

			// Check timeout
			if now > flow.When()+thor.BlockInterval {
				flow = nil
				activated = false
				continue
			}

			ed := ev.Endorsement
			log.Info("<== ed", "key", "packer", "id", ed.ID())

			if err := n.cons.ValidateEndorsement(ev.Endorsement, best.Header(), now); err != nil {
				log.Info("invalid ed", "key", "packer", "id", ed.ID())
				continue
			}

			if ok := flow.AddEndoresement(ed); !ok {
				log.Info("Failed to add ed", "key", "packer", "#", flow.NumOfEndorsements(), "id", ed.ID())
			} else {
				log.Info("Added ed", "key", "packer", "#", flow.NumOfEndorsements(), "id", ed.ID())
			}

			// Pack a new block if there have been enough endorsements collected
			if uint64(flow.NumOfEndorsements()) < thor.CommitteeSize {
				continue
			}

			endorsementDone = mclock.Now()

			log.Info("Packing new header")
			newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
			if err != nil {
				log.Error("PackBlockHeader", "err", err)
				flow = nil
				continue
			}
			blockDone = mclock.Now()

			// reset flow
			flow = nil

			log.Info("Committing new block")
			if _, err := stage.Commit(); err != nil {
				log.Error("commit state", "err", err)
			}

			fork, err := n.commitBlock(newBlock, receipts)
			if err != nil {
				log.Error("commit block", "err", err)
			}
			commitDone = mclock.Now()

			n.processFork(fork)

			if len(fork.Trunk) > 0 {
				display(newBlock, receipts,
					txSetBlockSummaryDone-start,
					endorsementDone-txSetBlockSummaryDone,
					blockDone-endorsementDone,
					commitDone-blockDone,
				)

				log.Info("hd ==>", "key", "packer", "id", newBlock.Header().ID())
				n.comm.BroadcastBlockHeader(newBlock.Header())
			}

			execElapsed := blockDone - endorsementDone
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
		}
	}
}

func (n *Node) packTxSetAndBlockSummary(flow *packer.Flow, maxTxPackingDur int) (*block.Summary, *block.TxSet, error) {
	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			n.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	done := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(maxTxPackingDur) * time.Second)
		done <- struct{}{}
	}()

	for _, tx := range n.txPool.Executables() {
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

	bs, ts, err := flow.PackTxSetAndBlockSummary(n.master.PrivateKey)
	if err != nil {
		return nil, nil, err
	}

	return bs, ts, nil
}

func display(blk *block.Block, receipts tx.Receipts, prepareElapsed, collectElapsed, packElapsed, commitElapsed mclock.AbsTime) {
	blockID := blk.Header().ID()
	log.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(blk.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v|%v|%v",
			common.PrettyDuration(prepareElapsed),
			common.PrettyDuration(collectElapsed),
			common.PrettyDuration(packElapsed),
			common.PrettyDuration(commitElapsed),
		),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
}

// func (n *Node) pack(flow *packer.Flow) error {
// 	txs := n.txPool.Executables()
// 	var txsToRemove []*tx.Transaction
// 	defer func() {
// 		for _, tx := range txsToRemove {
// 			n.txPool.Remove(tx.Hash(), tx.ID())
// 		}
// 	}()

// 	startTime := mclock.Now()
// 	for _, tx := range txs {
// 		if err := flow.Adopt(tx); err != nil {
// 			if packer.IsGasLimitReached(err) {
// 				break
// 			}
// 			if packer.IsTxNotAdoptableNow(err) {
// 				continue
// 			}
// 			txsToRemove = append(txsToRemove, tx)
// 		}
// 	}

// 	newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
// 	if err != nil {
// 		return err
// 	}
// 	execElapsed := mclock.Now() - startTime

// 	if _, err := stage.Commit(); err != nil {
// 		return errors.WithMessage(err, "commit state")
// 	}

// 	fork, err := n.commitBlock(newBlock, receipts)
// 	if err != nil {
// 		return errors.WithMessage(err, "commit block")
// 	}
// 	commitElapsed := mclock.Now() - startTime - execElapsed

// 	n.processFork(fork)

// 	if len(fork.Trunk) > 0 {
// 		n.comm.BroadcastBlock(newBlock)
// 		log.Info("📦 new block packed",
// 			"txs", len(receipts),
// 			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
// 			"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
// 			"id", shortID(newBlock.Header().ID()),
// 		)
// 	}

// 	if n.targetGasLimit == 0 {
// 		n.packer.SetTargetGasLimit(0)
// 		if execElapsed > 0 {
// 			gasUsed := newBlock.Header().GasUsed()
// 			// calc target gas limit only if gas used above third of gas limit
// 			if gasUsed > newBlock.Header().GasLimit()/3 {
// 				targetGasLimit := uint64(math.Log2(float64(newBlock.Header().Number()+1))*float64(thor.TolerableBlockPackingTime)*float64(gasUsed)) / (32 * uint64(execElapsed))
// 				n.packer.SetTargetGasLimit(targetGasLimit)
// 				log.Debug("reset target gas limit", "value", targetGasLimit)
// 			}
// 		}
// 	}
// 	return nil
// }
