// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/tx"
)

func (c *Consensus) validateStakingProposer(header *block.Header, parent *block.Header, st *state.State) (cacheHandler, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}
	stakerContract := builtin.Staker.Native(st)
	activeStake, err := stakerContract.ActiveStake()
	if err != nil {
		return nil, err
	}

	var validators *pos.Validators
	if entry, ok := c.validatorCache.Get(parent.ID()); ok {
		validators = entry.(*pos.Validators).Copy()
	} else {
		leaders, err := stakerContract.LeaderGroup()
		if err != nil {
			return nil, err
		}
		validators = pos.NewValidators(leaders)
	}
	proposers, err := validators.Pick(st)
	if err != nil {
		return nil, err
	}

	// TODO: We're using the same seed mechanism as PoA. Should we use a different one?
	// TODO: See also packer/pos_scheduler.go
	// https://github.com/vechain/protocol-board-repo/issues/442
	var seed []byte
	seed, err = c.seeder.Generate(header.ParentID())
	if err != nil {
		return nil, err
	}

	sched, err := pos.NewScheduler(signer, proposers, activeStake, parent.Number(), parent.Timestamp(), seed)
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}
	if !sched.IsTheTime(header.Timestamp()) {
		return nil, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}
	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return nil, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
	}

	for _, addr := range updates {
		if err := stakerContract.IncrementMissedSlot(addr); err != nil {
			return nil, err
		}
	}

	// TODO: Call the staker.Housekeeping function and tidy up the pos.Validators
	// https://github.com/vechain/protocol-board-repo/issues/443

	return c.validatorCacheHandler(validators, header), nil
}

func (c *Consensus) validatorCacheHandler(validators *pos.Validators, header *block.Header) cacheHandler {
	return func(receipts tx.Receipts) error {
		c.validatorCache.Add(header.ID(), validators)
		return nil
	}
}
