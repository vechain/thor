// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/thor"
)

var validatorWeightMultiplier = big.NewInt(2)

type validations struct {
	storage             *storage
	leaderGroup         *linkedList
	validatorQueue      *linkedList
	lowStakingPeriod    uint32
	mediumStakingPeriod uint32
	highStakingPeriod   uint32
}

func newValidations(storage *storage) *validations {
	// debug overrides for testing
	storage.debugOverride(&LowStakingPeriod, slotLowStakingPeriod)
	storage.debugOverride(&MediumStakingPeriod, slotMediumStakingPeriod)
	storage.debugOverride(&HighStakingPeriod, slotHighStakingPeriod)

	return &validations{
		storage:             storage,
		leaderGroup:         newLinkedList(storage, slotActiveHead, slotActiveTail, slotActiveGroupSize),
		validatorQueue:      newLinkedList(storage, slotQueuedHead, slotQueuedTail, slotQueuedGroupSize),
		lowStakingPeriod:    LowStakingPeriod,
		mediumStakingPeriod: MediumStakingPeriod,
		highStakingPeriod:   HighStakingPeriod,
	}
}

// IsActive returns true if there are active validations.
func (v *validations) IsActive() (bool, error) {
	activeCount, err := v.leaderGroup.count.Get()
	if err != nil {
		return false, err
	}
	return activeCount.Cmp(big.NewInt(0)) > 0, nil
}

// FirstActive returns validator address of first entry.
func (v *validations) FirstActive() (thor.Bytes32, error) {
	return v.leaderGroup.head.Get()
}

// FirstQueued returns validator address of first entry.
func (v *validations) FirstQueued() (thor.Bytes32, error) {
	return v.validatorQueue.head.Get()
}

func (v *validations) LeaderGroupIterator(callback func(thor.Bytes32, *Validation) error) error {
	return v.leaderGroup.Iter(callback)
}

// LeaderGroup lists all registered candidates.
func (v *validations) LeaderGroup() (map[thor.Bytes32]*Validation, error) {
	var group = make(map[thor.Bytes32]*Validation)
	err := v.LeaderGroupIterator(func(id thor.Bytes32, entry *Validation) error {
		group[id] = entry
		return nil
	})
	return group, err
}

func (v *validations) Add(
	endorsor thor.Address,
	node thor.Address,
	period uint32,
	stake *big.Int,
	autoRenew bool,
	currentBlock uint32,
) (thor.Bytes32, error) {
	if stake.Cmp(MinStake) < 0 || stake.Cmp(MaxStake) > 0 {
		return thor.Bytes32{}, errors.New("stake is out of range")
	}
	lookup, err := v.storage.GetLookup(node)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if !lookup.IsZero() {
		return thor.Bytes32{}, errors.New("validator already exists")
	}
	if period != v.lowStakingPeriod && period != v.mediumStakingPeriod && period != v.highStakingPeriod {
		return thor.Bytes32{}, errors.New("period is out of boundaries")
	}

	var b [4]byte
	binary.BigEndian.PutUint32(b[:], currentBlock)
	id := thor.Blake2b(node.Bytes(), b[:])

	entry := &Validation{
		Node:               node,
		Endorsor:           endorsor,
		Period:             period,
		CompleteIterations: 0,
		Status:             StatusQueued,
		Online:             true,
		AutoRenew:          autoRenew,
		LockedVET:          big.NewInt(0),
		PendingLocked:      stake,
		CooldownVET:        big.NewInt(0),
		NextPeriodDecrease: big.NewInt(0),
		WithdrawableVET:    big.NewInt(0),
		Weight:             big.NewInt(0),
	}

	if err := v.storage.queuedVET.Add(stake); err != nil {
		return thor.Bytes32{}, err
	}
	if err := v.storage.queuedWeight.Add(big.NewInt(0).Mul(stake, validatorWeightMultiplier)); err != nil {
		return thor.Bytes32{}, err
	}
	if err := v.storage.SetLookup(node, id); err != nil {
		return thor.Bytes32{}, err
	}

	added, err := v.validatorQueue.Add(id, entry)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if !added {
		return thor.Bytes32{}, errors.New("failed to add validator to queue")
	}

	if err := v.storage.SetAggregation(id, newAggregation(), true); err != nil {
		return thor.Bytes32{}, err
	}

	return id, nil
}

func (v *validations) ActivateNext(
	currentBlock uint32,
	params *params.Params,
) (*thor.Bytes32, error) {
	leaderGroupLength, err := v.leaderGroup.Len()
	if err != nil {
		return nil, err
	}
	maxLeaderGroupSize, err := params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, err
	}
	if leaderGroupLength.Cmp(maxLeaderGroupSize) >= 0 {
		return nil, errors.New("leader group is full")
	}
	// Check if queue is empty
	queuedSize, err := v.validatorQueue.Len()
	if err != nil {
		return nil, err
	}
	if queuedSize.Cmp(big.NewInt(0)) <= 0 {
		return nil, errors.New("no validator in the queue")
	}
	// pop the head of the queue
	id, validator, err := v.validatorQueue.Pop()
	if err != nil {
		return nil, err
	}
	if validator.IsEmpty() {
		return nil, errors.New("no validator in the queue")
	}
	aggregation, err := v.storage.GetAggregation(id)
	if err != nil {
		return nil, err
	}

	logger.Debug("activating validator", "id", id, "node", validator.Node, "block", currentBlock)

	// update the validator
	validatorLocked := big.NewInt(0).Add(validator.LockedVET, validator.PendingLocked)
	validator.PendingLocked = big.NewInt(0)
	validator.LockedVET = validatorLocked
	// x2 multiplier for validator's stake
	validatorWeight := big.NewInt(0).Mul(validatorLocked, validatorWeightMultiplier)

	renewal := aggregation.Renew()
	if err := v.storage.SetAggregation(id, aggregation, false); err != nil {
		return nil, err
	}

	validator.Weight = big.NewInt(0).Add(validatorWeight, renewal.ChangeWeight)
	totalLocked := big.NewInt(0).Add(validatorLocked, renewal.ChangeTVL)
	queuedDecrease := big.NewInt(0).Add(validatorLocked, renewal.QueuedDecrease)

	if err := v.storage.lockedVET.Add(totalLocked); err != nil {
		return nil, err
	}
	if err := v.storage.lockedWeight.Add(validator.Weight); err != nil {
		return nil, err
	}

	if err := v.storage.queuedVET.Sub(queuedDecrease); err != nil {
		return nil, err
	}
	if err := v.storage.queuedWeight.Sub(validator.Weight); err != nil {
		return nil, err
	}

	if !validator.AutoRenew {
		exitBlock, err := v.SetExitBlock(id, currentBlock+validator.Period)
		if err != nil {
			return nil, err
		}
		validator.ExitBlock = &exitBlock
	}

	validator.Status = StatusActive
	validator.Online = true
	validator.StartBlock = currentBlock
	// add to the active list
	added, err := v.leaderGroup.Add(id, validator)
	if err != nil {
		return nil, err
	}
	if !added {
		return nil, errors.New("failed to add validator to active list")
	}

	return &id, nil
}

func (v *validations) UpdateAutoRenew(endorsor thor.Address, id thor.Bytes32, autoRenew bool) error {
	validator, err := v.storage.GetValidation(id)
	if err != nil {
		return err
	}
	if validator.Endorsor != endorsor {
		return errors.New("invalid endorsor for node")
	}
	if validator.AutoRenew == autoRenew {
		return fmt.Errorf("auto-renewal is already set to %t", autoRenew)
	}

	validator.AutoRenew = autoRenew

	if validator.Status == StatusActive {
		if autoRenew {
			if err := v.storage.SetExitEpoch(*validator.ExitBlock, thor.Bytes32{}); err != nil {
				return err
			}
			validator.ExitBlock = nil
		} else {
			minBlock := validator.StartBlock + validator.Period*(validator.CurrentIteration())
			exitBlock, err := v.SetExitBlock(id, minBlock)
			if err != nil {
				return err
			}
			validator.ExitBlock = &exitBlock
		}
	}

	return v.storage.SetValidation(id, validator, false)
}

// SetExitBlock sets the exit block for a validator.
// It ensures that the exit block is not already set for another validator.
// A validator cannot consume several epochs at once.
func (v *validations) SetExitBlock(id thor.Bytes32, minBlock uint32) (uint32, error) {
	start := minBlock
	for {
		existing, err := v.storage.GetExitEpoch(start)
		if err != nil {
			return 0, err
		}
		if existing == id {
			return start, nil
		}
		if existing.IsZero() {
			if err := v.storage.SetExitEpoch(start, id); err != nil {
				return 0, err
			}
			return start, nil
		}
		start += epochLength
	}
}

func (v *validations) IncreaseStake(id thor.Bytes32, endorsor thor.Address, amount *big.Int) error {
	entry, err := v.storage.GetValidation(id)
	if err != nil {
		return err
	}
	if entry.IsEmpty() {
		return errors.New("validator doesn't exist")
	}
	if entry.Endorsor != endorsor {
		return errors.New("invalid endorser")
	}
	if entry.Status == StatusExit {
		return errors.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && !entry.AutoRenew {
		return errors.New("validator is not set to renew in the next period")
	}
	aggregation, err := v.storage.GetAggregation(id)
	if err != nil {
		return err
	}
	validatorTVL := entry.NextPeriodTVL()
	// we do not consider aggregation.CurrentRecurringVET since the delegator could enable auto-renew
	delegationTVL := big.NewInt(0).Add(aggregation.CurrentRecurringVET, aggregation.PendingRecurringVET)
	nextPeriodTVL := big.NewInt(0).Add(validatorTVL, delegationTVL)
	newTVL := big.NewInt(0).Add(nextPeriodTVL, amount)

	if newTVL.Cmp(MaxStake) > 0 {
		return errors.New("stake is out of range")
	}

	entry.PendingLocked = big.NewInt(0).Add(amount, entry.PendingLocked)
	if err := v.storage.queuedVET.Add(amount); err != nil {
		return err
	}
	if err := v.storage.queuedWeight.Add(big.NewInt(0).Mul(amount, validatorWeightMultiplier)); err != nil {
		return err
	}

	return v.storage.SetValidation(id, entry, false)
}

func (v *validations) DecreaseStake(id thor.Bytes32, endorsor thor.Address, amount *big.Int) error {
	entry, err := v.storage.GetValidation(id)
	if err != nil {
		return err
	}
	if entry.IsEmpty() {
		return errors.New("validator doesn't exist")
	}
	if entry.Endorsor != endorsor {
		return errors.New("invalid endorser")
	}
	if entry.Status == StatusExit {
		return errors.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && !entry.AutoRenew {
		return errors.New("validator is not set to renew in the next period, all funds will be withdrawable")
	}

	aggregation, err := v.storage.GetAggregation(id)
	if err != nil {
		return err
	}

	if entry.Status == StatusActive {
		// We don't consider any increases, i.e., entry.PendingLocked. We only consider locked and current decreases.
		// The reason is that validator can instantly withdraw PendingLocked at any time.
		// We need to make sure the locked VET minus the sum of the current decreases is still above the minimum stake.
		nextPeriodTVL := big.NewInt(0).Sub(entry.LockedVET, entry.NextPeriodDecrease)
		nextPeriodTVL = nextPeriodTVL.Sub(nextPeriodTVL, amount)
		if nextPeriodTVL.Cmp(MinStake) < 0 {
			return errors.New("next period stake is too low for validator")
		}
		entry.NextPeriodDecrease = big.NewInt(0).Add(entry.NextPeriodDecrease, amount)
	}

	if entry.Status == StatusQueued {
		// All the validator's stake exists within PendingLocked, so we need to make sure it maintains a minimum of MinStake.
		nextPeriodTVL := big.NewInt(0).Sub(entry.PendingLocked, amount)
		if nextPeriodTVL.Cmp(MinStake) < 0 {
			return errors.New("next period stake is too low for validator")
		}
		entry.PendingLocked = big.NewInt(0).Sub(entry.PendingLocked, amount)
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, amount)
		if err := v.storage.queuedVET.Sub(amount); err != nil {
			return err
		}
		if err := v.storage.queuedWeight.Sub(big.NewInt(0).Mul(amount, validatorWeightMultiplier)); err != nil {
			return err
		}
	}
	err = v.storage.SetAggregation(id, aggregation, false)
	if err != nil {
		return err
	}

	return v.storage.SetValidation(id, entry, false)
}

// WithdrawStake allows validations to withdraw any withdrawable stake.
// It also verifies the endorsor and updates the validator totals.
func (v *validations) WithdrawStake(endorsor thor.Address, id thor.Bytes32, currentBlock uint32) (*big.Int, error) {
	entry, withdrawable, err := v.GetWithdrawable(id, currentBlock)
	if err != nil {
		return nil, err
	}
	if entry.Endorsor != endorsor {
		return big.NewInt(0), errors.New("invalid endorser")
	}

	// entry has exited and waited for the cooldown period
	if entry.ExitBlock != nil && *entry.ExitBlock+cooldownPeriod <= currentBlock {
		entry.CooldownVET = big.NewInt(0)
	}

	if entry.Status == StatusQueued {
		if err := v.storage.SetLookup(entry.Node, thor.Bytes32{}); err != nil {
			return nil, err
		}
		entry.PendingLocked = big.NewInt(0)
		entry.Status = StatusExit
		if _, err := v.validatorQueue.Remove(id, entry); err != nil {
			return nil, err
		}
	}
	if entry.PendingLocked.Sign() > 0 {
		entry.PendingLocked = big.NewInt(0)
	}

	entry.WithdrawableVET = big.NewInt(0)
	if err := v.storage.SetValidation(id, entry, false); err != nil {
		return nil, err
	}

	return withdrawable, nil
}

// GetWithdrawable returns the validator entry and the withdrawable amount.
// It does not perform any updates or verify the endorsor.
func (v *validations) GetWithdrawable(id thor.Bytes32, currentBlock uint32) (*Validation, *big.Int, error) {
	entry, err := v.storage.GetValidation(id)
	if err != nil {
		return nil, nil, err
	}
	if entry.IsEmpty() {
		return nil, nil, errors.New("validator doesn't exist")
	}
	withdrawAmount := big.NewInt(0).Set(entry.WithdrawableVET)

	// validator has exited and waited for the cooldown period
	if entry.ExitBlock != nil && *entry.ExitBlock+cooldownPeriod <= currentBlock {
		withdrawAmount = withdrawAmount.Add(withdrawAmount, entry.CooldownVET)
	}

	if entry.PendingLocked.Sign() > 0 {
		withdrawAmount = withdrawAmount.Add(withdrawAmount, entry.PendingLocked)
	}

	return entry, withdrawAmount, nil
}

// ExitValidator removes the validator from the active list and puts it in cooldown.
func (v *validations) ExitValidator(id thor.Bytes32) error {
	entry, err := v.storage.GetValidation(id)
	if err != nil {
		return err
	}
	if entry.IsEmpty() {
		return nil
	}
	// move locked to cooldown
	entry.Status = StatusExit
	entry.CooldownVET = big.NewInt(0).Set(entry.LockedVET)
	entry.LockedVET = big.NewInt(0)
	entry.NextPeriodDecrease = big.NewInt(0)
	// unlock delegator's stakes and remove their weight
	entry.CompleteIterations++

	if entry.PendingLocked.Sign() == 1 {
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, entry.PendingLocked)
		entry.PendingLocked = big.NewInt(0)
	}

	aggregation, err := v.storage.GetAggregation(id)
	if err != nil {
		return err
	}
	exitedTVL, queuedDecrease, exitedWeight, queuedWeightDecrease := aggregation.Exit()
	if err := v.storage.SetAggregation(id, aggregation, false); err != nil {
		return err
	}
	exitedTVL.Add(exitedTVL, entry.CooldownVET)
	weight := big.NewInt(0).Sub(entry.Weight, exitedWeight)
	entry.Weight = big.NewInt(0)

	if err := v.storage.queuedWeight.Sub(queuedWeightDecrease); err != nil {
		return err
	}
	if err := v.storage.queuedVET.Sub(queuedDecrease); err != nil {
		return err
	}
	if err := v.storage.lockedVET.Sub(exitedTVL); err != nil {
		return err
	}
	if err := v.storage.SetLookup(entry.Node, thor.Bytes32{}); err != nil {
		return err
	}
	if _, err = v.leaderGroup.Remove(id, entry); err != nil {
		return err
	}
	return v.storage.lockedWeight.Sub(weight)
}
