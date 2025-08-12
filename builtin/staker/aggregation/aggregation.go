// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/thor"
)

const SlotsUsed = 6 // Number of slots used in the Aggregation struct

// Aggregation represents the total amount of VET locked for a given validation's delegations.
type Aggregation struct {
	// All locked vet for a validations delegations.
	// ---- Slot 0 ----
	LockedVET    *big.Int // VET locked this period (autoRenew == true)
	// ---- Slot 1 ----
	LockedWeight *big.Int // Weight including multipliers

	// Pending delegations, does NOT contribute to current TVL, it will increase the LockedVET in the next period and reset to 0
	// ---- Slot 2 ----
	PendingVET    *big.Int // VET that is pending to be locked in the next period (autoRenew == false)
	// ---- Slot 3 ----
	PendingWeight *big.Int // Weight including multipliers

	// Exiting delegations, does NOT contribute to current TVL, it will decrease the LockedVET in the next period and reset to 0
	// ---- Slot 4 ----
	ExitingVET    *big.Int // VET that is exiting the next period
	// ---- Slot 5 ----
	ExitingWeight *big.Int // Weight including multipliers
}

func (a *Aggregation) DecodeSlots(slots []thor.Bytes32) error {
	if len(slots) != SlotsUsed {
		return errors.New("invalid number of slots for aggregation")
	}
	a.LockedVET = new(big.Int).SetBytes(slots[0][:])
	a.LockedWeight = new(big.Int).SetBytes(slots[1][:])
	a.PendingVET = new(big.Int).SetBytes(slots[2][:])
	a.PendingWeight = new(big.Int).SetBytes(slots[3][:])
	a.ExitingVET = new(big.Int).SetBytes(slots[4][:])
	a.ExitingWeight = new(big.Int).SetBytes(slots[5][:])
	return nil
}

func (a *Aggregation) EncodeSlots() []thor.Bytes32 {
	slots := make([]thor.Bytes32, SlotsUsed)
	slots[0] = thor.BytesToBytes32(a.LockedVET.Bytes())
	slots[1] = thor.BytesToBytes32(a.LockedWeight.Bytes())
	slots[2] = thor.BytesToBytes32(a.PendingVET.Bytes())
	slots[3] = thor.BytesToBytes32(a.PendingWeight.Bytes())
	slots[4] = thor.BytesToBytes32(a.ExitingVET.Bytes())
	slots[5] = thor.BytesToBytes32(a.ExitingWeight.Bytes())

	return slots
}

func (a *Aggregation) UsedSlots() int {
	return SlotsUsed
}

// newAggregation creates a new zero-initialized aggregation for a validator.
func newAggregation() *Aggregation {
	return &Aggregation{
		LockedVET:     big.NewInt(0),
		LockedWeight:  big.NewInt(0),
		PendingVET:    big.NewInt(0),
		PendingWeight: big.NewInt(0),
		ExitingVET:    big.NewInt(0),
		ExitingWeight: big.NewInt(0),
	}
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
func (a *Aggregation) exit() *delta.Exit {
	// Return these values to modify contract totals
	exitedTVL := big.NewInt(0).Set(a.LockedVET)
	exitedWeight := big.NewInt(0).Set(a.LockedWeight)
	queuedDecrease := big.NewInt(0).Set(a.PendingVET)
	queuedWeightDecrease := big.NewInt(0).Set(a.PendingWeight)

	// Reset the aggregation
	a.ExitingVET = big.NewInt(0)
	a.ExitingWeight = big.NewInt(0)
	a.LockedVET = big.NewInt(0)
	a.LockedWeight = big.NewInt(0)
	a.PendingVET = big.NewInt(0)
	a.PendingWeight = big.NewInt(0)

	return &delta.Exit{
		ExitedTVL:            exitedTVL,
		ExitedTVLWeight:      exitedWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedWeightDecrease,
	}
}
