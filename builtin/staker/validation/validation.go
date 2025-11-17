// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"errors"

	"github.com/ethereum/go-ethereum/common/math"

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
	Endorser         thor.Address  // the address providing the stake
	Beneficiary      *thor.Address `rlp:"nil"` // the address receiving the rewards, if not set then endorser is rewarded
	Period           uint32        // the staking period of the validation
	CompletedPeriods uint32        // the completed staking periods by the validation, this will be updated when signal exit is called
	Status           Status        // status of the validation
	StartBlock       uint32        // the block number when the validation started the first staking period
	ExitBlock        *uint32       `rlp:"nil"` // the block number when the validation moved to cooldown
	OfflineBlock     *uint32       `rlp:"nil"` // the block when validator went offline, it will be cleared once online

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
	var exiting, overflow bool
	// If the validation is due to exit, then all locked VET is considered exiting.
	if v.Status == StatusActive && v.ExitBlock != nil {
		exitingVET, overflow = math.SafeAdd(v.LockedVET, agg.Locked.VET)
		if overflow {
			return nil, errors.New("exiting VET overflow")
		}
		exiting = true
	} else {
		exitingVET, overflow = math.SafeAdd(v.PendingUnlockVET, agg.Exiting.VET)
		if overflow {
			return nil, errors.New("exiting VET overflow")
		}
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
		valNextPeriodWeight, overflow := math.SafeAdd(stakes.NewWeightedStakeWithMultiplier(valNextPeriodTVL, multiplier).Weight, agg.Locked.Weight)
		if overflow {
			return nil, errors.New("next period weight overflow")
		}
		valNextPeriodWeight, overflow = math.SafeAdd(valNextPeriodWeight, agg.Pending.Weight)
		if overflow {
			return nil, errors.New("next period weight overflow")
		}
		valNextPeriodWeight, underflow := math.SafeSub(valNextPeriodWeight, agg.Exiting.Weight)
		if underflow {
			return nil, errors.New("next period weight underflow")
		}
		nextPeriodWeight = valNextPeriodWeight
	}
	totalLockedStake, overflow := math.SafeAdd(v.LockedVET, agg.Locked.VET)
	if overflow {
		return nil, errors.New("total locked stake overflow")
	}
	totalQueuedStake, overflow := math.SafeAdd(v.QueuedVET, agg.Pending.VET)
	if overflow {
		return nil, errors.New("total queued stake overflow")
	}

	return &Totals{
		// Delegation totals can be calculated by subtracting validators stakes / weights from the global totals.
		TotalLockedStake:  totalLockedStake,
		TotalLockedWeight: v.Weight,
		TotalQueuedStake:  totalQueuedStake,
		TotalExitingStake: exitingVET,
		NextPeriodWeight:  nextPeriodWeight,
	}, nil
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
	nextPeriodLocked, overflow := math.SafeAdd(v.LockedVET, v.QueuedVET)
	if overflow {
		return 0, errors.New("next period locked overflow")
	}
	if v.PendingUnlockVET > nextPeriodLocked {
		return 0, errors.New("insufficient locked and queued VET to subtract")
	}
	return nextPeriodLocked - v.PendingUnlockVET, nil
}

func (v *Validation) CurrentIteration(currentBlock uint32) (uint32, error) {
	// Unknown, Queued return 0
	if v.Status == StatusUnknown || v.Status == StatusQueued {
		return 0, nil
	}

	// Exited, from active or queued
	if v.Status == StatusExit {
		return v.CompletedPeriods, nil
	}

	// Active(signaled exit)
	// Once signaled exit, complete iterations is set to the current
	// iteration of the time that exit is signaled
	if v.CompletedPeriods > 0 {
		return v.CompletedPeriods, nil
	}

	// Active
	if currentBlock < v.StartBlock {
		return 0, errors.New("current block cannot be less than start block")
	}
	if v.Period == 0 {
		return 0, errors.New("period cannot be zero")
	}
	elapsedBlocks := currentBlock - v.StartBlock
	completedPeriods := elapsedBlocks / v.Period
	return completedPeriods + 1, nil
}

func (v *Validation) CompletedIterations(currentBlock uint32) (uint32, error) {
	// Unknown, Queued return 0
	if v.Status == StatusUnknown || v.Status == StatusQueued {
		return 0, nil
	}

	if v.Status == StatusExit {
		return v.CompletedPeriods, nil
	}

	// Active
	current, err := v.CurrentIteration(currentBlock)
	if err != nil {
		return 0, err
	}

	return current - 1, nil
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
		valWeight  uint64
		multiplier uint8
	}
	prev.valWeight = stakes.NewWeightedStakeWithMultiplier(v.LockedVET, v.multiplier()).Weight

	lockedIncrease := stakes.NewWeightedStake(v.QueuedVET, 0)
	lockedDecrease := stakes.NewWeightedStake(v.PendingUnlockVET, 0)

	var overflow bool
	v.LockedVET, overflow = math.SafeAdd(v.LockedVET, v.QueuedVET)
	if overflow {
		return nil, errors.New("locked VET overflow")
	}

	var underflow bool
	v.LockedVET, underflow = math.SafeSub(v.LockedVET, v.PendingUnlockVET)
	if underflow {
		return nil, errors.New("pending unlock VET exceeds total locked VET")
	}

	// in renew, the multiplier is based on the actual delegation weight
	after.multiplier = Multiplier
	if delegationWeight > 0 {
		after.multiplier = MultiplierWithDelegations
	}
	after.valWeight = stakes.NewWeightedStakeWithMultiplier(v.LockedVET, after.multiplier).Weight
	// calculate the locked stake change based on the validator's weight
	if prev.valWeight < after.valWeight {
		lockedIncrease.Weight, underflow = math.SafeSub(after.valWeight, prev.valWeight)
		if underflow {
			return nil, errors.New("locked increase weight underflow")
		}
	} else {
		lockedDecrease.Weight, underflow = math.SafeSub(prev.valWeight, after.valWeight)
		if underflow {
			return nil, errors.New("locked decrease weight underflow")
		}
	}

	v.WithdrawableVET, overflow = math.SafeAdd(v.WithdrawableVET, v.PendingUnlockVET)
	if overflow {
		return nil, errors.New("withdrawable VET overflow")
	}
	v.Weight, overflow = math.SafeAdd(after.valWeight, delegationWeight)
	if overflow {
		return nil, errors.New("weight overflow")
	}
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

// CooldownEnded returns true if validator has exited and the cooldown period has ended.
func (v *Validation) CooldownEnded(currentBlock uint32) bool {
	return v.ExitBlock != nil && *v.ExitBlock+thor.CooldownPeriod() <= currentBlock
}

// CalculateWithdrawableVET returns the validator withdrawable amount for a given block + period
func (v *Validation) CalculateWithdrawableVET(currentBlock uint32) (uint64, error) {
	withdrawAmount := v.WithdrawableVET

	var overflow bool
	if v.CooldownEnded(currentBlock) {
		withdrawAmount, overflow = math.SafeAdd(withdrawAmount, v.CooldownVET)
		if overflow {
			return 0, errors.New("withdrawable VET overflow")
		}
	}

	withdrawAmount, overflow = math.SafeAdd(withdrawAmount, v.QueuedVET)
	if overflow {
		return 0, errors.New("withdrawable VET overflow")
	}

	return withdrawAmount, nil
}

// multiplier returns the acting multiplier for the validation of the current staking period
func (v *Validation) multiplier() uint8 {
	// no delegation and multiplier is 1
	if v.Weight == v.LockedVET {
		return Multiplier
	}
	return MultiplierWithDelegations
}
