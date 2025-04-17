// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

type validations struct {
	storage             *storage
	leaderGroup         *linkedList
	validatorQueue      *orderedLinkedList
	leaderGroupSize     *solidity.Uint256
	queuedGroupSize     *solidity.Uint256
	lockedVET           *solidity.Uint256
	queuedVET           *solidity.Uint256
	lowStakingPeriod    uint32
	mediumStakingPeriod uint32
	highStakingPeriod   uint32
}

func newValidations(storage *storage) *validations {
	lowStakingPeriod := uint32(360) * 24 * 7
	if num, err := solidity.NewUint256(storage.Address(), storage.State(), slotLowStakingPeriod).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			lowStakingPeriod = uint32(numUint64)
		}
	}
	mediumStakingPeriod := uint32(360) * 24 * 14
	if num, err := solidity.NewUint256(storage.Address(), storage.State(), slotMediumStakingPeriod).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			mediumStakingPeriod = uint32(numUint64)
		}
	}
	highStakingPeriod := uint32(360) * 24 * 30
	if num, err := solidity.NewUint256(storage.Address(), storage.State(), slotHighStakingPeriod).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			highStakingPeriod = uint32(numUint64)
		}
	}

	return &validations{
		storage:             storage,
		leaderGroup:         newLinkedList(storage, slotActiveHead, slotActiveTail),
		validatorQueue:      newOrderedLinkedList(storage, slotQueuedHead, slotQueuedTail),
		queuedGroupSize:     solidity.NewUint256(storage.Address(), storage.State(), slotQueuedGroupSize),
		leaderGroupSize:     solidity.NewUint256(storage.Address(), storage.State(), slotActiveGroupSize),
		lockedVET:           solidity.NewUint256(storage.Address(), storage.State(), slotLockedVET),
		queuedVET:           solidity.NewUint256(storage.Address(), storage.State(), slotQueuedVET),
		lowStakingPeriod:    lowStakingPeriod,
		mediumStakingPeriod: mediumStakingPeriod,
		highStakingPeriod:   highStakingPeriod,
	}
}

// IsActive returns true if there are active validators.
func (v *validations) IsActive() (bool, error) {
	activeCount, err := v.leaderGroupSize.Get()
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
	return v.validatorQueue.linkedList.head.Get()
}

func (v *validations) LeaderGroupIterator(callback func(thor.Bytes32, *Validation) error) error {
	ptr, err := v.FirstActive()
	if err != nil {
		return err
	}
	for {
		entry, err := v.storage.GetValidator(ptr)
		if err != nil {
			return err
		}
		if entry.IsEmpty() {
			break
		}
		if err := callback(ptr, entry); err != nil {
			return err
		}
		if entry.Next == nil || entry.Next.IsZero() {
			break
		}
		ptr = *entry.Next
	}
	return nil
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
	master thor.Address,
	period uint32,
	stake *big.Int,
	autoRenew bool,
	currentBlock uint32,
) (thor.Bytes32, error) {
	if stake.Cmp(minStake) < 0 || stake.Cmp(maxStake) > 0 {
		return thor.Bytes32{}, errors.New("stake is out of range")
	}
	lookup, err := v.storage.GetLookup(master)
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
	id := thor.Blake2b(master.Bytes(), b[:])

	entry := &Validation{
		Master:             master,
		Endorsor:           endorsor,
		Expiry:             nil,
		Period:             period,
		CompleteIterations: 0,
		Status:             StatusQueued,
		Online:             true,
		AutoRenew:          autoRenew,
		LockedVET:          big.NewInt(0),
		PendingLocked:      stake,
		CooldownVET:        big.NewInt(0),
		WithdrawableVET:    big.NewInt(0),
		Weight:             big.NewInt(0),
		Next:               nil,
		Prev:               nil,
	}

	if err := v.validatorQueue.Add(id, entry); err != nil {
		return thor.Bytes32{}, err
	}

	if err := v.queuedVET.Add(stake); err != nil {
		return thor.Bytes32{}, err
	}

	// Increment queuedGroupSize when adding validator to queue
	if err := v.queuedGroupSize.Add(big.NewInt(1)); err != nil {
		return thor.Bytes32{}, err
	}

	if err := v.storage.SetLookup(master, id); err != nil {
		return thor.Bytes32{}, err
	}

	return id, nil
}

func (v *validations) ActivateNext(
	currentBlock uint32,
	params *params.Params,
) error {
	leaderGroupLength, err := v.leaderGroupSize.Get()
	if err != nil {
		return err
	}
	maxLeaderGroupSize, err := params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return err
	}
	if leaderGroupLength.Cmp(maxLeaderGroupSize) >= 0 {
		return errors.New("leader group is full")
	}
	// Check if queue is empty
	queuedSize, err := v.queuedGroupSize.Get()
	if err != nil {
		return err
	}
	if queuedSize.Cmp(big.NewInt(0)) <= 0 {
		return errors.New("no validator in the queue")
	}
	if err := v.leaderGroupSize.Add(big.NewInt(1)); err != nil {
		return nil
	}
	// pop the head of the queue
	id, validator, err := v.validatorQueue.Pop()
	if err != nil {
		return err
	}
	if validator.IsEmpty() {
		return errors.New("no validator in the queue")
	}
	delegation, err := v.storage.GetDelegation(id)
	if err != nil {
		return err
	}

	// Decrement queuedGroupSize when removing from queue
	if err := v.queuedGroupSize.Sub(big.NewInt(1)); err != nil {
		return err
	}

	logger.Debug("activating validator", "master", validator.Master, "block", currentBlock)

	// update the validator
	validatorLocked := big.NewInt(0).Add(validator.LockedVET, validator.PendingLocked)
	validator.PendingLocked = big.NewInt(0)
	validator.LockedVET = validatorLocked

	changeTVL, changeWeight, changePending := delegation.RenewDelegations()
	validator.Weight = big.NewInt(0).Add(validatorLocked, changeWeight)
	if err := v.storage.SetDelegation(id, delegation); err != nil {
		return err
	}

	totalLocked := big.NewInt(0).Add(validatorLocked, changeTVL)
	changePending = changePending.Add(changePending, validatorLocked)

	if err := v.lockedVET.Add(totalLocked); err != nil {
		return err
	}
	if err := v.queuedVET.Sub(changePending); err != nil {
		return err
	}

	expiry := currentBlock + validator.Period
	validator.Status = StatusActive
	validator.Online = true
	validator.Expiry = &expiry
	validator.ExitTxBlock = currentBlock
	validator.StartBlock = currentBlock
	// add to the active list
	if err := v.leaderGroup.Add(id, validator); err != nil {
		return err
	}

	return nil
}

func (v *validations) UpdateAutoRenew(endorsor thor.Address, id thor.Bytes32, autoRenew bool, blockNumber uint32) error {
	validator, err := v.storage.GetValidator(id)
	if err != nil {
		return err
	}
	if validator.Endorsor != endorsor {
		return errors.New("invalid endorsor for master")
	}
	validator.AutoRenew = autoRenew
	if !autoRenew {
		validator.ExitTxBlock = blockNumber
		validator.CooldownVET = big.NewInt(0).Add(validator.CooldownVET, validator.LockedVET)
		validator.LockedVET = big.NewInt(0)
	}
	return v.storage.SetValidator(id, validator)
}

func (v *validations) IncreaseStake(id thor.Bytes32, endorsor thor.Address, amount *big.Int) error {
	entry, err := v.storage.GetValidator(id)
	if err != nil {
		return err
	}
	if entry.IsEmpty() {
		return errors.New("validator doesn't exist")
	}
	if entry.Endorsor != endorsor {
		return errors.New("invalid endorser")
	}
	if entry.Status == StatusExit || entry.Status == StatusCooldown {
		return errors.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && !entry.AutoRenew {
		return errors.New("validator is not set to renew in the next period")
	}
	delegation, err := v.storage.GetDelegation(id)
	if err != nil {
		return err
	}
	nextPeriodTVL := entry.NextPeriodStakes(delegation)
	newTVL := big.NewInt(0).Add(nextPeriodTVL, amount)

	if newTVL.Cmp(maxStake) > 0 {
		return errors.New("stake is out of range")
	}

	entry.PendingLocked = amount.Add(amount, entry.PendingLocked)
	if err := v.queuedVET.Add(amount); err != nil {
		return err
	}

	if entry.Status == StatusActive {
		err = v.storage.SetValidator(id, entry)
		if err != nil {
			return err
		}
	}

	if entry.Status == StatusQueued {
		// queue is stake based, so we need to remove and re-add the validator
		if err := v.validatorQueue.Remove(id, entry); err != nil {
			return err
		}
		if err := v.validatorQueue.Add(id, entry); err != nil {
			return err
		}
	}

	return nil
}

func (v *validations) DecreaseStake(id thor.Bytes32, endorsor thor.Address, amount *big.Int) error {
	entry, err := v.storage.GetValidator(id)
	if err != nil {
		return err
	}
	if entry.IsEmpty() {
		return errors.New("validator doesn't exist")
	}
	if entry.Endorsor != endorsor {
		return errors.New("invalid endorser")
	}
	if entry.Status == StatusExit || entry.Status == StatusCooldown {
		return errors.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && !entry.AutoRenew {
		return errors.New("validator is not set to renew in the next period, all funds will be withdrawable")
	}
	newStake := big.NewInt(0).Add(entry.LockedVET, entry.PendingLocked)
	newStake = newStake.Sub(newStake, amount)
	if newStake.Cmp(minStake) < 0 {
		return errors.New("stake is too low for validator")
	}

	if entry.Status == StatusActive {
		entry.CooldownVET = entry.CooldownVET.Add(entry.CooldownVET, amount)
		entry.LockedVET = entry.LockedVET.Sub(entry.LockedVET, amount)

		err = v.storage.SetValidator(id, entry)
		if err != nil {
			return err
		}
	}

	if entry.Status == StatusQueued {
		entry.PendingLocked = big.NewInt(0).Sub(entry.PendingLocked, amount)
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, amount)
		if err := v.queuedVET.Sub(amount); err != nil {
			return err
		}

		// queue is stake based, so we need to remove and re-add the validator
		if err := v.validatorQueue.Remove(id, entry); err != nil {
			return err
		}
		if err := v.validatorQueue.Add(id, entry); err != nil {
			return err
		}
	}

	return nil
}

// WithdrawStake allows expired validations to withdraw their stake.
func (v *validations) WithdrawStake(endorsor thor.Address, id thor.Bytes32) (*big.Int, error) {
	entry, err := v.storage.GetValidator(id)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return big.NewInt(0), nil
	}
	if entry.Endorsor != endorsor {
		return big.NewInt(0), errors.New("invalid endorser")
	}
	withdrawAmount := big.NewInt(0).Set(entry.WithdrawableVET)

	if entry.Status == StatusExit || entry.Status == StatusQueued {
		if err := v.storage.SetLookup(entry.Master, thor.Bytes32{}); err != nil {
			return nil, err
		}
	}
	if entry.Status == StatusQueued {
		withdrawAmount = withdrawAmount.Add(withdrawAmount, entry.PendingLocked)
		entry.PendingLocked = big.NewInt(0)
		entry.Status = StatusExit
		if err := v.validatorQueue.Remove(id, entry); err != nil {
			return nil, err
		}
		if err := v.queuedGroupSize.Sub(big.NewInt(1)); err != nil {
			return nil, err
		}
	}
	if entry.PendingLocked.Sign() > 0 {
		withdrawAmount = withdrawAmount.Add(withdrawAmount, entry.PendingLocked)
		entry.PendingLocked = big.NewInt(0)
	}

	entry.WithdrawableVET = big.NewInt(0)
	if err := v.storage.SetValidator(id, entry); err != nil {
		return nil, err
	}
	return withdrawAmount, nil
}

// ExitValidator sets a validators status to exited and removes it from the active list.
// It will also decrease the total stake. Exited validators can then withdraw their stake.
func (v *validations) ExitValidator(id thor.Bytes32, currentBlock uint32) error {
	entry, err := v.storage.GetValidator(id)
	if err != nil {
		return err
	}
	if entry.IsEmpty() {
		return nil
	}
	if entry.Status != StatusExit && *entry.Expiry > currentBlock {
		return errors.New("validator cannot be removed")
	}
	validatorTVL := big.NewInt(0).Add(entry.LockedVET, entry.CooldownVET)

	logger.Debug("removing validator", "master", entry.Master, "block", currentBlock)

	entry.Status = StatusExit
	entry.Weight = big.NewInt(0)

	withdrawable := big.NewInt(0).Add(validatorTVL, entry.PendingLocked)
	withdrawable = withdrawable.Add(withdrawable, entry.WithdrawableVET)

	entry.WithdrawableVET = withdrawable
	entry.CooldownVET = big.NewInt(0)
	entry.LockedVET = big.NewInt(0)
	entry.PendingLocked = big.NewInt(0)

	if err = v.leaderGroup.Remove(id, entry); err != nil {
		return err
	}
	if err = v.storage.SetLookup(entry.Master, thor.Bytes32{}); err != nil {
		return err
	}
	if err = v.lockedVET.Sub(validatorTVL); err != nil {
		return err
	}
	return v.leaderGroupSize.Sub(big.NewInt(1))
}
