// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	stakerContract "github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/thor"
)

func (c *Consensus) validateStakingProposer(
	header *block.Header,
	parent *block.Header,
	staker *stakerContract.Staker,
	providedLeaders map[thor.Address]*validation.Validation,
) error {
	signer, err := header.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("pos - block signer unavailable: %v", err))
	}

	var seed []byte
	seed, err = c.seeder.Generate(header.ParentID())
	if err != nil {
		return err
	}

	var leaders map[thor.Address]*validation.Validation
	if entry, ok := c.validatorsCache.Get(parent.ID()); ok {
		possiblyLeaders, ok := entry.(*map[thor.Address]*validation.Validation)
		if ok {
			leaders = *possiblyLeaders
		} else {
			leaders, err = staker.LeaderGroup()
			if err != nil {
				return err
			}
		}
	} else {
		if len(providedLeaders) > 0 {
			leaders = providedLeaders
		} else {
			leaders, err = staker.LeaderGroup()
			if err != nil {
				return err
			}
		}
	}
	sched, err := pos.NewScheduler(signer, leaders, parent.Number(), parent.Timestamp(), seed)
	if err != nil {
		return consensusError(fmt.Sprintf("pos - block signer invalid: %v %v", signer, err))
	}
	if !sched.IsTheTime(header.Timestamp()) {
		return consensusError(fmt.Sprintf("pos - block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}

	_, totalWeight, err := staker.LockedVET()
	if err != nil {
		return consensusError(fmt.Sprintf("pos - cannot get total weight: %v", err))
	}
	updates, score := sched.Updates(header.Timestamp(), totalWeight)
	if parent.TotalScore()+score != header.TotalScore() {
		return consensusError(fmt.Sprintf("pos - block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
	}
	validator, ok := leaders[signer]
	if !ok {
		return consensusError(fmt.Sprintf("pos - block proposer %v not found in leader group", signer))
	}
	if validator.Beneficiary != nil && *validator.Beneficiary != header.Beneficiary() {
		return consensusError(fmt.Sprintf("pos - stake beneficiary mismatch: want %v, have %v", *validator.Beneficiary, header.Beneficiary()))
	}

	hasUpdates := false
	for addr, online := range updates {
		updated, err := staker.SetOnline(addr, header.Number(), online)
		if err != nil {
			return err
		}
		if updated {
			hasUpdates = true
		}
	}
	if !hasUpdates {
		c.validatorsCache.Add(header.ID(), leaders)
	}

	return nil
}
