// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/pos"
)

func (c *Consensus) validateStakingProposer(header *block.Header, parent *block.Header, staker *staker.Staker) error {
	signer, err := header.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("pos - block signer unavailable: %v", err))
	}

	var seed []byte
	seed, err = c.seeder.Generate(header.ParentID())
	if err != nil {
		return err
	}

	leaders, err := staker.LeaderGroup()
	if err != nil {
		return err
	}
	sched, err := pos.NewScheduler(signer, leaders, parent.Number(), parent.Timestamp(), seed)
	if err != nil {
		return consensusError(fmt.Sprintf("pos - block signer invalid: %v %v", signer, err))
	}
	if !sched.IsTheTime(header.Timestamp()) {
		return consensusError(fmt.Sprintf("pos - block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}
	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return consensusError(fmt.Sprintf("pos - block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
	}

	for addr, online := range updates {
		if err := staker.SetOnline(addr, online); err != nil {
			return err
		}
	}

	return nil
}
