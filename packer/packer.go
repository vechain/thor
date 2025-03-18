// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
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
	forkConfig thor.ForkConfig,
) *Packer {
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

// Schedule a packing flow to pack new block upon given parent and clock time.
func (p *Packer) Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (*Flow, error) {
	st := p.stater.NewState(parent.Root())

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	var (
		beneficiary  thor.Address
		newBlockTime uint64
		score        uint64
		err          error
	)

	if parent.Header.Number()+1 <= p.forkConfig.HAYABUSA {
		beneficiary, newBlockTime, score, err = p.schedulePOA(parent, nowTimestamp, st)
	} else {
		beneficiary, newBlockTime, score, err = p.schedulePOS(parent, nowTimestamp, st)
	}
	if err != nil {
		return nil, err
	}

	rt := runtime.New(
		p.repo.NewChain(parent.Header.ID()),
		st,
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
	state := p.stater.NewState(parent.Root())

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	var score uint64
	if p.forkConfig.HAYABUSA < parent.Header.Number()+1 {
		leaders, err := builtin.Staker.Native(state).LeaderGroup()
		if err != nil {
			return nil, err
		}
		for _, leader := range leaders {
			if leader.Online {
				score++
			}
		}
	} else {
		authorities, err := builtin.Authority.Native(state).Candidates(big.NewInt(0), thor.InitialMaxBlockProposers)
		if err != nil {
			return nil, err
		}
		for _, authority := range authorities {
			if authority.Active {
				score++
			}
		}
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
			TotalScore:  parent.Header.TotalScore() + score,
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
