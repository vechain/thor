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
	Master             thor.Address  // the node address of the validation
	Endorsor           thor.Address  // the address providing the stake
	Period             uint32        // the staking period of the validation
	CompleteIterations uint32        // the completed staking periods by the validation
	Status             Status        // status of the validation
	Online             bool          // whether the validation is online or not
	AutoRenew          bool          // whether the validations staking period is auto-renewed
	StartBlock         uint32        // the block number when the validation started the first staking period
	ExitBlock          *uint32       `rlp:"nil"` // the block number when the validation moved to cooldown
	Beneficiary        *thor.Address `rlp:"nil"` // the address that receives the rewards for the validation

	LockedVET          *big.Int // the amount of VET locked for the current staking period
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

// NextPeriodStakes returns the validation stake and all the delegator stakes for the next staking period.
func (v *Validation) NextPeriodStakes(delegation *Aggregation) *big.Int {
	validationTotal := big.NewInt(0).Add(v.LockedVET, v.PendingLocked)
	return validationTotal.Add(validationTotal, delegation.NextPeriodLocked())
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
	AutoRenew      bool
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

// IsLocked returns whether the delegator is locked for the current staking period.
func (d *Delegation) IsLocked(validation *Validation) bool {
	if d.IsEmpty() {
		return false
	}
	// validation is not active, so the delegator is not locked
	if validation.Status != StatusActive {
		return false
	}
	current := validation.CurrentIteration()
	if d.LastIteration == nil {
		return current >= d.FirstIteration
	}
	if current < d.FirstIteration {
		return false
	}
	return current == d.FirstIteration || current < *d.LastIteration
}

// Aggregation represents the total amount of VET locked for a given validation's delegations.
type Aggregation struct {
	// Auto-renewing (recurring) delegations
	CurrentRecurringVET    *big.Int // VET locked this period (autoRenew == true)
	CurrentRecurringWeight *big.Int // Weight including multipliers

	PendingRecurringVET    *big.Int // VET to be locked next period (autoRenew == true)
	PendingRecurringWeight *big.Int // Weight including multipliers

	// One-time (non-recurring) delegations
	CurrentOneTimeVET    *big.Int // VET locked this period (autoRenew == false)
	CurrentOneTimeWeight *big.Int // Weight including multipliers

	PendingOneTimeVET    *big.Int // VET to be locked next period (autoRenew == false)
	PendingOneTimeWeight *big.Int // Weight including multipliers

	// Withdrawable funds
	WithdrawableVET *big.Int // VET available for withdrawal
}

func newAggregation() *Aggregation {
	return &Aggregation{
		CurrentRecurringVET:    big.NewInt(0),
		CurrentRecurringWeight: big.NewInt(0),
		PendingRecurringVET:    big.NewInt(0),
		PendingRecurringWeight: big.NewInt(0),
		CurrentOneTimeVET:      big.NewInt(0),
		CurrentOneTimeWeight:   big.NewInt(0),
		PendingOneTimeVET:      big.NewInt(0),
		PendingOneTimeWeight:   big.NewInt(0),
		WithdrawableVET:        big.NewInt(0),
	}
}

func (a *Aggregation) IsEmpty() bool {
	return a.CurrentRecurringVET == nil && a.CurrentOneTimeVET == nil && a.PendingRecurringVET == nil && a.PendingOneTimeVET == nil && a.WithdrawableVET == nil
}

// PeriodLocked returns the VET locked for a given validation's delegations for the current staking period.
func (a *Aggregation) PeriodLocked() *big.Int {
	return big.NewInt(0).Add(a.CurrentRecurringVET, a.CurrentOneTimeVET)
}

// NextPeriodLocked returns the PeriodLocked for the next staking period
func (a *Aggregation) NextPeriodLocked() *big.Int {
	total := big.NewInt(0).Add(a.CurrentRecurringVET, a.PendingRecurringVET)
	total = total.Add(total, a.PendingOneTimeVET)
	return total
}

// Renew moves the stakes and weights around as follows:
// 1. Move CurrentOneTimeVET => WithdrawableVET
// 2. Move PendingRecurringVET => CurrentRecurringVET
// 3. Move PendingOneTimeVET => CurrentOneTimeVET
// 4. Return the change in TVL and weight
func (a *Aggregation) Renew() *Renewal {
	changeTVL := big.NewInt(0)
	changeWeight := big.NewInt(0)
	queuedDecrease := big.NewInt(0)
	queuedDecreaseWeight := big.NewInt(0).Add(a.PendingRecurringWeight, a.PendingOneTimeWeight)

	// Move CurrentOneTimeVET => WithdrawableVET
	a.WithdrawableVET = big.NewInt(0).Add(a.WithdrawableVET, a.CurrentOneTimeVET)
	changeTVL.Sub(changeTVL, a.CurrentOneTimeVET)
	changeWeight.Sub(changeWeight, a.CurrentOneTimeWeight)
	a.CurrentOneTimeVET = big.NewInt(0)
	a.CurrentOneTimeWeight = big.NewInt(0)

	// Move PendingRecurringVET => CurrentRecurringVET
	a.CurrentRecurringVET = big.NewInt(0).Add(a.CurrentRecurringVET, a.PendingRecurringVET)
	a.CurrentRecurringWeight = big.NewInt(0).Add(a.CurrentRecurringWeight, a.PendingRecurringWeight)
	changeTVL.Add(changeTVL, a.PendingRecurringVET)
	changeWeight.Add(changeWeight, a.PendingRecurringWeight)
	queuedDecrease.Add(queuedDecrease, a.PendingRecurringVET)
	a.PendingRecurringVET = big.NewInt(0)
	a.PendingRecurringWeight = big.NewInt(0)

	// Move PendingOneTimeVET => CurrentOneTimeVET
	a.CurrentOneTimeVET = big.NewInt(0).Set(a.PendingOneTimeVET)
	a.CurrentOneTimeWeight = big.NewInt(0).Set(a.PendingOneTimeWeight)
	changeTVL.Add(changeTVL, a.PendingOneTimeVET)
	changeWeight.Add(changeWeight, a.PendingOneTimeWeight)
	queuedDecrease.Add(queuedDecrease, a.PendingOneTimeVET)
	a.PendingOneTimeVET = big.NewInt(0)
	a.PendingOneTimeWeight = big.NewInt(0)

	return &Renewal{
		ChangeTVL:            changeTVL,
		ChangeWeight:         changeWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedDecreaseWeight,
	}
}

// Exit moves all the funds to withdrawable
func (a *Aggregation) Exit() (*big.Int, *big.Int, *big.Int, *big.Int) {
	withdrawable := big.NewInt(0).Set(a.WithdrawableVET)
	withdrawable = withdrawable.Add(withdrawable, a.CurrentRecurringVET)
	withdrawable = withdrawable.Add(withdrawable, a.CurrentOneTimeVET)

	// The change in TVL is the amount of VET that went from locked to withdrawable
	// Subtract the previously withdrawable amount from the new total
	exitedTVL := big.NewInt(0).Sub(withdrawable, a.WithdrawableVET)
	exitedWeight := big.NewInt(0).Add(a.CurrentRecurringWeight, a.CurrentOneTimeWeight)
	queuedDecrease := big.NewInt(0).Add(a.PendingRecurringVET, a.PendingOneTimeVET)
	queuedWeightDecrease := big.NewInt(0).Add(a.PendingRecurringWeight, a.PendingOneTimeWeight)

	// PendingRecurringVET did not previously contribute to the TVL, so we need to add it after
	withdrawable = withdrawable.Add(withdrawable, a.PendingRecurringVET)
	withdrawable = withdrawable.Add(withdrawable, a.PendingOneTimeVET)

	a.CurrentRecurringVET = big.NewInt(0)
	a.CurrentRecurringWeight = big.NewInt(0)
	a.CurrentOneTimeVET = big.NewInt(0)
	a.CurrentOneTimeWeight = big.NewInt(0)
	a.PendingRecurringVET = big.NewInt(0)
	a.PendingRecurringWeight = big.NewInt(0)
	a.PendingOneTimeVET = big.NewInt(0)
	a.PendingOneTimeWeight = big.NewInt(0)
	a.WithdrawableVET = withdrawable

	return exitedTVL, queuedDecrease, exitedWeight, queuedWeightDecrease
}

type Renewal struct {
	ChangeTVL            *big.Int
	ChangeWeight         *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}
