// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

// Packer to pack txs and build new blocks.
type Packer struct {
	repo           *chain.Repository
	stater         *state.Stater
	nodeMaster     thor.Address
	beneficiary    *thor.Address
	targetGasLimit uint64
	forkConfig     thor.ForkConfig
	seeder         *poa.Seeder
}

// New create a new Packer instance.
// The beneficiary is optional, it defaults to endorsor if not set.
func New(
	repo *chain.Repository,
	stater *state.Stater,
	nodeMaster thor.Address,
	beneficiary *thor.Address,
	forkConfig thor.ForkConfig) *Packer {

	return &Packer{
		repo,
		stater,
		nodeMaster,
		beneficiary,
		0,
		forkConfig,
		poa.NewSeeder(repo),
	}
}

// Schedule schedule a packing flow to pack new block upon given parent and clock time.
func (p *Packer) Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (flow *Flow, err error) {
	state := p.stater.NewState(parent.Header.StateRoot(), parent.Header.Number(), parent.Conflicts, parent.SteadyNum)

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

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
	if maxBlockProposers == 0 || maxBlockProposers > thor.InitialMaxBlockProposers {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}

	candidates, err := authority.Candidates(endorsement, maxBlockProposers)
	if err != nil {
		return nil, err
	}
	var (
		proposers   = make([]poa.Proposer, 0, len(candidates))
		beneficiary thor.Address
	)
	if p.beneficiary != nil {
		beneficiary = *p.beneficiary
	}

	for _, c := range candidates {
		if p.beneficiary == nil && c.NodeMaster == p.nodeMaster {
			// no beneficiary not set, set it to endorsor
			beneficiary = c.Endorsor
		}
		proposers = append(proposers, poa.Proposer{
			Address: c.NodeMaster,
			Active:  c.Active,
		})
	}

	// calc the time when it's turn to produce block
	var sched poa.Scheduler
	if parent.Header.Number()+1 < p.forkConfig.VIP214 {
		sched, err = poa.NewSchedulerV1(p.nodeMaster, proposers, parent.Header.Number(), parent.Header.Timestamp())
	} else {
		var seed []byte
		seed, err = p.seeder.Generate(parent.Header.ID())
		if err != nil {
			return nil, err
		}
		sched, err = poa.NewSchedulerV2(p.nodeMaster, proposers, parent.Header.Number(), parent.Header.Timestamp(), seed)
	}
	if err != nil {
		return nil, err
	}

	newBlockTime := sched.Schedule(nowTimestamp)
	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		if _, err := authority.Update(u.Address, u.Active); err != nil {
			return nil, err
		}
	}

	rt := runtime.New(
		p.repo.NewChain(parent.Header.ID()),
		state,
		&xenv.BlockContext{
			Beneficiary: beneficiary,
			Signer:      p.nodeMaster,
			Number:      parent.Header.Number() + 1,
			Time:        newBlockTime,
			GasLimit:    p.gasLimit(parent.Header.GasLimit()),
			TotalScore:  parent.Header.TotalScore() + score,
		},
		p.forkConfig)

	return newFlow(p, parent.Header, rt, features), nil
}

// Mock create a packing flow upon given parent, but with a designated timestamp.
// It will skip the PoA verification and scheduling, and the block produced by
// the returned flow is not in consensus.
func (p *Packer) Mock(parent *chain.BlockSummary, targetTime uint64, gasLimit uint64) (*Flow, error) {
	state := p.stater.NewState(parent.Header.StateRoot(), parent.Header.Number(), parent.Conflicts, parent.SteadyNum)

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	gl := gasLimit
	if gasLimit == 0 {
		gl = p.gasLimit(parent.Header.GasLimit())
	}

	rt := runtime.New(
		p.repo.NewChain(parent.Header.ID()),
		state,
		&xenv.BlockContext{
			Beneficiary: p.nodeMaster,
			Signer:      p.nodeMaster,
			Number:      parent.Header.Number() + 1,
			Time:        targetTime,
			GasLimit:    gl,
			TotalScore:  parent.Header.TotalScore() + 1,
		},
		p.forkConfig)

	return newFlow(p, parent.Header, rt, features), nil
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
