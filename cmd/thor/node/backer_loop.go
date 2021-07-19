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
	seenAccepted, _ = simplelru.NewLRU(512, nil)

	knownProposal   = cache.NewPrioCache(16)
	unknownDraft    = cache.NewRandCache(128)
	unknownAccepted = cache.NewRandCache(128)
)

type status struct {
	parent            *block.Header
	proposers         map[thor.Address]bool
	maxBlockProposers uint64
	scheduler         *poa.SchedulerV2
	alpha             []byte
}

type acceptedWithPub struct {
	Accepted *proto.Accepted
	Pub      *ecdsa.PublicKey
}

type draftWithSigner struct {
	Draft  *proto.Draft
	Signer thor.Address
}

func newStatus(node *Node, parent *block.Block) (*status, error) {
	state := node.stater.NewState(parent.Header().StateRoot())

	authority := builtin.Authority.Native(state)
	endorsement, err := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
	if err != nil {
		return nil, err
	}
	mbp, err := builtin.Params.Native(state).Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, err
	}
	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}
	candidates, err := authority.Candidates(endorsement, maxBlockProposers)
	if err != nil {
		return nil, err
	}

	proposers := make([]poa.Proposer, 0, len(candidates))
	pps := make(map[thor.Address]bool, len(candidates))
	for _, c := range candidates {
		proposers = append(proposers, poa.Proposer{
			Address: c.NodeMaster,
			Active:  c.Active,
		})
		pps[c.NodeMaster] = true
	}

	seed, err := node.seeder.Generate(parent.Header().ID())
	if err != nil {
		return nil, err
	}

	// use parent block's signer just for initiating the scheduler
	signer, _ := parent.Header().Signer()
	scheduler, err := poa.NewSchedulerV2(signer, proposers, parent, seed.Bytes())
	if err != nil {
		return nil, err
	}

	alpha := append([]byte(nil), seed.Bytes()...)
	alpha = append(alpha, parent.Header().ID().Bytes()[:4]...)

	return &status{
		parent:            parent.Header(),
		proposers:         pps,
		maxBlockProposers: maxBlockProposers,
		scheduler:         scheduler,
		alpha:             alpha,
	}, nil
}

func (st *status) IsAuthority(addr thor.Address) bool {
	return st.proposers[addr]
}

func (st *status) IsScheduled(blockTime uint64, proposer thor.Address) bool {
	return st.scheduler.IsScheduled(blockTime, proposer)
}

func (st *status) Alpha() []byte {
	return st.alpha
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

			hash := ev.Draft.Hash()
			// skip if draft already seen(prevent DoS)
			if seenDraft.Contains(hash) {
				continue
			}
			seenDraft.Add(hash, struct{}{})

			signer, err := n.validateDraft(ev.Draft, st)
			if err != nil {
				log.Debug("validate draft failed", "err", err)
				continue
			}

			if st.parent.ID() != ev.Proposal.ParentID {
				unknownDraft.Set(hash, &draftWithSigner{
					Draft:  ev.Draft,
					Signer: signer,
				})
				continue
			}

			if err := n.processDraft(ev.Draft, signer, st); err != nil {
				log.Debug("failed to process draft", "err", err)
				continue
			}
			if signer == n.master.Address() {
				continue
			}
			if err := n.tryBacking(ev.Proposal.Hash(), st); err != nil {
				log.Debug("failed to back proposal", "err", err)
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

			if val, _, ok := knownProposal.Get(ev.ProposalHash); ok {
				if st.parent.ID() != val.(*block.Proposal).ParentID {
					continue
				}
				if err := n.processBackerSignature(ev.Accepted, pub, st); err != nil {
					log.Debug("failed to validate backer signature", "err", err)
					continue
				}
			} else {
				unknownAccepted.Set(hash, &acceptedWithPub{
					Accepted: ev.Accepted,
					Pub:      pub,
				})
			}
		case <-unknownTicker.C:
			parent := n.repo.BestBlock()

			if st == nil || st.parent.ID() != parent.Header().ID() {
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
				draft := ent.Value.(*draftWithSigner).Draft

				if math.Abs(float64(draft.Proposal.Number())-float64(st.parent.Number()+1)) > 3 {
					unknownDraft.Remove(ent.Key.(thor.Bytes32))
					continue
				}

				if st.parent.ID() == draft.Proposal.ParentID {
					unknownDraft.Remove(ent.Key.(thor.Bytes32))

					signer := ent.Value.(*draftWithSigner).Signer
					if err := n.processDraft(draft, signer, st); err != nil {
						log.Debug("failed to process draft", "err", err)
						continue
					}
					if signer == n.master.Address() {
						continue
					}
					if err := n.tryBacking(draft.Proposal.Hash(), st); err != nil {
						log.Debug("failed to back proposal", "err", err)
					}
				}
			}

			var aps []*cache.Entry
			unknownAccepted.ForEach(func(ent *cache.Entry) bool {
				aps = append(aps, ent)
				return true
			})

			for _, ent := range aps {
				accepted := ent.Value.(*acceptedWithPub).Accepted
				if val, _, ok := knownProposal.Get(accepted.ProposalHash); ok {
					unknownAccepted.Remove(ent.Key.(thor.Bytes32))

					if st.parent.ID() != val.(*block.Proposal).ParentID {
						continue
					}

					pub := ent.Value.(*acceptedWithPub).Pub
					if err := n.processBackerSignature(accepted, pub, st); err != nil {
						log.Debug("failed to validate backer signature", "err", err)
						continue
					}

				}
			}
		}
	}
}

func (n *Node) validateDraft(d *proto.Draft, st *status) (thor.Address, error) {
	p := d.Proposal

	if math.Abs(float64(p.Number())-float64(st.parent.Number()+1)) > 3 {
		return thor.Address{}, errors.New("obsolete proposal")
	}

	pub, err := crypto.SigToPub(p.Hash().Bytes(), d.Signature)
	if err != nil {
		return thor.Address{}, err
	}
	signer := thor.Address(crypto.PubkeyToAddress(*pub))

	if !st.IsAuthority(signer) {
		return thor.Address{}, errors.Errorf("proposer: %v is not an authority", signer)
	}

	// limit that proposer can propose only one proposal in a round
	var key [32]byte
	copy(key[:], signer.Bytes())
	binary.BigEndian.PutUint32(key[20:], p.Number())
	binary.BigEndian.PutUint64(key[24:], p.Timestamp)
	if seenDraft.Contains(key) {
		return thor.Address{}, errors.Errorf("proposer:%v already proposed in this round", signer)
	}
	seenDraft.Add(key, struct{}{})

	return signer, nil
}

func (n *Node) processDraft(d *proto.Draft, signer thor.Address, st *status) error {
	p := d.Proposal
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

	if !block.GasLimit(p.GasLimit).IsValid(st.parent.GasLimit()) {
		return errors.New("invalid gaslimit")
	}

	if !st.IsScheduled(p.Timestamp, signer) {
		return errors.New("proposal not scheduled")
	}

	knownProposal.Set(p.Hash(), p, float64(p.Timestamp))
	n.comm.BroadcastDraft(d)
	return nil
}

func (n *Node) tryBacking(proposalHash thor.Bytes32, st *status) error {
	if !st.IsAuthority(n.master.Address()) {
		return nil
	}

	beta, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(n.master.PrivateKey, st.Alpha())
	if err != nil {
		return err
	}

	if !poa.EvaluateVRF(beta, st.maxBlockProposers) {
		return errors.New("not lucky enough")
	}

	signature, err := crypto.Sign(proposalHash.Bytes(), n.master.PrivateKey)
	if err != nil {
		return err
	}

	bs, err := block.NewComplexSignature(proof, signature)
	if err != nil {
		return err
	}

	accepted := proto.Accepted{
		ProposalHash: proposalHash,
		Signature:    bs,
	}

	seenAccepted.Add(accepted.Hash(), struct{}{})
	n.comm.BroadcastAccepted(&accepted)
	return nil
}

func (n *Node) validateBacker(acc *proto.Accepted, st *status) (*ecdsa.PublicKey, error) {
	pub, err := crypto.SigToPub(acc.ProposalHash.Bytes(), acc.Signature.Signature())
	if err != nil {
		return nil, err
	}
	backer := thor.Address(crypto.PubkeyToAddress(*pub))

	if !st.IsAuthority(backer) {
		return nil, errors.Errorf("backer:%v is not an authority", backer)
	}

	return pub, nil
}

func (n *Node) processBackerSignature(acc *proto.Accepted, pub *ecdsa.PublicKey, st *status) error {
	beta, err := ecvrf.NewSecp256k1Sha256Tai().Verify(pub, st.Alpha(), acc.Signature.Proof())
	if err != nil {
		return err
	}

	if !poa.EvaluateVRF(beta, st.maxBlockProposers) {
		return fmt.Errorf("VRF output is not lucky enough to be a backer: %v", crypto.PubkeyToAddress(*pub))
	}
	n.comm.BroadcastAccepted(acc)
	return nil
}
