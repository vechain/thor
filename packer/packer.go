// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/scheduler"
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
	seeder           *scheduler.Seeder
	minTxPriorityFee *big.Int
}

// New create a new Packer instance.
// The beneficiary is optional, it defaults to endorser if not set.
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
		scheduler.NewSeeder(repo),
		new(big.Int).SetUint64(minTxPriorityFee),
	}
}

type scheduleFunc func(parent *chain.BlockSummary, nowTimestamp uint64, state *state.State) (thor.Address, uint64, uint64, error)

// Schedule a packing flow to pack new block upon given parent and clock time.
func (p *Packer) Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (*Flow, bool, error) {
	st := p.stater.NewState(parent.Root())

	var features tx.Features
	if parent.Header.Number()+1 >= p.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	var sched scheduleFunc
	checkpoint := st.NewCheckpoint()
	dPosStatus, err := builtin.Staker.Native(st).SyncPOS(p.forkConfig, parent.Header.Number()+1)
	if err != nil {
		log.Error("staker sync pos failed - reverting state", "err", err, "height", parent.Header.Number()+1, "parent", parent, "checkpoint", checkpoint)
		st.RevertTo(checkpoint)
	}
	if dPosStatus.Active {
		sched = p.schedulePOS
	} else {
		sched = p.schedulePOA
	}

	beneficiary, newBlockTime, score, err := sched(parent, nowTimestamp, st)
	if err != nil {
		return nil, false, err
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

	return newFlow(p, parent.Header, rt, features, dPosStatus.Active), dPosStatus.Active, nil
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

	staker := builtin.Staker.Native(state)
	dPosStatus, err := staker.SyncPOS(p.forkConfig, parent.Header.Number()+1)
	if err != nil {
		return nil, false, err
	}

	beneficiary := p.beneficiary

	var score uint64
	if dPosStatus.Active {
		leaders, err := staker.LeaderGroup()
		if err != nil {
			return nil, false, err
		}
		_, totalWeight, err := staker.LockedStake()
		if err != nil {
			return nil, false, err
		}

		onlineWeight := uint64(0)
		for _, leader := range leaders {
			if leader.Active {
				onlineWeight += leader.Weight
			}
			if leader.Address == p.nodeMaster {
				if leader.Beneficiary != nil {
					beneficiary = leader.Beneficiary
				} else if beneficiary == nil {
					beneficiary = &leader.Endorser
				}
			}
		}
		score = onlineWeight * thor.MaxPosScore / totalWeight
	} else {
		endorsement, err := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
		if err != nil {
			return nil, false, err
		}
		checker := staker.TransitionPeriodBalanceCheck(p.forkConfig, parent.Header.Number()+1, endorsement)
		authorities, err := builtin.Authority.Native(state).Candidates(checker, thor.InitialMaxBlockProposers)
		if err != nil {
			return nil, false, err
		}
		for _, authority := range authorities {
			if authority.Active {
				score++
			}
			if beneficiary == nil && authority.NodeMaster == p.nodeMaster {
				beneficiary = &authority.Endorsor
			}
		}
	}
	if beneficiary == nil {
		return nil, false, errors.New("no beneficiary set, cannot pack block")
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
			Beneficiary: *beneficiary,
			Signer:      p.nodeMaster,
			Number:      parent.Header.Number() + 1,
			Time:        targetTime,
			GasLimit:    gl,
			TotalScore:  parent.Header.TotalScore() + score,
			BaseFee:     baseFee,
		},
		p.forkConfig)

	return newFlow(p, parent.Header, rt, features, dPosStatus.Active), dPosStatus.Active, nil
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
