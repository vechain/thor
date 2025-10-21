// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func (p *Packer) schedulePOA(parent *chain.BlockSummary, nowTimestamp uint64, state *state.State) (thor.Address, uint64, uint64, error) {
	authority := builtin.Authority.Native(state)
	endorsement, err := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	staker := builtin.Staker.Native(state)

	mbp, err := builtin.Params.Native(state).Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return thor.Address{}, 0, 0, err
	}

	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 || maxBlockProposers > thor.InitialMaxBlockProposers {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}

	balanceCheck := staker.TransitionPeriodBalanceCheck(p.forkConfig, parent.Header.Number()+1, endorsement)
	candidates, err := authority.Candidates(balanceCheck, maxBlockProposers)
	if err != nil {
		return thor.Address{}, 0, 0, err
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
			// no beneficiary not set, set it to endorser
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
			return thor.Address{}, 0, 0, err
		}
		sched, err = poa.NewSchedulerV2(p.nodeMaster, proposers, parent.Header.Number(), parent.Header.Timestamp(), seed)
	}
	if err != nil {
		return thor.Address{}, 0, 0, err
	}

	newBlockTime := sched.Schedule(nowTimestamp)
	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		if _, err := authority.Update(u.Address, u.Active); err != nil {
			return thor.Address{}, 0, 0, err
		}
	}

	return beneficiary, newBlockTime, score, nil
}
