// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/poa"
)

func (n *Node) backerLoop(ctx context.Context) {
	log.Debug("enter backer loop")
	defer log.Debug("leave backer loop")

	select {
	case <-ctx.Done():
		return
	case <-n.comm.Synced():
	}

	ticker := n.repo.NewTicker()
	for {
		if n.repo.BestBlock().Header().Number() >= n.forkConfig.VIP193 {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			continue
		}
	}

	var scope event.SubscriptionScope
	defer scope.Close()

	newProposalCh := make(chan *comm.NewBlockProposalEvent)
	scope.Track(n.comm.SubscribeProposal(newProposalCh))

	newApprovalCh := make(chan *comm.NewBlockApprovalEvent)
	scope.Track(n.comm.SubscribeApproval(newApprovalCh))

	unknownTicker := time.NewTicker(time.Duration(1) * time.Second)
	defer unknownTicker.Stop()

	var knownProposal = cache.NewPrioCache(10)
	var unknownApproval = cache.NewPrioCache(100)

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-newProposalCh:
			best := n.repo.BestBlock()

			proposal := ev.Proposal
			if best.Header().ID() == proposal.ParentID() {
				err := n.cons.ValidateProposal(proposal)
				if err != nil {
					log.Debug("block proposal is not valid", "err", err)
					continue
				}
				knownProposal.Set(proposal.Hash(), proposal, float64(proposal.Timestamp()))
				n.comm.BroadcastProposal(proposal)

				inPower, err := n.packer.InPower(best.Header(), n.master.Address())
				if err != nil {
					log.Debug("failed to validate master", "err", err)
					continue
				}
				if inPower == true {
					alpha := proposal.Hash().Bytes()
					lucky, proof, err := poa.TryApprove(n.master.PrivateKey, alpha)
					if err != nil {
						log.Debug("failed during trying prove proposal", "err", err)
						continue
					}
					if lucky == false {
						continue
					}

					pub := crypto.CompressPubkey(&n.master.PrivateKey.PublicKey)
					approval := block.NewFullApproval(proposal.Hash(), block.NewApproval(pub, proof))

					n.comm.BroadcastApproval(approval)
				}
			}
		case ev := <-newApprovalCh:
			if val, _, ok := knownProposal.Get(ev.Approval.ProposalHash()); ok == true {
				proposal := val.(*block.Proposal)

				best := n.repo.BestBlock()
				if best.Header().ID() == proposal.ParentID() {
					err := n.validateApproval(best.Header(), proposal, ev.Approval.Approval())
					if err != nil {
						log.Debug("failed to validate approval", "err", err)
						continue
					}
					n.comm.BroadcastApproval(ev.Approval)
				}
			} else {
				unknownApproval.Set(ev.Approval.Hash(), ev.Approval, float64(time.Now().Unix()))
			}
		case <-unknownTicker.C:
			var approvals []*block.FullApproval
			unknownApproval.ForEach(func(ent *cache.PrioEntry) bool {
				approvals = append(approvals, ent.Value.(*block.FullApproval))
				return true
			})
			for _, a := range approvals {
				if val, _, ok := knownProposal.Get(a.ProposalHash()); ok == true {
					proposal := val.(*block.Proposal)
					best := n.repo.BestBlock()
					if best.Header().ID() == proposal.ParentID() {
						err := n.validateApproval(best.Header(), proposal, a.Approval())
						if err != nil {
							log.Debug("failed to validate approval", "err", err)
							continue
						}
						n.comm.BroadcastApproval(a)
					}
					unknownApproval.Remove(a.Hash())
				}
			}
		}
	}
}

func (n *Node) validateApproval(parentHeader *block.Header, proposal *block.Proposal, approval *block.Approval) error {
	if err := n.cons.ValidateProposal(proposal); err != nil {
		return err
	}

	signer, err := approval.Signer()
	inPower, err := n.packer.InPower(parentHeader, signer)
	if err != nil {
		return err
	}
	if inPower == false {
		return fmt.Errorf("backer: %v not in power", signer)
	}

	alpha := proposal.Hash().Bytes()
	pub, err := approval.PublickKey()
	if err != nil {
		return err
	}
	isBacker, err := poa.VerifyBacker(pub, alpha, approval.Proof())
	if err != nil {
		return err
	}
	if isBacker == false {
		return fmt.Errorf("signer is not qualified to be a backer: %v", signer)
	}
	return nil
}
