// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"errors"

	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

type Status = uint8

const (
	StatusUnknown = Status(iota) // 0 -> default value
	StatusQueued                 // Once on the queue
	StatusActive                 // When activated by protocol
	StatusExit                   // Validation should not be used again
)

const (
	Multiplier                = uint8(100) // 100% for validators if no delegations
	MultiplierWithDelegations = uint8(200) // 200% for validators with delegations
)

var ErrMaxTryReached = errors.New("max try reached")

type Validation struct {
	Endorser           thor.Address  // the address providing the stake
	Beneficiary        *thor.Address `rlp:"nil"` // the address receiving the rewards
	Period             uint32        // the staking period of the validation
	CompleteIterations uint32        // the completed staking periods by the validation
	Status             Status        // status of the validation
	StartBlock         uint32        // the block number when the validation started the first staking period
	ExitBlock          *uint32       `rlp:"nil"` // the block number when the validation moved to cooldown
	OfflineBlock       *uint32       `rlp:"nil"` // the block when validator went offline, it will be cleared once online

	LockedVET        uint64 // the amount(in VET not wei) locked for the current staking period, for the validator only
	PendingUnlockVET uint64 // the amount(in VET not wei) that will be unlocked in the next staking period. DOES NOT contribute to the TVL
	QueuedVET        uint64 // the amount(in VET not wei) queued to be locked in the next staking period
	CooldownVET      uint64 // the amount(in VET not wei) that is locked into the validation's cooldown
	WithdrawableVET  uint64 // the amount(in VET not wei) that is currently withdrawable

	Weight uint64 // The weight(in VET not wei), LockedVET x1 if no delegations, otherwise x2 + total weight from all delegators

	LinkedListEntry
}

type Totals struct {
	TotalLockedStake  uint64 // total locked stake in validation (current period), validation's stake + all delegators stake
	TotalLockedWeight uint64 // total locked weight in validation (current period), validation's weight + all delegators weight
	TotalQueuedStake  uint64 // total queued stake in validation (next period), validation's stake + all delegators stake
	TotalExitingStake uint64 // total exiting stake in validation (next period), validation's stake + all delegators stake
	NextPeriodWeight  uint64 // total weight which will be effective (next period), validations weight + all delegators weight
}

func (v *Validation) Totals(agg *aggregation.Aggregation) (*Totals, error) {
	var exitingVET uint64
	var exiting bool
	// If the validation is due to exit, then all locked VET is considered exiting.
	if v.Status == StatusActive && v.ExitBlock != nil {
		exitingVET = v.LockedVET + agg.LockedVET
		exiting = true
	} else {
		exitingVET = v.PendingUnlockVET + agg.ExitingVET
		exiting = false
	}

	// if the validation is exiting, then next period weight is zero
	nextPeriodWeight := uint64(0)
	if !exiting {
		multiplier := Multiplier
		nextPeriodTvl, err := agg.NextPeriodTVL()
		if err != nil {
			return nil, err
		}
		if nextPeriodTvl > 0 {
			multiplier = MultiplierWithDelegations
		}

		valNextPeriodTVL, err := v.NextPeriodTVL()
		if err != nil {
			return nil, err
		}
		nextPeriodWeight = stakes.NewWeightedStakeWithMultiplier(valNextPeriodTVL, multiplier).Weight +
			agg.LockedWeight + agg.PendingWeight - agg.ExitingWeight
	}

	return &Totals{
		// Delegation totals can be calculated by subtracting validators stakes / weights from the global totals.
		TotalLockedStake:  v.LockedVET + agg.LockedVET,
		TotalLockedWeight: v.Weight,
		TotalQueuedStake:  v.QueuedVET + agg.PendingVET,
		TotalExitingStake: exitingVET,
		NextPeriodWeight:  nextPeriodWeight,
	}, nil
}

// IsEmpty returns whether the entry can be treated as empty.
func (v *Validation) IsEmpty() bool {
	return v.Status == StatusUnknown
}

func (v *Validation) IsOnline() bool {
	return v.OfflineBlock == nil
}

// IsPeriodEnd returns whether the provided block is the last block of the current staking period.
func (v *Validation) IsPeriodEnd(current uint32) bool {
	diff := current - v.StartBlock
	return diff%v.Period == 0
}

// NextPeriodTVL returns the amount of VET that will be locked in the next staking period for the validator only.
func (v *Validation) NextPeriodTVL() (uint64, error) {
	nextPeriodLocked := v.LockedVET + v.QueuedVET
	if v.PendingUnlockVET > nextPeriodLocked {
		return 0, errors.New("insufficient locked and queued VET to subtract")
	}
	return nextPeriodLocked - v.PendingUnlockVET, nil
}

func (v *Validation) CurrentIteration() uint32 {
	if v.Status == StatusActive {
		return v.CompleteIterations + 1 // +1 because the current iteration is not completed yet
	}
	return v.CompleteIterations
}

// renew moves the stakes and weights around as follows:
// 1. Move QueuedVET => Locked
// 2. Decrease LockedVET by PendingUnlockVET
// 3. Increase WithdrawableVET by PendingUnlockVET
// 4. Set QueuedVET to 0
// 5. Set PendingUnlockVET to 0
func (v *Validation) renew(delegationWeight uint64) (*globalstats.Renewal, error) {
	queuedDecrease := v.QueuedVET

	var prev, after struct {
		lockedVET  uint64
		valWeight  uint64
		multiplier uint8
	}
	prev.lockedVET = v.LockedVET
	prev.valWeight = stakes.NewWeightedStakeWithMultiplier(v.LockedVET, v.multiplier()).Weight

	// in renew, the multiplier is based on the actual delegation weight
	after.multiplier = Multiplier
	if delegationWeight > 0 {
		after.multiplier = MultiplierWithDelegations
	}

	after.lockedVET = v.LockedVET + v.QueuedVET
	if v.PendingUnlockVET > after.lockedVET {
		return nil, errors.New("pending unlock VET exceeds total locked VET")
	}
	after.lockedVET -= v.PendingUnlockVET
	after.valWeight = stakes.NewWeightedStakeWithMultiplier(after.lockedVET, after.multiplier).Weight

	lockedIncrease := stakes.NewWeightedStake(0, 0)
	lockedDecrease := stakes.NewWeightedStake(0, 0)

	// calculate the locked stake change based on the validator's weight
	if prev.valWeight < after.valWeight {
		lockedIncrease.Weight = after.valWeight - prev.valWeight
	} else {
		lockedDecrease.Weight = prev.valWeight - after.valWeight
	}

	if prev.lockedVET < after.lockedVET {
		lockedIncrease.VET = after.lockedVET - prev.lockedVET
	} else {
		lockedDecrease.VET = prev.lockedVET - after.lockedVET
	}

	v.LockedVET = after.lockedVET
	v.WithdrawableVET += v.PendingUnlockVET
	v.Weight = after.valWeight + delegationWeight
	v.CompleteIterations++
	v.QueuedVET = 0
	v.PendingUnlockVET = 0

	return &globalstats.Renewal{
		LockedIncrease: lockedIncrease,
		LockedDecrease: lockedDecrease,
		QueuedDecrease: queuedDecrease,
	}, nil
}

func (v *Validation) exit() *globalstats.Exit {
	ExitedTVL := stakes.NewWeightedStakeWithMultiplier(v.LockedVET, v.multiplier()) // use the acting multiplier for locked stake
	QueuedDecrease := v.QueuedVET                                                   // queued weight is always initial weight

	v.Status = StatusExit
	// move locked to cooldown
	v.CooldownVET = v.LockedVET
	v.LockedVET = 0
	v.PendingUnlockVET = 0
	v.Weight = 0
	v.CompleteIterations++

	// unlock pending stake
	if v.QueuedVET > 0 {
		// pending never contributes to weight as it's not active
		v.WithdrawableVET += v.QueuedVET
		v.QueuedVET = 0
	}

	// We only return the change in the validation's TVL and weight
	return &globalstats.Exit{
		ExitedTVL:      ExitedTVL,
		QueuedDecrease: QueuedDecrease,
	}
}

// CalculateWithdrawableVET returns the validator withdrawable amount for a given block + period
func (v *Validation) CalculateWithdrawableVET(currentBlock uint32) uint64 {
	withdrawAmount := v.WithdrawableVET

	// validator has exited and waited for the cooldown period
	if v.ExitBlock != nil && *v.ExitBlock+thor.CooldownPeriod() <= currentBlock {
		withdrawAmount += v.CooldownVET
	}

	withdrawAmount += v.QueuedVET
	return withdrawAmount
}

// multiplier returns the acting multiplier for the validation of the current staking period
func (v *Validation) multiplier() uint8 {
	// no delegation and multiplier is 1
	if v.Weight == v.LockedVET {
		return Multiplier
	}
	return MultiplierWithDelegations
}
