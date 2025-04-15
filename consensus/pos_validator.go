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

	var seed []byte
	seed, err = c.seeder.Generate(header.ParentID())
	if err != nil {
		return nil, err
	}

	sched, err := pos.NewScheduler(signer, proposers, parent.Number(), parent.Timestamp(), seed)
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

	for addr, online := range updates {
		if err := stakerContract.SetOnline(addr, online); err != nil {
			return nil, err
		}
	}

	// Perform validator housekeeping on epoch boundaries
	headerNumber := header.Number()
	updateValidators, err := stakerContract.Housekeep(headerNumber)
	if err != nil {
		return nil, err
	}
	if updateValidators || len(updates) > 0 {
		// Refresh validators from contract since they've changed
		validators.InvalidateCache()
	}

	return c.validatorCacheHandler(validators, header), nil
}

func (c *Consensus) validatorCacheHandler(validators *pos.Validators, header *block.Header) cacheHandler {
	return func(receipts tx.Receipts) error {
		c.validatorCache.Add(header.ID(), validators)
		return nil
	}
}
