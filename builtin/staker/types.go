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
	Master             thor.Address // the node address of the validator
	Endorsor           thor.Address // the address providing the stake
	Period             uint32       // the staking period of the validator
	CompleteIterations uint32       // the completed staking periods by the validator
	Status             Status       // status of the validator
	Online             bool         // whether the validator is online or not
	AutoRenew          bool         // whether the validations staking period is auto-renewed
	StartBlock         uint32       // the block number when the validator started the first staking period
	ExitBlock          *uint32      `rlp:"nil"` // the block number when the validator moved to cooldown

	LockedVET       *big.Int // the amount of VET locked for the current staking period
	LockedOnePeriod *big.Int // the amount of VET that will be unlocked in the next staking period. Continues to contribute to the validators TVL for the current staking period
	PendingLocked   *big.Int // the amount of VET that will be locked in the next staking period
	CooldownVET     *big.Int // the amount of VET that is locked into the validator's cooldown
	WithdrawableVET *big.Int // the amount of VET that is currently withdrawable

	Weight *big.Int // LockedVET + CooldownVET + total weight from delegators

	Next *thor.Bytes32 `rlp:"nil"` // double linked list
	Prev *thor.Bytes32 `rlp:"nil"` // double linked list
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

// NextPeriodStakes returns the validator stake and all the delegator stakes for the next staking period.
func (v *Validation) NextPeriodStakes(delegation *Aggregation) *big.Int {
	validatorTotal := big.NewInt(0).Add(v.LockedVET, v.PendingLocked)
	return validatorTotal.Add(validatorTotal, delegation.NextPeriodLocked())
}

type Delegation struct {
	ValidatorID    thor.Bytes32 // the ID of the validator to which the delegator is delegating
	Stake          *big.Int
	AutoRenew      bool
	Multiplier     uint8
	ExitIteration  *uint32 `rlp:"nil"` // the Validation iteration at which the delegator may withdraw their funds
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
func (d *Delegation) IsLocked(validator *Validation) bool {
	if d.IsEmpty() {
		return false
	}
	// validator is not active, so the delegator is not locked
	if validator.Status != StatusActive {
		return false
	}
	// the delegation is not yet locked into the validator
	if d.FirstIteration == validator.CompleteIterations+1 {
		return false
	}
	if d.ExitIteration == nil {
		return true
	}
	return *d.ExitIteration > validator.CompleteIterations
}

// Aggregation represents the total amount of VET locked for a given validation's delegations.
type Aggregation struct {
	LockedVET           *big.Int // LockedVET (autoRenew == true) represents the amount of VET locked for the current staking period
	LockedWeight        *big.Int // LockedWeight the weight of LockedVET including multipliers
	PendingLockedVET    *big.Int // PendingLockedVET (autoRenew == true) represents the amount of VET that will be locked in the next staking period
	PendingLockedWeight *big.Int // PendingLockedWeight the weight of PendingLockedVET including multipliers

	CooldownVET           *big.Int // CooldownVET (autoRenew == false) represents the amount of VET locked for the current staking period
	CooldownWeight        *big.Int // CooldownWeight the weight of CooldownVET including multipliers
	PendingCooldownVET    *big.Int // PendingCooldownVET (autoRenew == false) represents the amount of VET that will be locked for 1 staking period only
	PendingCooldownWeight *big.Int // PendingCooldownWeight the weight of PendingCooldownVET including multipliers

	WithdrawVET *big.Int // WithdrawVET represents the amount of VET that is available for withdrawal
}

func newAggregation() *Aggregation {
	return &Aggregation{
		LockedVET:             big.NewInt(0),
		LockedWeight:          big.NewInt(0),
		PendingLockedVET:      big.NewInt(0),
		PendingLockedWeight:   big.NewInt(0),
		CooldownVET:           big.NewInt(0),
		CooldownWeight:        big.NewInt(0),
		PendingCooldownVET:    big.NewInt(0),
		PendingCooldownWeight: big.NewInt(0),
		WithdrawVET:           big.NewInt(0),
	}
}

func (a *Aggregation) IsEmpty() bool {
	return a.LockedVET == nil && a.CooldownVET == nil && a.PendingLockedVET == nil && a.PendingCooldownVET == nil && a.WithdrawVET == nil
}

// PeriodLocked returns the VET locked for a given validator's delegations for the current staking period.
func (a *Aggregation) PeriodLocked() *big.Int {
	return big.NewInt(0).Add(a.LockedVET, a.CooldownVET)
}

// NextPeriodLocked returns the PeriodLocked for the next staking period
func (a *Aggregation) NextPeriodLocked() *big.Int {
	total := big.NewInt(0).Add(a.LockedVET, a.PendingLockedVET)
	total = total.Add(total, a.PendingCooldownVET)
	return total
}

// RenewDelegations moves the stakes and weights around as follows:
// 1. Move Cooldown => Withdrawable
// 2. Move PendingLocked => Locked
// 3. Move PendingCooldown => Cooldown
// 4. Return the change in TVL and weight
func (a *Aggregation) RenewDelegations() (*big.Int, *big.Int, *big.Int) {
	changeTVL := big.NewInt(0)
	changeWeight := big.NewInt(0)
	queuedDecrease := big.NewInt(0)

	// Move Cooldown => Withdrawable
	a.WithdrawVET = big.NewInt(0).Add(a.WithdrawVET, a.CooldownVET)
	changeTVL.Sub(changeTVL, a.CooldownVET)
	changeWeight.Sub(changeWeight, a.CooldownWeight)
	a.CooldownVET = big.NewInt(0)
	a.CooldownWeight = big.NewInt(0)

	// Move PendingLocked => Locked
	a.LockedVET = big.NewInt(0).Add(a.LockedVET, a.PendingLockedVET)
	a.LockedWeight = big.NewInt(0).Add(a.LockedWeight, a.PendingLockedWeight)
	changeTVL.Add(changeTVL, a.PendingLockedVET)
	changeWeight.Add(changeWeight, a.PendingLockedWeight)
	queuedDecrease.Add(queuedDecrease, a.PendingLockedVET)
	a.PendingLockedVET = big.NewInt(0)
	a.PendingLockedWeight = big.NewInt(0)

	// Move PendingCooldown => Cooldown
	a.CooldownVET = big.NewInt(0).Set(a.PendingCooldownVET)
	a.CooldownWeight = big.NewInt(0).Set(a.PendingCooldownWeight)
	changeTVL.Add(changeTVL, a.PendingCooldownVET)
	changeWeight.Add(changeWeight, a.PendingCooldownWeight)
	queuedDecrease.Add(queuedDecrease, a.PendingCooldownVET)
	a.PendingCooldownVET = big.NewInt(0)
	a.PendingCooldownWeight = big.NewInt(0)

	return changeTVL, changeWeight, queuedDecrease
}

// Exit moves all the funds to withdrawable
func (a *Aggregation) Exit() (*big.Int, *big.Int, *big.Int, *big.Int) {
	withdrawable := big.NewInt(0).Set(a.WithdrawVET)
	withdrawable = withdrawable.Add(withdrawable, a.LockedVET)
	withdrawable = withdrawable.Add(withdrawable, a.CooldownVET)

	// The change in TVL is the amount of VET that went from locked to withdrawable
	// Subtract the previously withdrawable amount from the new total
	exitedTVL := big.NewInt(0).Sub(withdrawable, a.WithdrawVET)
	exitedWeight := big.NewInt(0).Add(a.LockedWeight, a.CooldownWeight)
	queuedDecrease := big.NewInt(0).Add(a.PendingLockedVET, a.PendingCooldownVET)
	queuedWeightDecrease := big.NewInt(0).Add(a.PendingLockedWeight, a.PendingCooldownWeight)

	// PendingLockedVET did not previously contribute to the TVL, so we need to add it after
	withdrawable = withdrawable.Add(withdrawable, a.PendingLockedVET)
	withdrawable = withdrawable.Add(withdrawable, a.PendingCooldownVET)

	a.LockedVET = big.NewInt(0)
	a.LockedWeight = big.NewInt(0)
	a.CooldownVET = big.NewInt(0)
	a.CooldownWeight = big.NewInt(0)
	a.PendingLockedVET = big.NewInt(0)
	a.PendingLockedWeight = big.NewInt(0)
	a.PendingCooldownVET = big.NewInt(0)
	a.PendingCooldownWeight = big.NewInt(0)
	a.WithdrawVET = withdrawable

	return exitedTVL, queuedDecrease, exitedWeight, queuedWeightDecrease
}
