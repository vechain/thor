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

	var beneficiary thor.Address
	if p.beneficiary != nil {
		beneficiary = *p.beneficiary
	} else {
		validator, _, err := staker.LookupMaster(p.nodeMaster)
		if err != nil {
			return thor.Address{}, 0, 0, err
		}
		if validator.IsEmpty() {
			return thor.Address{}, 0, 0, errNotScheduled
		}
		beneficiary = validator.Endorsor
	}

	var seed []byte
	seed, err := p.seeder.Generate(parent.Header.ID())
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	leaderGroup, err := staker.LeaderGroup()
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	sched, err := pos.NewScheduler(p.nodeMaster, leaderGroup, parent.Header.Number(), parent.Header.Timestamp(), seed)
	if err != nil {
		return thor.Address{}, 0, 0, err
	}
	newBlockTime, err := sched.Schedule(nowTimestamp)
	if err != nil {
		return thor.Address{}, 0, 0, errNotScheduled
	}
	updates, score := sched.Updates(newBlockTime)

	for addr, online := range updates {
		if err := staker.SetOnline(addr, online); err != nil {
			return thor.Address{}, 0, 0, err
		}
	}

	return beneficiary, newBlockTime, score, nil
}
