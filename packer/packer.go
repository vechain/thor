// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// Packer to pack txs and build new blocks.
type Packer struct {
	repo             *chain.Repository
	stater           *state.Stater
	nodeMaster       thor.Address
	beneficiary      *thor.Address
	targetGasLimit   uint64
	forkConfig       *thor.ForkConfig
	seeder           *poa.Seeder
	minTxPriorityFee *big.Int
}

// New create a new Packer instance.
// The beneficiary is optional, it defaults to endorsor if not set.
func New(
	repo *chain.Repository,
	stater *state.Stater,
	nodeMaster thor.Address,
	beneficiary *thor.Address,
	forkConfig *thor.ForkConfig,
	minTxPriorityFee uint64,
) *Packer {
	return &Packer{
		repo,
		stater,
		nodeMaster,
		beneficiary,
		0,
		forkConfig,
		poa.NewSeeder(repo),
		new(big.Int).SetUint64(minTxPriorityFee),
	}
}

type scheduler func(parent *chain.BlockSummary, nowTimestamp uint64, state *state.State) (thor.Address, uint64, uint64, error)

// Schedule a packing flow to pack new block upon given parent and clock time.
func (p *Packer) Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (*Flow, bool, error) {
	st := p.stater.NewState(parent.Root())

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	var sched scheduler
	posActive, activated, err := p.syncPOS(st, parent.Header.Number()+1)
	if err != nil {
		return nil, false, err
	}
	if posActive {
		sched = p.schedulePOS
	} else {
		sched = p.schedulePOA
	}

	beneficiary, newBlockTime, score, err := sched(parent, nowTimestamp, st)
	if err != nil {
		return nil, false, err
	}

	if activated {
		err := builtin.Energy.Native(st, parent.Header.Timestamp()).StopEnergyGrowth()
		if err != nil {
			return nil, false, err
		}
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
			BaseFee:     galactica.CalcBaseFee(parent.Header, p.forkConfig),
		},
		p.forkConfig)

	return newFlow(p, parent.Header, rt, features, posActive), posActive, nil
}

// Mock create a packing flow upon given parent, but with a designated timestamp.
// It will skip the PoA verification and scheduling, and the block produced by
// the returned flow is not in consensus.
func (p *Packer) Mock(parent *chain.BlockSummary, targetTime uint64, gasLimit uint64) (*Flow, bool, error) {
	state := p.stater.NewState(parent.Root())

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	posActive, activated, err := p.syncPOS(state, parent.Header.Number()+1)
	if err != nil {
		return nil, false, err
	}

	if activated {
		err := builtin.Energy.Native(state, parent.Header.Timestamp()).StopEnergyGrowth()
		if err != nil {
			return nil, false, err
		}
	}

	var score uint64
	if posActive {
		leaders, err := builtin.Staker.Native(state).LeaderGroup()
		if err != nil {
			return nil, false, err
		}
		for _, leader := range leaders {
			if leader.Online {
				score++
			}
		}
	} else {
		authorities, err := builtin.Authority.Native(state).Candidates(big.NewInt(0), thor.InitialMaxBlockProposers)
		if err != nil {
			return nil, false, err
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

	var baseFee *big.Int
	if parent.Header.Number()+1 >= p.forkConfig.GALACTICA {
		baseFee = galactica.CalcBaseFee(parent.Header, p.forkConfig)
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
			BaseFee:     baseFee,
		},
		p.forkConfig)

	return newFlow(p, parent.Header, rt, features, posActive), posActive, nil
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

// syncPOS checks if POS consensus is active, or tries to activate it if conditions are met.
// If the staker contract is active, it will perform housekeeping.
func (p *Packer) syncPOS(st *state.State, current uint32) (active bool, activated bool, err error) {
	// still on PoA
	if p.forkConfig.HAYABUSA+p.forkConfig.HAYABUSA_TP > current {
		return false, false, nil
	}
	// check if the staker contract currently is active
	staker := builtin.Staker.Native(st)
	active, err = staker.IsPoSActive()
	if err != nil {
		return false, false, err
	}

	// attempt to transition if we're on a transition block and the staker contract is not active
	if !active && current%p.forkConfig.HAYABUSA_TP == 0 {
		activated, err = staker.Transition(current)
		if err != nil {
			return false, false, err
		}
		if activated {
			log.Info("dPoS activated", "pkg", "packer", "block", current)
			return true, true, nil
		}
	}

	// perform housekeeping if the staker contract is active
	if active {
		_, _, err := staker.Housekeep(current)
		if err != nil {
			return false, false, err
		}
		return true, false, nil
	}

	return active, false, nil
}
