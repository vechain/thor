// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"errors"

	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
)

// Aggregation represents the total amount of VET locked for a given validation's delegations.
type Aggregation struct {
	body *body
}

// Locked returns a copy of the locked delegation stake.
// Modifications to the returned stake do not affect the aggregation.
func (a *Aggregation) Locked() *stakes.WeightedStake {
	return a.body.Locked.Clone()
}

// Pending returns a copy of the pending delegation stake.
// Modifications to the returned stake do not affect the aggregation.
func (a *Aggregation) Pending() *stakes.WeightedStake {
	return a.body.Pending.Clone()
}

// Exiting returns a copy of the exiting delegation stake.
// Modifications to the returned stake do not affect the aggregation.
func (a *Aggregation) Exiting() *stakes.WeightedStake {
	return a.body.Exiting.Clone()
}

type body struct {
	// All locked vet and weight for a validations delegations.
	Locked *stakes.WeightedStake
	// Pending delegated vet and weight, does NOT contribute to current TVL, it will increase the LockedVET in the next period and reset to 0
	Pending *stakes.WeightedStake
	// Exiting delegated vet ,does NOT contribute to current TVL, it will decrease the LockedVET in the next period and reset to 0
	Exiting *stakes.WeightedStake
}

// newAggregation creates a new zero-initialized aggregation for a validator.
func newAggregation() *Aggregation {
	return &Aggregation{
		body: &body{
			Locked:  &stakes.WeightedStake{},
			Pending: &stakes.WeightedStake{},
			Exiting: &stakes.WeightedStake{},
		},
	}
}

func (a *Aggregation) IsEmpty() bool {
	// aggregation subfields are expected to never be nil
	return a.body.Locked.VET == 0 && a.body.Exiting.VET == 0 && a.body.Pending.VET == 0
}

// NextPeriodTVL is the total value locked (TVL) for the next period.
// It is the sum of the currently recurring VET, plus any pending recurring and one-time VET.
func (a *Aggregation) NextPeriodTVL() (uint64, error) {
	nextPeriodLocked := a.body.Locked.VET + a.body.Pending.VET
	if a.body.Exiting.VET > nextPeriodLocked {
		return 0, errors.New("insufficient locked and pending VET to subtract")
	}

	return nextPeriodLocked - a.body.Exiting.VET, nil
}

// renew transitions delegations to the next staking period.
// Pending() delegations become locked, exiting delegations become withdrawable.
// 1. Move Pending() => Locked()
// 2. Remove ExitingVET from LockedVET
// 3. Move ExitingVET to WithdrawableVET
// return a delta object
func (a *Aggregation) renew() (*globalstats.Renewal, error) {
	lockedIncrease := a.Pending()
	lockedDecrease := a.Exiting()
	queuedDecrease := a.Pending().VET

	// Move Pending => Locked
	if err := a.body.Locked.Add(a.Pending()); err != nil {
		return nil, err
	}
	a.body.Pending = &stakes.WeightedStake{}

	// Remove ExitingVET from LockedVET
	if err := a.body.Locked.Sub(a.Exiting()); err != nil {
		return nil, err
	}
	a.body.Exiting = &stakes.WeightedStake{}

	return &globalstats.Renewal{
		LockedIncrease: lockedIncrease,
		LockedDecrease: lockedDecrease,
		QueuedDecrease: queuedDecrease,
	}, nil
}

// exit immediately moves all delegation funds to withdrawable state.
// Called when the validator exits, making all delegations withdrawable regardless of their individual state.
func (a *Aggregation) exit() *globalstats.Exit {
	// Return these values to modify contract totals
	exit := globalstats.Exit{
		ExitedTVL:      a.Locked().Clone(),
		QueuedDecrease: a.Pending().VET,
	}

	// Reset the aggregation
	a.body.Exiting = &stakes.WeightedStake{}
	a.body.Locked = &stakes.WeightedStake{}
	a.body.Pending = &stakes.WeightedStake{}

	return &exit
}
