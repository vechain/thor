// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

type Status = uint8

const (
	StatusUnknown = Status(iota) // 0 -> default value
	StatusQueued                 // Once on the queue
	StatusActive                 // When activated by protocol
	StatusExit                   // Validation should not be used again
)

type Validation struct {
	Node               thor.Address // the node address of the validation
	Endorsor           thor.Address // the address providing the stake
	Period             uint32       // the staking period of the validation
	CompleteIterations uint32       // the completed staking periods by the validation
	Status             Status       // status of the validation
	Online             bool         // whether the validation is online or not
	StartBlock         uint32       // the block number when the validation started the first staking period
	ExitBlock          *uint32      `rlp:"nil"` // the block number when the validation moved to cooldown

	LockedVET          *big.Int // the amount of VET locked for the current staking period, for the validator only
	NextPeriodDecrease *big.Int // the amount of VET that will be unlocked in the next staking period. DOES NOT contribute to the TVL
	PendingLocked      *big.Int // the amount of VET that will be locked in the next staking period
	CooldownVET        *big.Int // the amount of VET that is locked into the validation's cooldown
	WithdrawableVET    *big.Int // the amount of VET that is currently withdrawable

	Weight *big.Int // LockedVET x2 + total weight from delegators

	Next *thor.Bytes32 `rlp:"nil"` // doubly linked list
	Prev *thor.Bytes32 `rlp:"nil"` // doubly linked list
}

type ValidationTotals struct {
	TotalLockedStake        *big.Int // total locked stake in validation (current period), validation's stake + all delegators stake
	TotalLockedWeight       *big.Int // total locked weight in validation (current period), validation's weight + all delegators weight
	DelegationsLockedStake  *big.Int // total locked stake in validation (current period) by all delegators
	DelegationsLockedWeight *big.Int // total locked weight in validation (current period) by all delegators
}

// IsEmpty returns whether the entry can be treated as empty.
func (v *Validation) IsEmpty() bool {
	return v.Status == StatusUnknown
}

// IsPeriodEnd returns whether the provided block is the last block of the current staking period.
func (v *Validation) IsPeriodEnd(current uint32) bool {
	diff := current - v.StartBlock
	return diff%v.Period == 0
}

// NextPeriodTVL returns the amount of VET that will be locked in the next staking period for the validator only.
func (v *Validation) NextPeriodTVL() *big.Int {
	validationTotal := big.NewInt(0).Add(v.LockedVET, v.PendingLocked)
	validationTotal = big.NewInt(0).Sub(validationTotal, v.NextPeriodDecrease)
	return validationTotal
}

func (v *Validation) CurrentIteration() uint32 {
	if v.Status == StatusActive {
		return v.CompleteIterations + 1 // +1 because the current iteration is not completed yet
	}
	return v.CompleteIterations
}

// Renew moves the stakes and weights around as follows:
// 1. Move PendingLocked => Locked
// 2. Decrease LockedVET by NextPeriodDecrease
// 3. Increase WithdrawableVET by NextPeriodDecrease
// 4. Set PendingLocked to 0
// 5. Set NextPeriodDecrease to 0
func (v *Validation) Renew() *Renewal {
	changeTVL := big.NewInt(0)

	changeTVL.Add(changeTVL, v.PendingLocked)
	changeTVL.Sub(changeTVL, v.NextPeriodDecrease)

	queuedDecrease := big.NewInt(0).Set(v.PendingLocked)
	v.WithdrawableVET = big.NewInt(0).Add(v.WithdrawableVET, v.NextPeriodDecrease)
	v.PendingLocked = big.NewInt(0)
	v.NextPeriodDecrease = big.NewInt(0)

	changeWeight := big.NewInt(0).Mul(changeTVL, validatorWeightMultiplier) // Apply x2 multiplier for validation's stake

	v.CompleteIterations++

	return &Renewal{
		ChangeTVL:            changeTVL,
		ChangeWeight:         changeWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: big.NewInt(0).Mul(queuedDecrease, validatorWeightMultiplier),
	}
}

type Delegation struct {
	ValidationID   thor.Bytes32 // the ID of the validation to which the delegator is delegating
	Stake          *big.Int
	Multiplier     uint8
	LastIteration  *uint32 `rlp:"nil"` // the last staking period in which the delegator was active
	FirstIteration uint32  // the iteration at which the delegator was created
}

// IsEmpty returns whether the entry can be treated as empty.
func (d *Delegation) IsEmpty() bool {
	return (d.Stake == nil || d.Stake.Sign() == 0) && d.Multiplier == 0
}

// Weight returns the weight of the delegator, which is calculated as:
// weight = stake * multiplier / 100
func (d *Delegation) Weight() *big.Int {
	if d.IsEmpty() {
		return big.NewInt(0)
	}

	weight := big.NewInt(0).Mul(d.Stake, big.NewInt(int64(d.Multiplier))) // multiplier is %
	weight = weight.Quo(weight, big.NewInt(100))                          // convert to %

	return weight
}

// Started returns whether the delegation became locked
func (d *Delegation) Started(validation *Validation) bool {
	if d.IsEmpty() {
		return false
	}
	if validation.Status == StatusQueued {
		return false // Delegation cannot start if the validation is not active
	}
	currentStakingPeriod := validation.CurrentIteration()
	return currentStakingPeriod >= d.FirstIteration
}

// Ended returns whether the delegation has ended
// It returns true if:
// - the delegation's exit iteration is less than the current staking period
// - OR if the validation is in exit status and the delegation has started
func (d *Delegation) Ended(validation *Validation) bool {
	if d.IsEmpty() {
		return false
	}
	if validation.Status == StatusQueued {
		return false // Delegation cannot end if the validation is not active
	}
	if validation.Status == StatusExit && d.Started(validation) {
		return true // Delegation is ended if the validation is in exit status
	}
	currentStakingPeriod := validation.CurrentIteration()
	if d.LastIteration == nil {
		return false
	}
	return *d.LastIteration < currentStakingPeriod
}

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
	return a.LockedVET == nil || a.ExitingVET == nil || a.PendingVET == nil || a.WithdrawableVET == nil
}

// NextPeriodTVL is the total value locked (TVL) for the next period.
// It is the sum of the currently recurring VET, plus any pending recurring and one-time VET.
// Does not include CurrentOneTimeVET since that stake is due to withdraw.
func (a *Aggregation) NextPeriodTVL() *big.Int {
	nextTVL := big.NewInt(0)
	nextTVL.Add(nextTVL, a.LockedVET)
	nextTVL.Add(nextTVL, a.PendingVET)
	return nextTVL
}

// Renew moves the stakes and weights around as follows:
// 1. Move CurrentOneTimeVET => WithdrawableVET
// 2. Move PendingRecurringVET => CurrentRecurringVET
// 3. Move PendingOneTimeVET => CurrentOneTimeVET
// 4. Return the change in TVL and weight
func (a *Aggregation) Renew() *Renewal {
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

	return &Renewal{
		ChangeTVL:            changeTVL,
		ChangeWeight:         changeWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedDecreaseWeight,
	}
}

// Exit moves all the funds to withdrawable
func (a *Aggregation) Exit() (*big.Int, *big.Int, *big.Int, *big.Int) {
	// Return these values to modify contract totals
	exitedTVL := big.NewInt(0).Set(a.LockedVET)
	exitedWeight := big.NewInt(0).Set(a.LockedWeight)
	queuedDecrease := big.NewInt(0).Set(a.PendingVET)
	queuedWeightDecrease := big.NewInt(0).Set(a.PendingWeight)

	// Move all the funds to withdrawable
	withdrawable := big.NewInt(0).Set(a.WithdrawableVET)
	withdrawable.Add(withdrawable, a.LockedVET)
	withdrawable.Add(withdrawable, a.PendingVET)

	// Reset the aggregation
	a.ExitingVET = big.NewInt(0)
	a.ExitingWeight = big.NewInt(0)
	a.LockedVET = big.NewInt(0)
	a.LockedWeight = big.NewInt(0)
	a.PendingVET = big.NewInt(0)
	a.PendingWeight = big.NewInt(0)

	// Make all funds withdrawable
	a.WithdrawableVET = withdrawable

	return exitedTVL, queuedDecrease, exitedWeight, queuedWeightDecrease
}

type Renewal struct {
	ChangeTVL            *big.Int
	ChangeWeight         *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}
