// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/pkg/errors"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/thor"
)

var (
	seenDraft, _    = simplelru.NewLRU(512, nil)
	seenProposer, _ = simplelru.NewLRU(512, nil)
	seenAccepted, _ = simplelru.NewLRU(512, nil)

	knownProposal   = cache.NewPrioCache(16)
	unknownDraft    = cache.NewRandCache(128)
	unknownAccepted = cache.NewRandCache(128)
)

type status struct {
	parent    *block.Header
	proposers []poa.Proposer
	scheduler *poa.SchedulerV2
	seed      []byte
}

type acceptedWithPub struct {
	Accepted *proto.Accepted
	Pub      *ecdsa.PublicKey
}

func newStatus(node *Node, parent *block.Block) (*status, error) {
	state := node.stater.NewState(parent.Header().StateRoot())

	authority := builtin.Authority.Native(state)
	endorsement, err := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
	if err != nil {
		return nil, err
	}
	candidates, err := authority.Candidates(endorsement, thor.MaxBlockProposers)
	if err != nil {
		return nil, err
	}

	proposers := make([]poa.Proposer, 0, len(candidates))
	for _, c := range candidates {
		proposers = append(proposers, poa.Proposer{
			Address: c.NodeMaster,
			Active:  c.Active,
		})
	}

	seed, err := node.seeder.Generate(parent.Header().ID())
	if err != nil {
		return nil, err
	}

	scheduler, err := poa.NewSchedulerV2(node.master.Address(), proposers, parent, seed.Bytes())
	if err != nil {
		return nil, err
	}

	return &status{
		parent:    parent.Header(),
		proposers: proposers,
		scheduler: scheduler,
		seed:      seed.Bytes(),
	}, nil
}

func (st *status) GetAuthority(addr thor.Address) *poa.Proposer {
	for _, p := range st.proposers {
		if p.Address == addr {
			return &poa.Proposer{
				Address: p.Address,
				Active:  p.Active,
			}
		}
	}
	return nil
}

func (st *status) IsScheduled(blockTime uint64, proposer thor.Address) bool {
	return st.scheduler.IsScheduled(blockTime, proposer)
}

func (st *status) Seed() []byte {
	return st.seed
}

func (st *status) ParentID() thor.Bytes32 {
	return st.parent.ID()
}

func (st *status) ParentNumber() uint32 {
	return st.parent.Number()
}

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

	newDraftCh := make(chan *comm.NewDraftEvent)
	scope.Track(n.comm.SubscribeDraft(newDraftCh))

	newAcceptedCh := make(chan *comm.NewAcceptedEvent)
	scope.Track(n.comm.SubscribeAccepted(newAcceptedCh))

	unknownTicker := time.NewTicker(time.Second)
	defer unknownTicker.Stop()

	var st *status
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-newDraftCh:
			if st == nil {
				// not yet initiated
				continue
			}

			p := ev.Proposal
			hash := ev.Draft.Hash()

			// only accept proposal that are within 3 rounds.
			if math.Abs(float64(p.Number())-float64(st.ParentNumber()+1)) > 3 {
				continue
			}

			// skip if draft already seen(prevent DoS)
			if seenDraft.Contains(hash) {
				continue
			}
			seenDraft.Add(hash, struct{}{})

			if st.ParentID() != p.ParentID {
				unknownDraft.Set(hash, ev.Draft)
				continue
			}

			proposalHash := p.Hash()
			if err := n.validateProposer(ev.Draft, proposalHash, st); err != nil {
				log.Debug("signer not valid", "err", err)
				continue
			}

			if err := n.validateProposal(p, st); err != nil {
				log.Debug("block Proposal is not valid", "err", err)
				continue
			}

			n.comm.BroadcastDraft(ev.Draft)
			knownProposal.Set(proposalHash, p, float64(p.Timestamp))

			if st.GetAuthority(n.master.Address()) != nil {
				signature, err := n.tryBacking(proposalHash, st)
				if err != nil {
					log.Debug("failed to back a proposal", "err", err)
					continue
				}

				accepted := proto.Accepted{
					ProposalHash: proposalHash,
					Signature:    signature,
				}

				seenAccepted.Add(accepted.Hash(), struct{}{})
				n.comm.BroadcastAccepted(&accepted)
			}
		case ev := <-newAcceptedCh:
			if st == nil {
				// not yet initiated
				continue
			}

			hash := ev.Hash()
			// skip if backer signature already seen(prevent DOS)
			if seenAccepted.Contains(hash) {
				continue
			}
			seenAccepted.Add(hash, struct{}{})

			pub, err := n.validateBacker(ev.Accepted, st)
			if err != nil {
				log.Debug("failed to validate backer", "err", err)
				continue
			}

			if val, _, ok := knownProposal.Get(ev.ProposalHash); ok == true {
				p := val.(*block.Proposal)

				if st.ParentID() != p.ParentID {
					continue
				}
				if err := n.validateBackerSignature(ev.Signature, pub, st); err != nil {
					log.Debug("failed to validate backer signature", "err", err)
					continue
				}

				n.comm.BroadcastAccepted(ev.Accepted)
			} else {
				unknownAccepted.Set(hash, acceptedWithPub{
					Accepted: ev.Accepted,
					Pub:      pub,
				})
			}
		case <-unknownTicker.C:
			parent := n.repo.BestBlock()

			if st == nil || st.ParentID() != parent.Header().ID() {
				new, err := newStatus(n, parent)
				if err != nil {
					log.Debug("failed to initiate status", "err", err)
					continue
				}
				st = new
			}

			var drafts []*cache.Entry
			unknownDraft.ForEach(func(ent *cache.Entry) bool {
				drafts = append(drafts, ent)
				return true
			})
			for _, ent := range drafts {
				draft := ent.Value.(*proto.Draft)
				p := draft.Proposal
				hash := ent.Key.(thor.Bytes32)
				// remove obsolete proposals
				if math.Abs(float64(p.Number())-float64(st.ParentNumber()+1)) > 3 {
					unknownDraft.Remove(hash)
					continue
				}
				if p.ParentID == parent.Header().ID() {
					unknownDraft.Remove(hash)

					proposalHash := p.Hash()
					if err := n.validateProposer(draft, proposalHash, st); err != nil {
						log.Debug("signer not valid", "err", err)
						continue
					}

					if err := n.validateProposal(p, st); err != nil {
						log.Debug("block proposal is not valid", "err", err)
						continue
					}

					n.comm.BroadcastDraft(draft)
					knownProposal.Set(proposalHash, p, float64(p.Timestamp))

					if st.GetAuthority(n.master.Address()) != nil {
						signature, err := n.tryBacking(proposalHash, st)
						if err != nil {
							log.Debug("failed to back a proposal", "err", err)
							continue
						}

						accepted := proto.Accepted{
							ProposalHash: hash,
							Signature:    signature,
						}

						seenAccepted.Add(accepted.Hash(), struct{}{})
						n.comm.BroadcastAccepted(&accepted)
					}
				}
			}

			var aps []*acceptedWithPub
			unknownAccepted.ForEach(func(ent *cache.Entry) bool {
				aps = append(aps, ent.Value.(*acceptedWithPub))
				return true
			})

			for _, ap := range aps {
				accepted := ap.Accepted
				pub := ap.Pub
				if val, _, ok := knownProposal.Get(accepted.ProposalHash); ok == true {
					unknownAccepted.Remove(accepted.Hash())

					p := val.(*block.Proposal)

					if parent.Header().ID() != p.ParentID {
						continue
					}

					if err := n.validateBackerSignature(accepted.Signature, pub, st); err != nil {
						log.Debug("failed to validate backer signature", "err", err)
						continue
					}

					n.comm.BroadcastAccepted(accepted)
				}
			}
		}
	}
}

func (n *Node) validateProposer(d *proto.Draft, proposalHash thor.Bytes32, st *status) error {
	pub, err := crypto.SigToPub(proposalHash.Bytes(), d.Signature)
	if err != nil {
		return err
	}
	signer := thor.Address(crypto.PubkeyToAddress(*pub))

	if st.GetAuthority(signer) == nil {
		return errors.Errorf("proposer: %v is not an authority", signer)
	}

	p := d.Proposal
	var key [32]byte
	copy(key[:], signer.Bytes())
	binary.BigEndian.PutUint32(key[20:], p.Number())
	binary.BigEndian.PutUint64(key[24:], p.Timestamp)
	if seenProposer.Contains(key) {
		return errors.Errorf("proposer:%v already proposed in this round", signer)
	}
	seenProposer.Add(key, struct{}{})

	if st.IsScheduled(p.Timestamp, signer) == false {
		return errors.New("proposal not scheduled")
	}
	return nil
}

func (n *Node) validateProposal(p *block.Proposal, st *status) error {
	now := uint64(time.Now().Unix())
	if p.Timestamp <= st.parent.Timestamp() {
		return errors.New("proposal timestamp behind parents")
	}

	if (p.Timestamp-st.parent.Timestamp())%thor.BlockInterval != 0 {
		return errors.New("block interval not rounded")
	}

	if p.Timestamp > now+thor.BlockInterval {
		return errors.New("proposal in the future")
	}

	return nil
}

func (n *Node) tryBacking(proposalHash thor.Bytes32, st *status) (block.ComplexSignature, error) {
	alpha := append([]byte(nil), st.Seed()...)
	alpha = append(alpha, st.ParentID().Bytes()[:4]...)

	beta, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(n.master.PrivateKey, alpha)
	if err != nil {
		return nil, err
	}

	if lucky := poa.EvaluateVRF(beta); lucky == false {
		return nil, errors.New("not lucky enough")
	}

	signature, err := crypto.Sign(proposalHash.Bytes(), n.master.PrivateKey)
	if err != nil {
		return nil, err
	}

	bs, err := block.NewComplexSignature(proof, signature)
	if err != nil {
		return nil, err
	}

	return bs, nil
}

func (n *Node) validateBacker(acc *proto.Accepted, st *status) (*ecdsa.PublicKey, error) {
	pub, err := crypto.SigToPub(acc.ProposalHash.Bytes(), acc.Signature.Signature())
	if err != nil {
		return nil, err
	}
	backer := thor.Address(crypto.PubkeyToAddress(*pub))

	if st.GetAuthority(backer) == nil {
		return nil, errors.Errorf("backer:%v is not an authority", backer)
	}

	return pub, nil
}

func (n *Node) validateBackerSignature(bs block.ComplexSignature, pub *ecdsa.PublicKey, st *status) error {
	alpha := append([]byte(nil), st.Seed()...)
	alpha = append(alpha, st.ParentID().Bytes()[:4]...)

	beta, err := ecvrf.NewSecp256k1Sha256Tai().Verify(pub, alpha, bs.Proof())
	if err != nil {
		return err
	}

	if isBacker := poa.EvaluateVRF(beta); isBacker == false {
		return fmt.Errorf("VRF output is not lucky enough to be a backer: %v", crypto.PubkeyToAddress(*pub))
	}
	return nil
}
