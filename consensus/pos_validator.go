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

func (c *Consensus) validateStakingProposer(header *block.Header, parent *block.Header, st *state.State) error {
	signer, err := header.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}
	staker := builtin.Staker.Native(st)

	// TODO: Add caching for leaderGroup, similar to candidatesCache & poa.NewCandidates(list)
	// https://github.com/vechain/protocol-board-repo/issues/430
	leaderGroup, err := staker.LeaderGroup()
	if err != nil {
		return err
	}

	// TODO: We're copying the old validator selector, not based on weight. This is a temporary solution that will be iterated.
	// TODO: See also packer/pos_scheduler.go
	// https://github.com/vechain/protocol-board-repo/issues/429
	var seed []byte
	seed, err = c.seeder.Generate(header.ParentID())
	if err != nil {
		return err
	}

	sched, err := pos.NewScheduler(signer, leaderGroup, parent.Number(), parent.Timestamp(), seed)
	if err != nil {
		return consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
	}

	for _, addr := range updates {
		if err := staker.IncrementMissedSlot(addr); err != nil {
			return err
		}
	}

	return nil
}

func (c *Consensus) stakerReceiptsHandler() func(receipts tx.Receipts) error {
	// TODO Check the queue if we're @ an epoch block and implement post block logic
	// https://github.com/vechain/protocol-board-repo/issues/431
	return func(receipts tx.Receipts) error {
		return nil
	}
}
