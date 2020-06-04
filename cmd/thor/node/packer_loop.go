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
	"github.com/vechain/thor/poa"
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

		for {
			if uint64(time.Now().Unix())+thor.BlockInterval > flow.When() {
				if err := n.pack(flow); err != nil {
					log.Error("failed to pack block", "err", err)
				}
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				if n.repo.BestBlock().Header().TotalScore() > flow.TotalScore() {
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

	var scope event.SubscriptionScope
	defer scope.Close()

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
	execElapsed := mclock.Now() - startTime

	var proposal *block.Proposal
	if flow.Number() >= n.forkConfig.VIP193 {
		var err error
		proposal, err = flow.Propose(n.master.PrivateKey)
		if err != nil {
			return nil
		}
		n.comm.BroadcastProposal(proposal)
	}

	newApprovalCh := make(chan *comm.NewBlockApprovalEvent)
	scope.Track(n.comm.SubscribeApproval(newApprovalCh))

	now := uint64(time.Now().Unix())
	if now < flow.When()-1 {
		ticker := time.NewTimer(time.Duration(flow.When()-1-now) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case ev := <-newApprovalCh:
				if flow.Number() >= n.forkConfig.VIP193 {
					if ev.Approval.ProposalHash() == proposal.Hash() {
						err := func() (err error) {
							startTime := mclock.Now()
							defer func() {
								if err != nil {
									execElapsed += mclock.Now() - startTime
								}
							}()

							approval := ev.Approval.Approval()
							signer, err := approval.Signer()

							if known := flow.IsBackerKnown(signer); known == true {
								return errors.New("known backer")
							}

							inPower, err := n.packer.InPower(flow.ParentHeader(), signer)
							if err != nil {
								return
							}
							if inPower == false {
								return fmt.Errorf("backer of approval is not in power %v", signer)
							}

							alpha := proposal.Hash().Bytes()
							beta, err := approval.Validate(alpha)
							if err != nil {
								return err
							}
							isBacker := poa.EvaluateVRF(beta)
							if isBacker == true {
								flow.AddApproval(approval)
							} else {
								return fmt.Errorf("signer is not qualified to be a backer: %v", signer)
							}
							return
						}()
						if err != nil {
							log.Debug("failed to process approval", "err", err)
							continue
						}
					}
				}
			case <-ticker.C:
				goto NEXT
			}
		}
	NEXT:
	}

	startTime = mclock.Now()
	newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
	if err != nil {
		return err
	}
	execElapsed += mclock.Now() - startTime

	startTime = mclock.Now()
	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	prevTrunk, curTrunk, err := n.commitBlock(newBlock, receipts)
	if err != nil {
		return errors.WithMessage(err, "commit block")
	}
	commitElapsed := mclock.Now() - startTime

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
