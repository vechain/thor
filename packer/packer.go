// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

// Packer to pack txs and build new blocks.
type Packer struct {
	chain          *chain.Chain
	stateCreator   *state.Creator
	nodeMaster     thor.Address
	beneficiary    *thor.Address
	targetGasLimit uint64
}

// New create a new Packer instance.
// The beneficiary is optional, it defaults to endorsor if not set.
func New(
	chain *chain.Chain,
	stateCreator *state.Creator,
	nodeMaster thor.Address,
	beneficiary *thor.Address) *Packer {

	return &Packer{
		chain,
		stateCreator,
		nodeMaster,
		beneficiary,
		0,
	}
}

// Schedule schedule a packing flow to pack new block upon given parent and clock time.
func (p *Packer) Schedule(parent *block.Header, nowTimestamp uint64) (flow *Flow, err error) {
	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return nil, errors.Wrap(err, "state")
	}

	var (
		endorsement = builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
		authority   = builtin.Authority.Native(state)
		candidates  = authority.Candidates(endorsement, thor.MaxBlockProposers)
		proposers   = make([]poa.Proposer, 0, len(candidates))
		beneficiary thor.Address
	)
	if p.beneficiary != nil {
		beneficiary = *p.beneficiary
	}

	for _, c := range candidates {
		if p.beneficiary == nil && c.NodeMaster == p.nodeMaster {
			// not beneficiary not set, set it to endorsor
			beneficiary = c.Endorsor
		}
		proposers = append(proposers, poa.Proposer{
			Address: c.NodeMaster,
			Active:  c.Active,
		})
	}

	// calc the time when it's turn to produce block
	sched, err := poa.NewScheduler(p.nodeMaster, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		return nil, err
	}

	newBlockTime := sched.Schedule(nowTimestamp)
	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		authority.Update(u.Address, u.Active)
	}

	rt := runtime.New(
		p.chain.NewSeeker(parent.ID()),
		state,
		&xenv.BlockContext{
			Beneficiary: beneficiary,
			Signer:      p.nodeMaster,
			Number:      parent.Number() + 1,
			Time:        newBlockTime,
			GasLimit:    p.gasLimit(parent.GasLimit()),
			TotalScore:  parent.TotalScore() + score,
		})

	return newFlow(p, parent, rt), nil
}

// Mock create a packing flow upon given parent, but with a designated timestamp.
// It will skip the PoA verification and scheduling, and the block produced by
// the returned flow is not in consensus.
func (p *Packer) Mock(parent *block.Header, targetTime uint64) (*Flow, error) {
	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return nil, errors.Wrap(err, "state")
	}

	rt := runtime.New(
		p.chain.NewSeeker(parent.ID()),
		state,
		&xenv.BlockContext{
			Beneficiary: p.nodeMaster,
			Signer:      p.nodeMaster,
			Number:      parent.Number() + 1,
			Time:        targetTime,
			GasLimit:    p.gasLimit(parent.GasLimit()),
			TotalScore:  parent.TotalScore() + 1,
		})

	return newFlow(p, parent, rt), nil
}

func (p *Packer) gasLimit(parentGasLimit uint64) uint64 {
	if p.targetGasLimit != 0 {
		return block.GasLimit(p.targetGasLimit).Qualify(parentGasLimit)
	}
	return parentGasLimit
}

// SetTargetGasLimit set target gas limit, the Packer will adjust block gas limit close to
// it as it can.
func (p *Packer) SetTargetGasLimit(gl uint64) {
	p.targetGasLimit = gl
}
