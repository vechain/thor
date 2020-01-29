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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
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
		flow   *packer.Flow
		ticker = time.NewTicker(time.Second)

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
			flow, err := n.packer.Schedule(best.Header(), now)
			if err != nil {
				log.Error("Schedule", "err", err)
				flow = nil
				continue
			}

			start = mclock.Now()

			maxTxPackingDur := 3 // max number of seconds allowed to prepare transactions
			bs, ts, err := n.packTxSetAndBlockSummary(flow, maxTxPackingDur)
			if err != nil {
				log.Error("packTxSetAndBlockSummary", "err", err)
				flow = nil
				continue
			}

			txSetBlockSummaryDone = mclock.Now()

			n.comm.BroadcastBlockSummary(bs)
			if !ts.IsEmpty() {
				n.comm.BroadcastTxSet(ts)
			}

		case ev := <-newBlockSummaryCh:
			bs := ev.Summary

			log.Debug("Incoming block summary:")
			log.Debug(bs.String())

			now := uint64(time.Now().Unix())
			parent := n.chain.BestBlock().Header()

			if err := n.cons.ValidateBlockSummary(bs, parent, now); err != nil {
				log.Error("ValidateBlockSummary", "err", err)
				continue
			}

			// Check committee membership
			ok, proof, err := n.cons.IsCommittee(n.master.VrfPrivateKey, now)
			if err != nil {
				log.Error("IsCommittee", "err", err)
			}
			if ok {
				log.Debug("Endorsing", "hash", bs.EndorseHash().Bytes())
				ed := block.NewEndorsement(bs, proof)
				sig, err := crypto.Sign(ed.SigningHash().Bytes(), n.master.PrivateKey)
				if err != nil {
					log.Error("Signing endorsement", "err", err)
					continue
				}
				ed = ed.WithSignature(sig)
				n.comm.BroadcastEndorsement(ed)
			}

		case ev := <-newEndorsementCh:
			ed := ev.Endorsement
			log.Debug("Incoming endoresement:")
			log.Debug(ed.String())

			parentHeader := n.chain.BestBlock().Header()
			now := uint64(time.Now().Unix())
			if err := n.cons.ValidateEndorsement(ev.Endorsement, parentHeader, now); err != nil {
				continue
			}

			// n.comm.BroadcastEndorsement(ed)

			if flow != nil {
				if flow.ParentHeader().ID() != parentHeader.ID() {
					flow = nil
					log.Debug("Re-schedule packer due to new best block")
					continue
				}

				if now >= flow.When()+thor.BlockInterval {
					flow = nil
					log.Debug("Current round timeout")
					continue
				}

				if ok := flow.AddEndoresement(ed); !ok {
					log.Debug("Failed to add new endorsement", "#", flow.NumOfEndorsements())
				} else {
					log.Debug("Added new endorsement", "#", flow.NumOfEndorsements())
				}

				if uint64(flow.NumOfEndorsements()) < thor.CommitteeSize {
					continue
				}

				endorsementDone = mclock.Now()

				log.Debug("Packing new block header")
				blk, stage, receipts, err := flow.Pack(n.master.PrivateKey)
				if err != nil {
					log.Error("PackBlockHeader", "err", err)
					flow = nil
					continue
				}
				blockDone = mclock.Now()

				// reset flow
				flow = nil

				log.Debug("Committing new block")
				if err := n.commit(blk, stage, receipts); err != nil {
					log.Error("commit", "err", err)
					continue
				}
				commitDone = mclock.Now()

				display(blk, receipts,
					txSetBlockSummaryDone-start,
					endorsementDone-txSetBlockSummaryDone,
					blockDone-endorsementDone,
					commitDone-blockDone,
				)

				n.comm.BroadcastBlockHeader(blk.Header())
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

func display(b *block.Block, receipts tx.Receipts, prepareElapsed, collectElapsed, packElapsed, commitElapsed mclock.AbsTime) {
	blockID := b.Header().ID()
	log.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v|%v|%v",
			common.PrettyDuration(prepareElapsed),
			common.PrettyDuration(collectElapsed),
			common.PrettyDuration(packElapsed),
			common.PrettyDuration(commitElapsed),
		),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	// log.Debug(b.String())
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

	prevTrunk, curTrunk, err := n.commitBlock(newBlock, receipts)
	if err != nil {
		return errors.WithMessage(err, "commit block")
	}
	commitElapsed := mclock.Now() - startTime - execElapsed

	n.processFork(prevTrunk, curTrunk)

	if prevTrunk.HeadID() != curTrunk.HeadID() {
		n.comm.BroadcastBlock(newBlock)
		log.Info("📦 new block packed",
			"txs", len(receipts),
			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
			"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
			"id", shortID(newBlock.Header().ID()),
		)
	}

	if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(execElapsed+commitElapsed)); updated {
		log.Debug("bandwidth updated", "gps", v)
	}
	return nil
}

func (n *Node) commit(b *block.Block, stage *state.Stage, receipts tx.Receipts) error {
	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	// ignore fork when solo
	_, err := n.chain.AddBlock(b, receipts)
	if err != nil {
		return errors.WithMessage(err, "commit block")
	}

	task := n.logDB.NewTask().ForBlock(b.Header())
	for i, tx := range b.Transactions() {
		origin, _ := tx.Origin()
		task.Write(tx.ID(), origin, receipts[i].Outputs)
	}
	if err := task.Commit(); err != nil {
		return errors.WithMessage(err, "commit log")
	}

	return nil
}
