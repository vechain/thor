// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/scheduler"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func (p *Packer) schedulePOS(parent *chain.BlockSummary, nowTimestamp uint64, state *state.State) (thor.Address, uint64, uint64, error) {
	staker := builtin.Staker.Native(state)

	seed, err := p.seeder.Generate(parent.Header.ID())
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	leaderGroup, err := staker.LeaderGroup()
	if err != nil {
		return thor.Address{}, 0, 0, err
	}

	var (
		beneficiary thor.Address
		proposers   = make([]scheduler.Proposer, 0, len(leaderGroup))
	)

	for _, leader := range leaderGroup {
		if leader.Address == p.nodeMaster {
			if leader.Beneficiary != nil { // staker contract beneficiary first
				beneficiary = *leader.Beneficiary
			} else if p.beneficiary != nil { // packer beneficiary option second
				beneficiary = *p.beneficiary
			} else { // fallback to endorser
				beneficiary = leader.Endorser
			}
		}

		proposers = append(proposers, scheduler.Proposer{
			Address: leader.Address,
			Active:  leader.Active,
			Weight:  leader.Weight,
		})
	}
	_, weight, err := staker.LockedStake()
	if err != nil {
		return thor.Address{}, 0, 0, err
	}

	sched, err := scheduler.NewPoSScheduler(p.nodeMaster, proposers, parent.Header.Number(), parent.Header.Timestamp(), seed, weight)
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	newBlockTime := sched.Schedule(nowTimestamp)
	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		if err := staker.SetOnline(u.Address, parent.Header.Number()+1, u.Active); err != nil {
			return thor.Address{}, 0, 0, err
		}
	}

	return beneficiary, newBlockTime, score, nil
}
