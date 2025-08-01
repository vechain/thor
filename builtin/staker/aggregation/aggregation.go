// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"math/big"

	"github.com/pkg/errors"
	
	"github.com/vechain/thor/v2/builtin/staker/delta"
)

// Aggregation represents the total amount of VET locked for a given validation's delegations.
type Aggregation struct {
	// All locked vet for a validations delegations.
	LockedVET    *big.Int // VET locked this period (autoRenew == true)
	LockedWeight *big.Int // Weight including multipliers

	// Pending delegations, does NOT contribute to current TVL, it will increase the LockedVET in the next period and reset to 0
	PendingVET    *big.Int // VET that is pending to be locked in the next period (autoRenew == false)
	PendingWeight *big.Int // Weight including multipliers

	// Exiting delegations, does NOT contribute to current TVL, it will decrease the LockedVET in the next period and reset to 0
	ExitingVET    *big.Int // VET that is exiting the next period
	ExitingWeight *big.Int // Weight including multipliers

	// Withdrawable funds
	WithdrawableVET *big.Int // VET available for withdrawal
}

// newAggregation creates a new zero-initialized aggregation for a validator.
func newAggregation() *Aggregation {
	return &Aggregation{
		LockedVET:       big.NewInt(0),
		LockedWeight:    big.NewInt(0),
		PendingVET:      big.NewInt(0),
		PendingWeight:   big.NewInt(0),
		ExitingVET:      big.NewInt(0),
		ExitingWeight:   big.NewInt(0),
		WithdrawableVET: big.NewInt(0),
	}
}

func (a *Aggregation) IsEmpty() bool {
	// aggregation subfields are expected to never be nil
	return a.LockedVET.Sign() == 0 && a.ExitingVET.Sign() == 0 && a.PendingVET.Sign() == 0 && a.WithdrawableVET.Sign() == 0
}

// NextPeriodTVL is the total value locked (TVL) for the next period.
// It is the sum of the currently recurring VET, plus any pending recurring and one-time VET.
func (a *Aggregation) NextPeriodTVL() *big.Int {
	return big.NewInt(0).Add(a.LockedVET, a.PendingVET)
}

// renew transitions delegations to the next staking period.
// Pending delegations become locked, exiting delegations become withdrawable.
// 1. Move Pending => Locked
// 2. Remove ExitingVET from LockedVET
// 3. Move ExitingVET to WithdrawableVET
// return a delta object
func (a *Aggregation) renew() *delta.Renewal {
	changeTVL := big.NewInt(0)
	changeWeight := big.NewInt(0)

	queuedDecrease := big.NewInt(0).Set(a.PendingVET)
	queuedDecreaseWeight := big.NewInt(0).Set(a.PendingWeight)

	// Move Pending => Locked
	changeTVL = changeTVL.Add(changeTVL, a.PendingVET)
	changeWeight = changeWeight.Add(changeWeight, a.PendingWeight)
	a.LockedVET = big.NewInt(0).Add(a.LockedVET, a.PendingVET)
	a.LockedWeight = big.NewInt(0).Add(a.LockedWeight, a.PendingWeight)
	a.PendingVET = big.NewInt(0)
	a.PendingWeight = big.NewInt(0)

	// Remove ExitingVET from LockedVET
	changeTVL = changeTVL.Sub(changeTVL, a.ExitingVET)
	changeWeight = changeWeight.Sub(changeWeight, a.ExitingWeight)
	a.LockedVET = big.NewInt(0).Sub(a.LockedVET, a.ExitingVET)
	a.LockedWeight = big.NewInt(0).Sub(a.LockedWeight, a.ExitingWeight)

	// Move ExitingVET to WithdrawableVET
	a.WithdrawableVET = big.NewInt(0).Add(a.WithdrawableVET, a.ExitingVET)
	a.ExitingVET = big.NewInt(0)
	a.ExitingWeight = big.NewInt(0)

	return &delta.Renewal{
		NewLockedVET:         changeTVL,
		NewLockedWeight:      changeWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedDecreaseWeight,
	}
}

// exit immediately moves all delegation funds to withdrawable state.
// Called when the validator exits, making all delegations withdrawable regardless of their individual state.
func (a *Aggregation) exit() (*delta.Exit, error) {
	// Return these values to modify contract totals
	exitedTVL := big.NewInt(0).Set(a.LockedVET)
	exitedWeight := big.NewInt(0).Set(a.LockedWeight)
	queuedDecrease := big.NewInt(0).Set(a.PendingVET)
	queuedWeightDecrease := big.NewInt(0).Set(a.PendingWeight)

	// Move all the funds to withdrawable
	withdrawable := big.NewInt(0).Set(a.WithdrawableVET)
	withdrawable.Add(withdrawable, a.LockedVET)
	withdrawable.Add(withdrawable, a.PendingVET)

	// ExitingVET should always be 0 at this point
	// as it was moved to withdrawable at the last renew()
	// TODO implement a test that breaks this:
	// The auto renew validator could, in their last staking period:
	// increase stake (pending)
	// signal exit
	// exit at the end
	if a.ExitingVET.Sign() == 1 {
		return nil, errors.New("ExitingVET should always be 0 at this point ")
	}

	// Reset the aggregation
	a.ExitingVET = big.NewInt(0)
	a.ExitingWeight = big.NewInt(0)
	a.LockedVET = big.NewInt(0)
	a.LockedWeight = big.NewInt(0)
	a.PendingVET = big.NewInt(0)
	a.PendingWeight = big.NewInt(0)

	// Make all funds withdrawable
	a.WithdrawableVET = withdrawable

	return &delta.Exit{
		ExitedTVL:            exitedTVL,
		ExitedTVLWeight:      exitedWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedWeightDecrease,
	}, nil
}
