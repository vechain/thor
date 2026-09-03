// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	stakerContract "github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/scheduler"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func (c *Consensus) validateStakingProposer(
	header *block.Header,
	parent *block.Header,
	staker *stakerContract.Staker,
) (cacher, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, consensusError(fmt.Sprintf("pos - block signer unavailable: %v", err))
	}

	var seed []byte
	seed, err = c.seeder.Generate(header.ParentID())
	if err != nil {
		return nil, err
	}
	var leaders []validation.Leader
	if cached, ok := c.validatorsCache.Get(header.ParentID()); ok {
		if cachedLeaders, ok := cached.([]validation.Leader); ok {
			leaders = cachedLeaders
		}
	}
	// not cached
	if len(leaders) == 0 {
		leaders, err = staker.LeaderGroup()
		if err != nil {
			return nil, consensusError(fmt.Sprintf("pos - cannot get leader group: %v", err))
		}
	}

	var (
		proposers   = make([]scheduler.Proposer, 0, len(leaders))
		beneficiary *thor.Address
	)
	for _, leader := range leaders {
		if leader.Address == signer && leader.Beneficiary != nil {
			beneficiary = leader.Beneficiary
		}
		proposers = append(proposers, scheduler.Proposer{
			Address: leader.Address,
			Active:  leader.Active,
			Weight:  leader.Weight,
		})
	}
	_, totalWeight, err := staker.LockedStake()
	if err != nil {
		return nil, consensusError(fmt.Sprintf("pos - cannot get total weight: %v", err))
	}

	sched, err := scheduler.NewPoSScheduler(signer, proposers, parent.Number(), parent.Timestamp(), seed, totalWeight)
	if err != nil {
		return nil, consensusError(fmt.Sprintf("pos - block signer invalid: %v %v", signer, err))
	}
	if !sched.IsTheTime(header.Timestamp()) {
		return nil, consensusError(fmt.Sprintf("pos - block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}
	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return nil, consensusError(fmt.Sprintf("pos - block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
	}
	if beneficiary != nil && *beneficiary != header.Beneficiary() {
		return nil, consensusError(fmt.Sprintf("pos - stake beneficiary mismatch: want %v, have %v", *beneficiary, header.Beneficiary()))
	}

	for _, u := range updates {
		if err := staker.SetOnline(u.Address, header.Number(), u.Active); err != nil {
			return nil, err
		}
	}

	if len(updates) > 0 {
		return &noOpCacher{}, nil
	}

	return &posCacher{leaderGroup: leaders}, nil
}

type posCacher struct {
	leaderGroup []validation.Leader
}

var _ cacher = (*posCacher)(nil)

func (p *posCacher) Handle(_ *block.Header, receipts tx.Receipts) (any, error) {
	beneficiaryABI, ok := builtin.Staker.Events().EventByName("BeneficiarySet")
	if !ok {
		return nil, fmt.Errorf("pos - cannot get BeneficiarySet event")
	}

	// if there is no beneficiary change event, skip the cache update
	for _, r := range receipts {
		for _, o := range r.Outputs {
			for _, ev := range o.Events {
				if ev.Address == builtin.Staker.Address && ev.Topics[0] == beneficiaryABI.ID() {
					return nil, nil // skip the cache update
				}
			}
		}
	}

	return p.leaderGroup, nil // no beneficiary change, reuse the leader group
}

type noOpCacher struct{}

var _ cacher = (*noOpCacher)(nil)

func (n *noOpCacher) Handle(_ *block.Header, _ tx.Receipts) (any, error) {
	return nil, nil
}
