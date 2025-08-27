// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
)

// Aggregation represents the total amount of VET locked for a given validation's delegations.
type Aggregation struct {
	// All locked vet for a validations delegations.
	LockedVET    uint64 // VET locked this period
	LockedWeight uint64 // Weight including multipliers

	// Pending delegations, does NOT contribute to current TVL, it will increase the LockedVET in the next period and reset to 0
	PendingVET    uint64 // VET that is pending to be locked in the next period
	PendingWeight uint64 // Weight including multipliers

	// Exiting delegations, does NOT contribute to current TVL, it will decrease the LockedVET in the next period and reset to 0
	ExitingVET    uint64 // VET that is exiting the next period
	ExitingWeight uint64 // Weight including multipliers
}

// newAggregation creates a new zero-initialized aggregation for a validator.
func newAggregation() *Aggregation {
	return &Aggregation{
		LockedVET:     0,
		LockedWeight:  0,
		PendingVET:    0,
		PendingWeight: 0,
		ExitingVET:    0,
		ExitingWeight: 0,
	}
}

func (a *Aggregation) IsEmpty() bool {
	// aggregation subfields are expected to never be nil
	return a.LockedVET == 0 && a.ExitingVET == 0 && a.PendingVET == 0
}

// NextPeriodTVL is the total value locked (TVL) for the next period.
// It is the sum of the currently recurring VET, plus any pending recurring and one-time VET.
func (a *Aggregation) NextPeriodTVL() uint64 {
	return a.LockedVET + a.PendingVET
}

// renew transitions delegations to the next staking period.
// Pending delegations become locked, exiting delegations become withdrawable.
// 1. Move Pending => Locked
// 2. Remove ExitingVET from LockedVET
// 3. Move ExitingVET to WithdrawableVET
// return a delta object
func (a *Aggregation) renew() *delta.Renewal {
	lockedIncrease := stakes.NewWeightedStake(a.PendingVET, a.PendingWeight)
	lockedDecrease := stakes.NewWeightedStake(a.ExitingVET, a.ExitingWeight)
	queuedDecrease := stakes.NewWeightedStake(a.PendingVET, a.PendingWeight)

	// Move Pending => Locked
	a.LockedVET += a.PendingVET
	a.LockedWeight += a.PendingWeight

	a.PendingVET = 0
	a.PendingWeight = 0

	// Remove ExitingVET from LockedVET
	a.LockedVET -= a.ExitingVET
	a.LockedWeight -= a.ExitingWeight

	// TODO: No WithdrawableVET?
	// Move ExitingVET to WithdrawableVET
	a.ExitingVET = 0
	a.ExitingWeight = 0

	return &delta.Renewal{
		LockedIncrease: lockedIncrease,
		LockedDecrease: lockedDecrease,
		QueuedDecrease: queuedDecrease,
	}
}

// exit immediately moves all delegation funds to withdrawable state.
// Called when the validator exits, making all delegations withdrawable regardless of their individual state.
func (a *Aggregation) exit() *delta.Exit {
	// Return these values to modify contract totals
	exit := delta.Exit{
		ExitedTVL:      stakes.NewWeightedStake(a.LockedVET, a.LockedWeight),
		QueuedDecrease: stakes.NewWeightedStake(a.PendingVET, a.PendingWeight),
	}

	// Reset the aggregation
	a.ExitingVET = 0
	a.ExitingWeight = 0
	a.LockedVET = 0
	a.LockedWeight = 0
	a.PendingVET = 0
	a.PendingWeight = 0

	return &exit
}
