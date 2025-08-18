// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package delta

import "math/big"

type Renewal struct {
	NewLockedVET         *big.Int
	NewLockedWeight      *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}

func NewRenewal() *Renewal {
	return &Renewal{
		NewLockedVET:         big.NewInt(0),
		NewLockedWeight:      big.NewInt(0),
		QueuedDecrease:       big.NewInt(0),
		QueuedDecreaseWeight: big.NewInt(0),
	}
}

// Add sets r to the sum of itself and other.
func (r *Renewal) Add(other *Renewal) *Renewal {
	if other == nil {
		return r
	}

	r.NewLockedVET = big.NewInt(0).Add(r.NewLockedVET, other.NewLockedVET)
	r.NewLockedWeight = big.NewInt(0).Add(r.NewLockedWeight, other.NewLockedWeight)
	r.QueuedDecrease = big.NewInt(0).Add(r.QueuedDecrease, other.QueuedDecrease)
	r.QueuedDecreaseWeight = big.NewInt(0).Add(r.QueuedDecreaseWeight, other.QueuedDecreaseWeight)
	return r
}

type Exit struct {
	ExitedTVL            *big.Int
	ExitedTVLWeight      *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}

// Add sets e to the sum of itself and other.
func (e *Exit) Add(other *Exit) *Exit {
	if other == nil {
		return e
	}

	e.ExitedTVL = big.NewInt(0).Add(e.ExitedTVL, other.ExitedTVL)
	e.ExitedTVLWeight = big.NewInt(0).Add(e.ExitedTVLWeight, other.ExitedTVLWeight)
	e.QueuedDecrease = big.NewInt(0).Add(e.QueuedDecrease, other.QueuedDecrease)
	e.QueuedDecreaseWeight = big.NewInt(0).Add(e.QueuedDecreaseWeight, other.QueuedDecreaseWeight)
	return e
}
