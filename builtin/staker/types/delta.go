// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package types

type Renewal struct {
	LockedIncrease *WeightedStake
	LockedDecrease *WeightedStake
	QueuedDecrease *WeightedStake
}

func NewRenewal() *Renewal {
	return &Renewal{
		LockedIncrease: &WeightedStake{},
		LockedDecrease: &WeightedStake{},
		QueuedDecrease: &WeightedStake{},
	}
}

// Add sets r to the sum of itself and other.
func (r *Renewal) Add(other *Renewal) *Renewal {
	if other == nil {
		return r
	}

	r.LockedIncrease.Add(other.LockedIncrease)
	r.LockedDecrease.Add(other.LockedDecrease)
	r.QueuedDecrease.Add(other.QueuedDecrease)
	return r
}

type Exit struct {
	ExitedTVL      *WeightedStake
	QueuedDecrease *WeightedStake
}

// Add sets e to the sum of itself and other.
func (e *Exit) Add(other *Exit) *Exit {
	if other == nil {
		return e
	}

	e.ExitedTVL.Add(other.ExitedTVL)
	e.QueuedDecrease.Add(other.QueuedDecrease)
	return e
}
