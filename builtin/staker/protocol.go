// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"fmt"

	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

type Status struct {
	Active      bool                                    // indicates if the staker contract is currently active
	LeaderGroup map[thor.Address]*validation.Validation // the current leader group, if the staker contract is active
}

// EvaluateOrUpdate checks the status of the staker contract and updates its state based on the current block number.
// It returns a Status object containing the activation status and the current leader group.
// If the staker contract is not active, it attempts to transition to dPoS on transition blocks.
// If the staker contract is active, it performs housekeeping on epoch blocks.
func (s *Staker) EvaluateOrUpdate(forkConfig *thor.ForkConfig, current uint32) (*Status, error) {
	// still on PoA
	if forkConfig.HAYABUSA+forkConfig.HAYABUSA_TP > current {
		return &Status{Active: false}, nil
	}

	var err error
	var activated bool
	status := &Status{
		LeaderGroup: make(map[thor.Address]*validation.Validation),
	}

	// check if the staker contract is active
	status.Active, err = s.IsPoSActive()
	if err != nil {
		return nil, err
	}

	// attempt to transition if we're on a transition block and the staker contract is not active
	if !status.Active && current%forkConfig.HAYABUSA_TP == 0 {
		activated, err = s.transition(current)
		if err != nil {
			return nil, fmt.Errorf("failed to transition to dPoS: %w", err)
		}
		if activated {
			status.Active = true
		}
	}

	// perform housekeeping if the staker contract is active
	if status.Active && !activated {
		_, status.LeaderGroup, err = s.Housekeep(current)
		if err != nil {
			return nil, err
		}
	}

	return status, nil
}
