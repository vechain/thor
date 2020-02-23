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
	"github.com/ethereum/go-ethereum/event"
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

	// log.Info("waiting for synchronization...")
	// select {
	// case <-ctx.Done():
	// 	return
	// case <-n.comm.Synced():
	// }
	// log.Info("synchronization process done")

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
			n.packer.SetTargetGasLimit(suggested)
		}

		// terminates the loop if the current height is vip193
		if n.isNextBlockVip193() {
			return
		}

		flow, err := n.packer.Schedule(n.repo.BestBlock().Header(), now)
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

		const halfBlockInterval = thor.BlockInterval / 2

		for {
			now := uint64(time.Now().Unix())
			if flow.When() > now+halfBlockInterval {
				delaySec := flow.When() - (now + halfBlockInterval)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C():
					if n.repo.BestBlock().Header().TotalScore() > flow.TotalScore() {
						log.Debug("re-schedule packer due to new best block")
						goto RE_SCHEDULE
					}
				case <-time.After(time.Duration(delaySec) * time.Second):
					goto PACK
				}
			} else {
				goto PACK
			}
		}
	PACK:
		if err := n.pack(flow); err != nil {
			log.Error("failed to pack block", "err", err)
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

// 1. prepare and broadcast block summary & tx set
// 2. pack and broadcast header
// 3. pack and commit new block
func (n *Node) packerLoopVip193(ctx context.Context) {
	debugLog := func(str string, kv ...interface{}) {
		log.Info(str, append([]interface{}{"key", "packer"}, kv...)...)
	}

	errLog := func(msg string, kv ...interface{}) {
		log.Error(msg, append([]interface{}{"key", "packer"}, kv...)...)
	}

	debugLog("enter vip193 packer loop")
	defer debugLog("leave vip193 packer loop")

	// debugLog("waiting for synchronization...")
	// select {
	// case <-ctx.Done():
	// 	return
	// case <-n.comm.Synced():
	// }
	// debugLog("synchronization process done")

	var (
		flow   = new(packer.Flow)
		err    error
		ticker = time.NewTicker(time.Second)

		minTxAdoptDur uint64             = 2 // minimum duration (sec) needs to adopt txs from txPool
		txAdoptStart  uint64                 // starting time of the loop that adopts txs
		cancel        context.CancelFunc     // cancel the go routine that adopts txs

		start, txSetBlockSummaryDone, endorsementDone, blockDone, commitDone mclock.AbsTime
	)

	defer ticker.Stop()

	n.packer.SetTargetGasLimit(n.targetGasLimit)

	var scope event.SubscriptionScope
	defer scope.Close()

	newEndorsementCh := make(chan *comm.NewEndorsementEvent)
	scope.Track(n.comm.SubscribeEndorsement(newEndorsementCh))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := uint64(time.Now().Unix())
			best := n.repo.BestBlock()

			if flow.IsEmpty() {
				// Schedule a round to be the leader
				flow, err = n.packer.Schedule(best.Header(), now)
				if err != nil {
					errLog("Schedule", "err", err)
					continue
				}

				// start the go routine for adopting txs right afer flow is renewed
				_ctx, _cancel := context.WithCancel(ctx)
				go n.adoptTxs(_ctx, flow)
				cancel = _cancel
				txAdoptStart = now
			}

			// Check whether the best block has changed
			if flow.ParentHeader().ID() != best.Header().ID() {
				cancel()
				flow.Close()

				debugLog("re-schedule packer due to new best block")
				continue
			}

			// wait until the scheduled time
			if now < flow.When() {
				continue
			}

			// if timeout, reset
			if now > flow.When()+thor.BlockInterval {
				cancel()
				flow.Close()

				debugLog("re-schedule packer due to timeout")
				continue
			}

			// do nothing if having already packed bs and ts
			if flow.HasPackedBlockSummary() {
				continue
			}

			start = mclock.Now()

			// Compute how long the adoptTx go routine can continue to run.
			// The go routine is guaranteed to run for at least the duration
			// defined by [minTxAdoptDur].
			waitTime := minTxAdoptDur - (now - txAdoptStart)
			if waitTime > 0 {
				<-time.After(time.Second * time.Duration(waitTime))
			}
			cancel()
			bs, ts, err := flow.PackTxSetAndBlockSummary(n.master.PrivateKey)
			if err != nil {

				flow.Close()
				errLog("pack bs and ts", "err", err)
				continue
			}

			txSetBlockSummaryDone = mclock.Now()

			debugLog("bs ==>", "id", bs.ID().Abev())

			// send the new block summary via comm.newBlockSummaryCh to the housekeeping
			// and endorsor loops for broadcasting and endorsement, respectively. packer loop
			// does not take the responsibility of endorsing block summary
			n.comm.SendBlockSummaryToFeed(bs)

			if ts != nil {
				debugLog("ts ==>", "id", ts.ID().Abev())
				n.comm.BroadcastTxSet(ts)
			}

		case ev := <-newEndorsementCh:
			// proceed only when the node has already packed a block summary
			if flow.IsEmpty() || !flow.HasPackedBlockSummary() {
				continue
			}

			best := n.repo.BestBlock()
			now := uint64(time.Now().Unix())

			// Check whether the best block has changed
			if flow.ParentHeader().ID() != best.Header().ID() {
				cancel()
				flow.Close()

				debugLog("re-schedule packer due to new best block")
				continue
			}

			// Check timeout
			if now > flow.When()+thor.BlockInterval {
				cancel()
				flow.Close()

				debugLog("re-schedule packer due to timeout")
				continue
			}

			ed := ev.Endorsement
			debugLog("<== ed", "id", ed.ID().Abev())

			if err := n.cons.ValidateEndorsement(ev.Endorsement, best.Header(), now); err != nil {
				debugLog("invalid ed", "err", err, "id", ed.ID().Abev())
				continue
			}

			if ok := flow.AddEndoresement(ed); !ok {
				debugLog("Failed to add ed", "#", flow.NumOfEndorsements(), "id", ed.ID().Abev())
			} else {
				debugLog("Added ed", "#", flow.NumOfEndorsements(), "id", ed.ID().Abev())
			}

			// Pack a new block if there have been enough endorsements collected
			if uint64(flow.NumOfEndorsements()) < thor.CommitteeSize {
				continue
			}

			endorsementDone = mclock.Now()

			debugLog("Packing new header")
			newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
			if err != nil {
				errLog("pack block", "err", err)
				flow.Close()

				continue
			}
			blockDone = mclock.Now()

			// reset flow
			flow.Close()

			debugLog("Committing new block")
			if _, err := stage.Commit(); err != nil {
				errLog("commit state", "err", err)
				flow.Close()

				continue
			}

			prevTrunk, curTrunk, err := n.commitBlock(newBlock, receipts)
			if err != nil {
				errLog("commit block", "err", err)
				flow.Close()

				continue
			}
			commitDone = mclock.Now()

			n.processFork(prevTrunk, curTrunk)

			if prevTrunk.HeadID() != curTrunk.HeadID() {
				debugLog("hd ==>", "id", newBlock.Header().ID().Abev())
				n.comm.BroadcastBlockHeader(newBlock.Header())

				display(newBlock, receipts,
					txSetBlockSummaryDone-start,
					endorsementDone-txSetBlockSummaryDone,
					blockDone-endorsementDone,
					commitDone-blockDone,
				)
			}

			if v, updated := n.bandwidth.Update(newBlock.Header(), time.Duration(commitDone-start)); updated {
				log.Debug("bandwidth updated", "gps", v)
			}
		}
	}
}

func (n *Node) adoptTxs(ctx context.Context, flow *packer.Flow) {
	start := uint64(time.Now().Unix())
	defer func() {
		dur := uint64(time.Now().Unix()) - start
		log.Debug("having adopted txs for %v seconds\n", dur)
	}()

	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			n.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// record txs that have been handled
	knownTxs := make(map[thor.Bytes32]struct{})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, tx := range n.txPool.Executables() {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if _, ok := knownTxs[tx.ID()]; ok {
					continue
				}
				knownTxs[tx.ID()] = struct{}{}

				err := flow.Adopt(tx)
				switch {
				case packer.IsGasLimitReached(err):
					return
				case packer.IsTxNotAdoptableNow(err):
					continue
				default:
					txsToRemove = append(txsToRemove, tx)
				}
			}
		}
	}

}

func display(blk *block.Block, receipts tx.Receipts, prepareElapsed, collectElapsed, packElapsed, commitElapsed mclock.AbsTime) {
	blockID := blk.Header().ID()
	log.Info("📦 new vip193 block packed",
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
