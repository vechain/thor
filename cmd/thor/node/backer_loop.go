// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/thor"
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

	newBsCh := make(chan *comm.NewBackerSignatureEvent)
	scope.Track(n.comm.SubscribeBackerSignature(newBsCh))

	unknownTicker := time.NewTicker(time.Duration(1) * time.Second)
	defer unknownTicker.Stop()

	seenProposal, _ := simplelru.NewLRU(512, nil)
	seenProposer, _ := simplelru.NewLRU(512, nil)
	seenBs, _ := simplelru.NewLRU(512, nil)

	var (
		knownProposal = cache.NewPrioCache(10)
		unknownBs     = cache.NewPrioCache(100)
		lastBacked    uint32
	)

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-newProposalCh:
			proposal := ev.Proposal
			parent := n.repo.BestBlock().Header()

			if parent.ID() != proposal.ParentID {
				continue
			}
			if seenProposal.Contains(proposal.Hash()) {
				continue // skip if proposal already seen
			}
			seenProposal.Add(proposal.Hash(), struct{}{})

			isProposerActive, err := func() (bool, error) {
				now := uint64(time.Now().Unix())

				signer, err := proposal.Signer()
				if err != nil {
					return false, err
				}

				isAuthority, isProposerActive, err := n.isAuthority(parent, signer)

				if err != nil {
					return false, err
				}
				if isAuthority == false {
					return false, fmt.Errorf("proposer: %s is not an authority", signer)
				}

				var key [24]byte
				copy(key[:], signer.Bytes())
				binary.BigEndian.PutUint32(key[20:], proposal.Number())
				if seenProposer.Contains(key) {
					return false, fmt.Errorf("proposer: %s already proposed in this round", signer)
				}
				seenProposer.Add(key, struct{}{})

				if proposal.Timestamp <= parent.Timestamp() {
					return false, errors.New("proposal timestamp behind parents")
				}

				if (proposal.Timestamp-parent.Timestamp())%thor.BlockInterval != 0 {
					return false, errors.New("block interval not rounded")
				}

				if proposal.Timestamp > now+thor.BlockInterval {
					return false, errors.New("proposal in the future")
				}

				return isProposerActive, nil
			}()
			if err != nil {
				log.Debug("block proposal is not valid", "err", err)
				continue
			}

			knownProposal.Set(proposal.Hash(), proposal, float64(proposal.Timestamp))

			if lastBacked == proposal.Number() && isProposerActive == true {
				log.Debug("already backed, skip this round", "block number", proposal.Number())
				continue
			}
			n.comm.BroadcastProposal(proposal)

			if isAuthority, _, err := n.isAuthority(parent, n.master.Address()); err != nil {
				log.Debug("failed to validate master", "err", err)
				continue
			} else if isAuthority == true {
				proposer, _ := proposal.Signer()
				alpha := proposal.Alpha(proposer).Bytes()
				beta, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(n.master.PrivateKey, alpha)
				if err != nil {
					log.Debug("failed trying to prove proposal", "err", err)
					continue
				}
				if lucky := poa.EvaluateVRF(beta); lucky == false {
					continue
				}

				bs := block.NewBackerSignature(crypto.CompressPubkey(&n.master.PrivateKey.PublicKey), proof)
				full := proto.FullBackerSignature{
					ProposalHash: proposal.Hash(),
					Signature:    bs,
				}
				lastBacked = proposal.Number()

				seenBs.Add(full.Hash(), struct{}{})
				n.comm.BroadcastBackerSignature(&full)
			}
		case ev := <-newBsCh:
			parent := n.repo.BestBlock().Header()

			if seenBs.Contains(ev.Hash()) {
				// skip if backer signature already seen
				continue
			}
			seenBs.Add(ev.Hash(), struct{}{})

			if err := n.validateBacker(ev.Signature, parent); err != nil {
				log.Debug("failed to verify backer", "err", err)
				continue
			}

			if val, _, ok := knownProposal.Get(ev.FullBackerSignature.ProposalHash); ok == true {
				proposal := val.(*block.Proposal)

				if parent.ID() != proposal.ParentID {
					continue
				}
				proposer, _ := proposal.Signer()
				alpha := proposal.Alpha(proposer)
				if err := n.validateBackerSignature(alpha.Bytes(), ev.Signature); err != nil {
					log.Debug("failed to validate backer signature", "err", err)
					continue
				}

				n.comm.BroadcastBackerSignature(ev.FullBackerSignature)
			} else {
				unknownBs.Set(ev.Hash(), ev.FullBackerSignature, float64(time.Now().Unix()))
			}
		case <-unknownTicker.C:
			var bss []*proto.FullBackerSignature
			unknownBs.ForEach(func(ent *cache.PrioEntry) bool {
				bss = append(bss, ent.Value.(*proto.FullBackerSignature))
				return true
			})

			parent := n.repo.BestBlock().Header()
			for _, bs := range bss {
				if val, _, ok := knownProposal.Get(bs.ProposalHash); ok == true {
					unknownBs.Remove(bs.Hash())
					proposal := val.(*block.Proposal)

					if parent.ID() != proposal.ParentID {
						continue
					}
					if err := n.validateBacker(bs.Signature, parent); err != nil {
						log.Debug("failed to verify backer", "err", err)
						continue
					}
					proposer, _ := proposal.Signer()
					alpha := proposal.Alpha(proposer)
					if err := n.validateBackerSignature(alpha.Bytes(), bs.Signature); err != nil {
						log.Debug("failed to validate backer signature", "err", err)
						continue
					}
					n.comm.BroadcastBackerSignature(bs)
				}
			}
		}
	}
}

func (n *Node) isAuthority(parent *block.Header, addr thor.Address) (isAuthority bool, isActive bool, err error) {
	st := n.stater.NewState(parent.StateRoot())
	authority := builtin.Authority.Native(st)

	listed, _, _, active, err := authority.Get(addr)
	if err != nil {
		return
	}

	if listed == false {
		return
	}

	return true, active, nil
}

func (n *Node) validateBacker(bs *block.BackerSignature, parentHeader *block.Header) error {
	signer, err := bs.Signer()
	if err != nil {
		return err
	}
	if isAuthority, _, err := n.isAuthority(parentHeader, signer); err != nil {
		return err
	} else if isAuthority == false {
		return fmt.Errorf("backer: %v not is not authority", signer)
	}
	return nil
}

func (n *Node) validateBackerSignature(alpha []byte, bs *block.BackerSignature) error {
	beta, err := bs.Validate(alpha)
	if err != nil {
		return err
	}

	if isBacker := poa.EvaluateVRF(beta); isBacker == false {
		signer, _ := bs.Signer()
		return fmt.Errorf("signer is not qualified to be a backer: %v", signer)
	}
	return nil
}
