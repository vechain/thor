// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/builtin/staker/delta"
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
	Multiplier = 200 // 200% for validators
)

func WeightedStake(amount *big.Int) *stakes.WeightedStake {
	if amount == nil {
		amount = big.NewInt(0)
	}
	return stakes.NewWeightedStake(amount, Multiplier)
}

const SlotsUsed = 8

type Validation struct {
	// ---- Slot 0 ----
	Endorsor           thor.Address // the address providing the stake
	Period             uint32       // the staking period of the validation
	CompleteIterations uint32       // the completed staking periods by the validation
	Status             Status       // status of the validation
	Online             bool         // whether the validation is online or not

	// ---- Slot 1 ----
	StartBlock uint32 // the block number when the validation started the first staking period
	ExitBlock  uint32 // the block number when the validation moved to cooldown

	// ---- Slot 2 ----
	LockedVET *big.Int // the amount of VET locked for the current staking period, for the validator only
	// ---- Slot 3 ----
	PendingUnlockVET *big.Int // the amount of VET that will be unlocked in the next staking period. DOES NOT contribute to the TVL
	// ---- Slot 4 ----
	QueuedVET *big.Int // the amount of VET queued to be locked in the next staking period
	// ---- Slot 5 ----
	CooldownVET *big.Int // the amount of VET that is locked into the validation's cooldown
	// ---- Slot 6 ----
	WithdrawableVET *big.Int // the amount of VET that is currently withdrawable

	// ---- Slot 7 ----
	Weight *big.Int // LockedVET x2 + total weight from delegators
}

// ... existing code ...

func (v *Validation) DecodeSlots(slots []thor.Bytes32) error {
	if len(slots) != SlotsUsed {
		return errors.New("invalid number of slots for validation")
	}

	// Slot 0 (right-aligned packing):
	// [0..1]=padding, [2]=Online(1), [3]=Status(1), [4..7]=CompleteIterations, [8..11]=Period, [12..31]=Endorsor
	v.Endorsor = thor.BytesToAddress(slots[0][12:32])
	v.Period = binary.BigEndian.Uint32(slots[0][8:12])
	v.CompleteIterations = binary.BigEndian.Uint32(slots[0][4:8])
	v.Status = slots[0][3]
	v.Online = slots[0][2] == 1

	// Slot 1 (right-aligned packing):
	// [24..27]=ExitBlock, [28..31]=StartBlock
	v.StartBlock = binary.BigEndian.Uint32(slots[1][28:32])
	v.ExitBlock = binary.BigEndian.Uint32(slots[1][24:28])

	// Slots 2..7 are full 32-byte words (big-endian), already correct.
	v.LockedVET = new(big.Int).SetBytes(slots[2][:])
	v.PendingUnlockVET = new(big.Int).SetBytes(slots[3][:])
	v.QueuedVET = new(big.Int).SetBytes(slots[4][:])
	v.CooldownVET = new(big.Int).SetBytes(slots[5][:])
	v.WithdrawableVET = new(big.Int).SetBytes(slots[6][:])
	v.Weight = new(big.Int).SetBytes(slots[7][:])

	return nil
}

func (v *Validation) EncodeSlots() []thor.Bytes32 {
	slots := make([]thor.Bytes32, SlotsUsed)

	// Slot 0 (right-aligned):
	// [12..31]=Endorsor, [8..11]=Period, [4..7]=CompleteIterations, [3]=Status, [2]=Online, [0..1]=padding
	copy(slots[0][12:32], v.Endorsor.Bytes())
	binary.BigEndian.PutUint32(slots[0][8:12], v.Period)
	binary.BigEndian.PutUint32(slots[0][4:8], v.CompleteIterations)
	slots[0][3] = v.Status
	if v.Online {
		slots[0][2] = 1
	} else {
		slots[0][2] = 0
	}

	// Slot 1 (right-aligned):
	// [28..31]=StartBlock, [24..27]=ExitBlock
	binary.BigEndian.PutUint32(slots[1][28:32], v.StartBlock)
	binary.BigEndian.PutUint32(slots[1][24:28], v.ExitBlock)

	// Slots 2..7 unchanged
	slots[2] = thor.BytesToBytes32(v.LockedVET.Bytes())
	slots[3] = thor.BytesToBytes32(v.PendingUnlockVET.Bytes())
	slots[4] = thor.BytesToBytes32(v.QueuedVET.Bytes())
	slots[5] = thor.BytesToBytes32(v.CooldownVET.Bytes())
	slots[6] = thor.BytesToBytes32(v.WithdrawableVET.Bytes())
	slots[7] = thor.BytesToBytes32(v.Weight.Bytes())

	return slots
}

func (v *Validation) UsedSlots() int {
	return SlotsUsed
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
	weight := WeightedStake(newLockedVET).Weight()
	queuedDecreaseWeight := WeightedStake(queuedDecrease).Weight()

	v.CompleteIterations++

	return &delta.Renewal{
		NewLockedVET:         newLockedVET,
		NewLockedWeight:      weight,
		QueuedDecrease:       queuedDecrease,
		QueuedDecreaseWeight: queuedDecreaseWeight,
	}
}

func (v *Validation) Exit() *delta.Exit {
	releaseLockedTVL := big.NewInt(0).Set(v.LockedVET)
	releaseQueuedTVL := big.NewInt(0).Set(v.QueuedVET)

	// move locked to cooldown
	v.Status = StatusExit
	v.CooldownVET = big.NewInt(0).Set(v.LockedVET)
	v.LockedVET = big.NewInt(0)
	v.PendingUnlockVET = big.NewInt(0)
	v.Weight = big.NewInt(0)

	// unlock pending stake
	if v.QueuedVET.Sign() == 1 {
		// pending never contributes to weight as it's not active
		v.WithdrawableVET = big.NewInt(0).Add(v.WithdrawableVET, v.QueuedVET)
		v.QueuedVET = big.NewInt(0)
	}

	v.CompleteIterations++

	// We only return the change in the validation's TVL and weight
	return &delta.Exit{
		ExitedTVL:            releaseLockedTVL,
		ExitedTVLWeight:      WeightedStake(releaseLockedTVL).Weight(),
		QueuedDecrease:       releaseQueuedTVL,
		QueuedDecreaseWeight: WeightedStake(releaseQueuedTVL).Weight(),
	}
}
