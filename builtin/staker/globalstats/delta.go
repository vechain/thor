// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"errors"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
)

type Renewal struct {
	LockedIncrease *stakes.WeightedStake
	LockedDecrease *stakes.WeightedStake
	QueuedDecrease uint64 // stake/VET only
}

func NewRenewal() *Renewal {
	return &Renewal{
		LockedIncrease: &stakes.WeightedStake{},
		LockedDecrease: &stakes.WeightedStake{},
		QueuedDecrease: 0,
	}
}

// Add sets r to the sum of itself and other.
func (r *Renewal) Add(other *Renewal) (*Renewal, error) {
	if other == nil {
		return r, nil
	}

	if err := r.LockedIncrease.Add(other.LockedIncrease); err != nil {
		return nil, err
	}
	if err := r.LockedDecrease.Add(other.LockedDecrease); err != nil {
		return nil, err
	}

	var overflow bool
	if r.QueuedDecrease, overflow = math.SafeAdd(r.QueuedDecrease, other.QueuedDecrease); overflow {
		return nil, errors.New("queued decrease overflow occurred")
	}

	return r, nil
}

type Exit struct {
	ExitedTVL      *stakes.WeightedStake
	QueuedDecrease uint64 // stake/VET only
}
