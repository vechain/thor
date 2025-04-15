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
	StatusUnknown  = Status(iota) // 0 -> default value
	StatusQueued                  // Once on the queue
	StatusActive                  // When activated by protocol
	StatusCooldown                // When in cooldown
	StatusExit                    // Validation should not be used again
)

type Validation struct {
	Master             thor.Address // the node address of the validator
	Endorsor           thor.Address // the address providing the stake
	Expiry             *uint32      `rlp:"nil"` // the block number when the validator's current staking period ends
	Period             uint32       // the staking period of the validator
	CompleteIterations uint32       // the completed staking periods by the validator
	Status             Status       // status of the validator
	Online             bool         // whether the validator is online or not
	AutoRenew          bool         // whether the validations staking period is auto-renewed
	ExitTxBlock        uint32       // the block number when the validator signaled the exit
	StartBlock         uint32       // the block number when the validator started the first staking period

	LockedVET       *big.Int // the amount of VET locked for the current staking period
	PendingLocked   *big.Int // the amount of VET that will be locked in the next staking period
	CooldownVET     *big.Int // the amount of VET that will be withdrawable in the next staking period
	WithdrawableVET *big.Int // the amount of VET that is currently withdrawable

	Weight *big.Int // LockedVET + CooldownVET + total weight from delegators

	Next *thor.Bytes32 `rlp:"nil"` // double linked list
	Prev *thor.Bytes32 `rlp:"nil"` // double linked list
}

// IsEmpty returns whether the entry can be treated as empty.
func (v *Validation) IsEmpty() bool {
	emptyStake := v.LockedVET == nil || v.LockedVET.Sign() == 0
	emptyWeight := v.Weight == nil || v.Weight.Sign() == 0

	return emptyStake && emptyWeight && v.Status == StatusUnknown
}

// IsPeriodEnd returns whether the provided block is the last block of the current staking period.
func (v *Validation) IsPeriodEnd(current uint32) bool {
	diff := current - v.StartBlock
	return diff%v.Period == 0
}

// NextPeriodStakes returns the validator stake + all the delegators stakes for the next staking period.
func (v *Validation) NextPeriodStakes(delegation *ValidatorDelegations) *big.Int {
	validatorTotal := big.NewInt(0).Add(v.LockedVET, v.PendingLocked)
	return validatorTotal.Add(validatorTotal, delegation.NextPeriodLocked())
}

type Delegator struct {
	ValidatorID    thor.Bytes32 // the ID of the validator to which the delegator is delegating
	Stake          *big.Int
	AutoRenew      bool
	Multiplier     uint8
	ExitIteration  *uint32 `rlp:"nil"` // the Validation iteration at which the delegator may withdraw their funds
	FirstIteration uint32  // the iteration at which the delegator was created
}

// IsEmpty returns whether the entry can be treated as empty.
func (d *Delegator) IsEmpty() bool {
	return (d.Stake == nil || d.Stake.Sign() == 0) && d.Multiplier == 0
}

// Weight returns the weight of the delegator, which is calculated as:
// weight = stake * multiplier / 100
func (d *Delegator) Weight() *big.Int {
	if d.IsEmpty() {
		return big.NewInt(0)
	}

	weight := big.NewInt(0).Mul(d.Stake, big.NewInt(int64(d.Multiplier))) // multiplier is %
	weight = weight.Quo(weight, big.NewInt(100))                          // convert to %

	return weight
}

// IsLocked returns whether the delegator is locked for the current staking period.
func (d *Delegator) IsLocked(validator *Validation) bool {
	if d.IsEmpty() {
		return false
	}
	// validator is not active, so the delegator is not locked
	if validator.Status != StatusActive {
		return false
	}
	// the delegator is locked into the current staking period
	if d.FirstIteration == validator.CompleteIterations+1 {
		return true
	}
	if d.ExitIteration == nil {
		return true
	}
	return *d.ExitIteration > validator.CompleteIterations
}

type ValidatorDelegations struct {
	LockedVET           *big.Int // LockedVET (autoRenew == true) represents the amount of VET locked for the current staking period
	LockedWeight        *big.Int // LockedWeight the weight of LockedVET including multipliers
	PendingLockedVET    *big.Int // PendingLockedVET (autoRenew == true) represents the amount of VET that will be locked in the next staking period
	PendingLockedWeight *big.Int // PendingLockedWeight the weight of PendingLockedVET including multipliers

	CooldownVET           *big.Int // CooldownVET (autoRenew == false) represents the amount of VET locked for the current staking period
	CooldownWeight        *big.Int // CooldownWeight the weight of CooldownVET including multipliers
	PendingCooldownVET    *big.Int // PendingCooldownVET (autoRenew == false) represents the amount of VET that will be locked for 1 staking period only
	PendingCooldownWeight *big.Int // PendingCooldownWeight the weight of PendingCooldownVET including multipliers

	WithdrawVET *big.Int // WithdrawVET represents the amount of VET that is available for withdraw
}

func newDelegation() *ValidatorDelegations {
	return &ValidatorDelegations{
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

func (v *ValidatorDelegations) IsEmpty() bool {
	return v.LockedVET == nil && v.CooldownVET == nil && v.PendingLockedVET == nil && v.PendingCooldownVET == nil && v.WithdrawVET == nil
}

// PeriodLocked returns the VET that is locked for a given validator's delegations for the current staking period.
func (v *ValidatorDelegations) PeriodLocked() *big.Int {
	return big.NewInt(0).Add(v.LockedVET, v.CooldownVET)
}

// NextPeriodLocked returns the PeriodLocked for the next staking period
func (v *ValidatorDelegations) NextPeriodLocked() *big.Int {
	total := big.NewInt(0).Add(v.LockedVET, v.PendingLockedVET)
	total = total.Add(total, v.PendingCooldownVET)
	return total
}

// RenewDelegations moves the stakes and weights around as follows:
// 1. Move Cooldown => Withdrawable
// 2. Move PendingLocked => Locked
// 3. Move PendingCooldown => Cooldown
// 4. Return the change in TVL and weight
func (v *ValidatorDelegations) RenewDelegations() (*big.Int, *big.Int) {
	changeTVL := big.NewInt(0)
	changeWeight := big.NewInt(0)

	// Move Cooldown => Withdrawable
	v.WithdrawVET = big.NewInt(0).Add(v.WithdrawVET, v.CooldownVET)
	changeTVL.Sub(changeTVL, v.CooldownVET)
	changeWeight.Sub(changeWeight, v.CooldownWeight)
	v.CooldownVET = big.NewInt(0)
	v.CooldownWeight = big.NewInt(0)

	// Move PendingLocked => Locked
	v.LockedVET = big.NewInt(0).Add(v.LockedVET, v.PendingLockedVET)
	v.LockedWeight = big.NewInt(0).Add(v.LockedWeight, v.PendingLockedWeight)
	changeTVL.Add(changeTVL, v.PendingLockedVET)
	changeWeight.Add(changeWeight, v.PendingLockedWeight)
	v.PendingLockedVET = big.NewInt(0)
	v.PendingLockedWeight = big.NewInt(0)

	// Move PendingCooldown => Cooldown
	v.CooldownVET = big.NewInt(0).Set(v.PendingCooldownVET)
	v.CooldownWeight = big.NewInt(0).Set(v.PendingCooldownWeight)
	changeTVL.Add(changeTVL, v.PendingCooldownVET)
	changeWeight.Add(changeWeight, v.PendingCooldownWeight)
	v.PendingCooldownVET = big.NewInt(0)
	v.PendingCooldownWeight = big.NewInt(0)

	return changeTVL, changeWeight
}

// Exit moves all of the funds to withdrawable
func (v *ValidatorDelegations) Exit() *big.Int {
	withdrawable := big.NewInt(0).Set(v.WithdrawVET)
	withdrawable = withdrawable.Add(withdrawable, v.LockedVET)
	withdrawable = withdrawable.Add(withdrawable, v.CooldownVET)

	// The change in TVL is the amount of VET that went from locked to withdrawable
	// Subtract the previously withdrawable amount from the new total
	exitedTVL := big.NewInt(0).Sub(withdrawable, v.WithdrawVET)

	// PendingLockedVET did not previously contribute to the TVL, so we need to add it after
	withdrawable = withdrawable.Add(withdrawable, v.PendingLockedVET)
	withdrawable = withdrawable.Add(withdrawable, v.PendingCooldownVET)

	v.LockedVET = big.NewInt(0)
	v.LockedWeight = big.NewInt(0)
	v.CooldownVET = big.NewInt(0)
	v.CooldownWeight = big.NewInt(0)
	v.PendingLockedVET = big.NewInt(0)
	v.PendingLockedWeight = big.NewInt(0)
	v.PendingCooldownVET = big.NewInt(0)
	v.PendingCooldownWeight = big.NewInt(0)
	v.WithdrawVET = withdrawable

	return exitedTVL
}
