// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/staker/delta"
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
	Endorsor           thor.Address // the address providing the stake
	Period             uint32       // the staking period of the validation
	CompleteIterations uint32       // the completed staking periods by the validation
	Status             Status       // status of the validation
	Online             bool         // whether the validation is online or not
	StartBlock         uint32       // the block number when the validation started the first staking period
	ExitBlock          *uint32      `rlp:"nil"` // the block number when the validation moved to cooldown

	LockedVET        *big.Int // the amount of VET locked for the current staking period, for the validator only
	PendingUnlockVET *big.Int // the amount of VET that will be unlocked in the next staking period. DOES NOT contribute to the TVL
	QueuedVET        *big.Int // the amount of VET queued to be locked in the next staking period
	CooldownVET      *big.Int // the amount of VET that is locked into the validation's cooldown
	WithdrawableVET  *big.Int // the amount of VET that is currently withdrawable

	Weight *big.Int // LockedVET x2 + total weight from delegators

	Next *thor.Address `rlp:"nil"` // doubly linked list
	Prev *thor.Address `rlp:"nil"` // doubly linked list
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
	validationTotal := big.NewInt(0).Add(v.LockedVET, v.QueuedVET)
	validationTotal = big.NewInt(0).Sub(validationTotal, v.PendingUnlockVET)
	return validationTotal
}

func (v *Validation) CurrentIteration() uint32 {
	if v.Status == StatusActive {
		return v.CompleteIterations + 1 // +1 because the current iteration is not completed yet
	}
	return v.CompleteIterations
}

// Renew moves the stakes and weights around as follows:
// 1. Move QueuedVET => Locked
// 2. Decrease LockedVET by PendingUnlockVET
// 3. Increase WithdrawableVET by PendingUnlockVET
// 4. Set QueuedVET to 0
// 5. Set PendingUnlockVET to 0
func (v *Validation) Renew() *delta.Renewal {
	newLockedVET := big.NewInt(0)

	newLockedVET.Add(newLockedVET, v.QueuedVET)
	newLockedVET.Sub(newLockedVET, v.PendingUnlockVET)

	queuedDecrease := big.NewInt(0).Set(v.QueuedVET)
	v.WithdrawableVET = big.NewInt(0).Add(v.WithdrawableVET, v.PendingUnlockVET)
	v.QueuedVET = big.NewInt(0)
	v.PendingUnlockVET = big.NewInt(0)

	// Apply x2 multiplier for validation's stake
	newLockedWeight := big.NewInt(0).Mul(newLockedVET, pkgValidatorWeightMultiplier)
	queuedDecreaseWeight := big.NewInt(0).Mul(queuedDecrease, pkgValidatorWeightMultiplier)

	v.CompleteIterations++

	return &delta.Renewal{
		NewLockedVET:         newLockedVET,
		NewLockedWeight:      newLockedWeight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedDecreaseWeight,
	}
}
