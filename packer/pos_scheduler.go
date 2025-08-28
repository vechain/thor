// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func (p *Packer) schedulePOS(parent *chain.BlockSummary, nowTimestamp uint64, state *state.State) (thor.Address, uint64, uint64, error) {
	staker := builtin.Staker.Native(state)

	var seed []byte
	seed, err := p.seeder.Generate(parent.Header.ID())
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	leaderGroup, err := staker.LeaderGroup()
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	validator, ok := leaderGroup[p.nodeMaster]
	if !ok {
		return thor.Address{}, 0, 0, errNotScheduled
	}
	sched, err := pos.NewScheduler(p.nodeMaster, leaderGroup, parent.Header.Number(), parent.Header.Timestamp(), seed)
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	newBlockTime := sched.Schedule(nowTimestamp)

	_, weight, err := staker.LockedStake()
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	updates, score := sched.Updates(newBlockTime, weight)

	for addr, online := range updates {
		if err := staker.SetOnline(addr, parent.Header.Number()+1, online); err != nil {
			return thor.Address{}, 0, 0, err
		}
	}

	var beneficiary thor.Address
	if validator.Beneficiary != nil { // contract beneficiary get's validated in consensus
		beneficiary = *validator.Beneficiary
	} else if p.beneficiary != nil {
		beneficiary = *p.beneficiary
	} else {
		beneficiary = validator.Endorser
	}

	return beneficiary, newBlockTime, score, nil
}
