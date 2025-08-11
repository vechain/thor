// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package delegation

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

type Delegation struct {
	Validator      thor.Address // the validator to which the delegator is delegating
	Stake          *big.Int
	Multiplier     uint8
	LastIteration  *uint32 `rlp:"nil"` // the last staking period in which the delegator was active
	FirstIteration uint32  // the iteration at which the delegator was created
}

// IsEmpty returns whether the entry can be treated as empty.
func (d *Delegation) IsEmpty() bool {
	return (d.Stake == nil || d.Stake.Sign() == 0) && d.Multiplier == 0
}

// WeightedStake returns the weight of the delegator, which is calculated as:
// weight = stake * multiplier / 100
func (d *Delegation) WeightedStake() *stakes.WeightedStake {
	if d.IsEmpty() {
		return stakes.NewWeightedStake(big.NewInt(0), 0)
	}
	return stakes.NewWeightedStake(d.Stake, d.Multiplier)
}

// Started returns whether the delegation became locked
func (d *Delegation) Started(val *validation.Validation) bool {
	if d.IsEmpty() {
		return false
	}
	if val.Status == validation.StatusQueued {
		return false // Delegation cannot start if the validation is not active
	}
	return val.CurrentIteration() >= d.FirstIteration
}

// Ended returns whether the delegation has ended
// It returns true if:
// - the delegation's exit iteration is less than the current staking period
// - OR if the validation is in exit status and the delegation has started
func (d *Delegation) Ended(val *validation.Validation) bool {
	if d.IsEmpty() {
		return false
	}
	if val.Status == validation.StatusQueued {
		return false // Delegation cannot end if the validation is not active
	}
	if val.Status == validation.StatusExit && d.Started(val) {
		return true // Delegation is ended if the validation is in exit status
	}
	if d.LastIteration == nil {
		return false
	}
	return *d.LastIteration < val.CurrentIteration()
}
