// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"encoding/binary"
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/builtin/staker/linkedlist"
	"github.com/vechain/thor/v2/builtin/staker/reverts"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

type Service struct {
	leaderGroup    *linkedlist.LinkedList
	validatorQueue *linkedlist.LinkedList

	minStake *big.Int
	maxStake *big.Int

	repo *Repository
}

var (
	// active validations linked list
	slotActiveTail      = thor.BytesToBytes32([]byte(("validations-active-tail")))
	slotActiveHead      = thor.BytesToBytes32([]byte(("validations-active-head")))
	slotActiveGroupSize = thor.BytesToBytes32([]byte(("validations-active-group-size")))

	// queued validations linked list
	slotQueuedHead      = thor.BytesToBytes32([]byte(("validations-queued-head")))
	slotQueuedTail      = thor.BytesToBytes32([]byte(("validations-queued-tail")))
	slotQueuedGroupSize = thor.BytesToBytes32([]byte(("validations-queued-group-size")))
)

func New(sctx *solidity.Context,
	minStake *big.Int,
	maxStake *big.Int,
) *Service {
	repo := NewRepository(sctx)

	return &Service{
		repo: repo,

		leaderGroup:    linkedlist.NewLinkedList(sctx, slotActiveHead, slotActiveTail, slotActiveGroupSize),
		validatorQueue: linkedlist.NewLinkedList(sctx, slotQueuedHead, slotQueuedTail, slotQueuedGroupSize),

		minStake: minStake,
		maxStake: maxStake,
	}
}

func (s *Service) GetCompletedPeriods(validator thor.Address) (uint32, error) {
	v, err := s.GetValidation(validator)
	if err != nil {
		return uint32(0), err
	}
	return v.CompleteIterations, nil
}

func (s *Service) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) error {
	val, err := s.GetValidation(node)
	if err != nil {
		return err
	}

	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, val.CurrentIteration())
	key := thor.Blake2b([]byte("rewards"), node.Bytes(), periodBytes)

	rewards, err := s.repo.GetReward(key)
	if err != nil {
		return err
	}

	return s.repo.SetReward(key, big.NewInt(0).Add(rewards, reward), false)
}

func (s *Service) LeaderGroupIterator(callbacks ...func(thor.Address, *Validation) error) error {
	return s.leaderGroup.Iter(func(address thor.Address) error {
		// Fetch the validation object for this address
		validation, err := s.repo.GetValidation(address)
		if err != nil {
			return err
		}

		for _, callback := range callbacks {
			if err := callback(address, validation); err != nil {
				return err
			}
		}
		return nil
	})
}

// IsActive returns true if there are active validations.
func (s *Service) IsActive() (bool, error) {
	activeCount, err := s.leaderGroup.Len()
	if err != nil {
		return false, err
	}
	return activeCount.Cmp(big.NewInt(0)) > 0, nil
}

// FirstActive returns validator address of first entry.
func (s *Service) FirstActive() (*thor.Address, error) {
	validator, err := s.leaderGroup.Head()
	return &validator, err
}

// FirstQueued returns validator address of first entry.
func (s *Service) FirstQueued() (*thor.Address, error) {
	validator, err := s.validatorQueue.Head()
	return &validator, err
}

func (s *Service) QueuedGroupSize() (*big.Int, error) {
	return s.validatorQueue.Len()
}

func (s *Service) LeaderGroupSize() (*big.Int, error) {
	return s.leaderGroup.Len()
}

func (s *Service) GetLeaderGroupHead() (*Validation, error) {
	validatorID, err := s.leaderGroup.Head()
	if err != nil {
		return nil, err
	}

	return s.GetValidation(validatorID)
}

// LeaderGroup lists all registered candidates.
func (s *Service) LeaderGroup() (map[thor.Address]*Validation, error) {
	group := make(map[thor.Address]*Validation)
	err := s.LeaderGroupIterator(func(validator thor.Address, entry *Validation) error {
		group[validator] = entry
		return nil
	})
	return group, err
}

func (s *Service) Add(
	validator thor.Address,
	endorser thor.Address,
	period uint32,
	stake *big.Int,
) error {
	if stake.Cmp(s.minStake) < 0 || stake.Cmp(s.maxStake) > 0 {
		return reverts.New("stake is out of range")
	}
	val, err := s.GetValidation(validator)
	if err != nil {
		return err
	}
	if !val.IsEmpty() {
		return reverts.New("validator already exists")
	}
	if period != thor.LowStakingPeriod() && period != thor.MediumStakingPeriod() && period != thor.HighStakingPeriod() {
		return reverts.New("period is out of boundaries")
	}

	entry := &Validation{
		Endorser:           endorser,
		Period:             period,
		CompleteIterations: 0,
		Status:             StatusQueued,
		LockedVET:          big.NewInt(0),
		QueuedVET:          stake,
		CooldownVET:        big.NewInt(0),
		PendingUnlockVET:   big.NewInt(0),
		WithdrawableVET:    big.NewInt(0),
		Weight:             big.NewInt(0),
	}

	if err = s.validatorQueue.Add(validator); err != nil {
		return err
	}

	return s.repo.setValidation(validator, entry, true)
}

func (s *Service) SignalExit(validator thor.Address, endorser thor.Address) error {
	validation, err := s.GetExistingValidation(validator)
	if err != nil {
		return err
	}
	if validation.Endorser != endorser {
		return reverts.New("invalid endorser for node")
	}
	if validation.Status != StatusActive {
		return reverts.New("can't signal exit while not active")
	}

	minBlock := validation.StartBlock + validation.Period*(validation.CurrentIteration())
	exitBlock, err := s.SetExitBlock(validator, minBlock)
	if err != nil {
		return err
	}
	validation.ExitBlock = &exitBlock

	return s.repo.setValidation(validator, validation, false)
}

func (s *Service) Evict(validator thor.Address, currentBlock uint32) error {
	validation, err := s.GetExistingValidation(validator)
	if err != nil {
		return err
	}

	exitBlock, err := s.SetExitBlock(validator, currentBlock+thor.EpochLength())
	if err != nil {
		return err
	}
	if validation.ExitBlock != nil && *validation.ExitBlock < exitBlock {
		exitBlock = *validation.ExitBlock
	}
	validation.ExitBlock = &exitBlock

	return s.repo.setValidation(validator, validation, false)
}

func (s *Service) IncreaseStake(validator thor.Address, endorser thor.Address, amount *big.Int) error {
	entry, err := s.GetExistingValidation(validator)
	if err != nil {
		return err
	}
	if entry.Endorser != endorser {
		return reverts.New("invalid endorser")
	}
	if entry.Status == StatusExit {
		return reverts.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && entry.ExitBlock != nil {
		return reverts.New("validator has signaled exit, cannot increase stake")
	}

	entry.QueuedVET = big.NewInt(0).Add(amount, entry.QueuedVET)

	return s.repo.setValidation(validator, entry, false)
}

func (s *Service) SetBeneficiary(validator, endorser, beneficiary thor.Address) error {
	entry, err := s.GetExistingValidation(validator)
	if err != nil {
		return err
	}
	if entry.Endorser != endorser {
		return reverts.New("invalid endorser")
	}
	if entry.Status == StatusExit || entry.ExitBlock != nil {
		return reverts.New("validator has exited or signaled exit, cannot set beneficiary")
	}
	if beneficiary.IsZero() {
		entry.Beneficiary = nil
	} else {
		entry.Beneficiary = &beneficiary
	}
	if err = s.repo.setValidation(validator, entry, false); err != nil {
		return errors.Wrap(err, "failed to set beneficiary")
	}
	return nil
}

func (s *Service) DecreaseStake(validator thor.Address, endorser thor.Address, amount *big.Int) (bool, error) {
	entry, err := s.GetExistingValidation(validator)
	if err != nil {
		return false, err
	}
	if entry.Endorser != endorser {
		return false, reverts.New("invalid endorser")
	}
	if entry.Status == StatusExit {
		return false, reverts.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && entry.ExitBlock != nil {
		return false, reverts.New("validator has signaled exit, cannot decrease stake")
	}

	if entry.Status == StatusActive {
		// We don't consider any increases, i.e., entry.QueuedVET. We only consider locked and current decreases.
		// The reason is that validator can instantly withdraw QueuedVET at any time.
		// We need to make sure the locked VET minus the sum of the current decreases is still above the minimum stake.
		nextPeriodTVL := big.NewInt(0).Sub(entry.LockedVET, entry.PendingUnlockVET)
		nextPeriodTVL = nextPeriodTVL.Sub(nextPeriodTVL, amount)
		if nextPeriodTVL.Cmp(s.minStake) < 0 {
			return false, reverts.New("next period stake is too low for validator")
		}
		entry.PendingUnlockVET = big.NewInt(0).Add(entry.PendingUnlockVET, amount)
	}

	if entry.Status == StatusQueued {
		// All the validator's stake exists within QueuedVET, so we need to make sure it maintains a minimum of MinStake.
		nextPeriodTVL := big.NewInt(0).Sub(entry.QueuedVET, amount)
		if nextPeriodTVL.Cmp(s.minStake) < 0 {
			return false, reverts.New("next period stake is too low for validator")
		}
		entry.QueuedVET = big.NewInt(0).Sub(entry.QueuedVET, amount)
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, amount)
	}

	return entry.Status == StatusQueued, s.repo.setValidation(validator, entry, false)
}

// WithdrawStake allows validations to withdraw any withdrawable stake.
// It also verifies the endorser and updates the validator totals.
func (s *Service) WithdrawStake(
	validator thor.Address,
	endorser thor.Address,
	currentBlock uint32,
) (*big.Int, *big.Int, error) {
	val, err := s.GetExistingValidation(validator)
	if err != nil {
		return nil, nil, err
	}
	if val.Endorser != endorser {
		return big.NewInt(0), big.NewInt(0), reverts.New("invalid endorser")
	}

	// calculate currently available VET to withdraw
	withdrawable := val.CalculateWithdrawableVET(currentBlock)

	// val has exited and waited for the cooldown period
	if val.ExitBlock != nil && *val.ExitBlock+thor.CooldownPeriod() <= currentBlock {
		val.CooldownVET = big.NewInt(0)
	}

	// if the validator is queued make sure to exit it
	if val.Status == StatusQueued {
		val.QueuedVET = big.NewInt(0)
		val.Status = StatusExit
		if err := s.validatorQueue.Remove(validator); err != nil {
			return nil, nil, err
		}
	}
	queuedVET := big.NewInt(0).Set(val.QueuedVET)
	// remove any que
	if val.QueuedVET.Sign() > 0 {
		val.QueuedVET = big.NewInt(0)
	}

	// no more withdraw after this
	val.WithdrawableVET = big.NewInt(0)
	if err := s.repo.setValidation(validator, val, false); err != nil {
		return nil, nil, err
	}

	return withdrawable, queuedVET, nil
}

func (s *Service) NextToActivate(maxLeaderGroupSize *big.Int) (*thor.Address, error) {
	leaderGroupLength, err := s.leaderGroup.Len()
	if err != nil {
		return nil, err
	}
	if leaderGroupLength.Cmp(maxLeaderGroupSize) >= 0 {
		return nil, errors.New("leader group is full")
	}
	// Check if queue is empty
	queuedSize, err := s.validatorQueue.Len()
	if err != nil {
		return nil, err
	}
	if queuedSize.Cmp(big.NewInt(0)) <= 0 {
		return nil, errors.New("no validator in the queue")
	}
	// pop the head of the queue
	validatorID, err := s.validatorQueue.Pop()
	if err != nil {
		return nil, err
	}
	// ensure validation exists
	if _, err = s.GetExistingValidation(validatorID); err != nil {
		return nil, err
	}

	return &validatorID, nil
}

// ExitValidator removes the validator from the active list and puts it in cooldown.
func (s *Service) ExitValidator(validator thor.Address, aggWeight *big.Int) (*delta.Exit, error) {
	entry, err := s.GetValidation(validator)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return nil, nil
	}
	exit := entry.exit(aggWeight)
	if err = s.leaderGroup.Remove(validator); err != nil {
		return nil, err
	}

	if err = s.repo.setValidation(validator, entry, false); err != nil {
		return nil, err
	}

	return exit, nil
}

// SetExitBlock sets the exit block for a validator.
// It ensures that the exit block is not already set for another validator.
// A validator cannot consume several epochs at once.
func (s *Service) SetExitBlock(validator thor.Address, minBlock uint32) (uint32, error) {
	start := minBlock
	for {
		existing, err := s.GetExitEpoch(start)
		if err != nil {
			return 0, err
		}
		if existing == validator {
			return start, nil
		}
		if existing.IsZero() {
			if err = s.repo.SetExit(start, validator); err != nil {
				return 0, errors.Wrap(err, "failed to set exit epoch")
			}
			return start, nil
		}
		start += thor.EpochLength()
	}
}

func (s *Service) GetExitEpoch(block uint32) (thor.Address, error) {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	validator, err := s.repo.GetExit(bigBlock)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get exit epoch")
	}
	return validator, nil
}

func (s *Service) GetDelegatorRewards(validator thor.Address, stakingPeriod uint32) (*big.Int, error) {
	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, stakingPeriod)
	key := thor.Blake2b([]byte("rewards"), validator.Bytes(), periodBytes)

	return s.repo.GetReward(key)
}

// ActivateValidator transitions a validator from queued to active status.
// It updates the validator's state and adds it to the leader group.
// Returns a delta object representing the state changes.
func (s *Service) ActivateValidator(
	validationID thor.Address,
	currentBlock uint32,
	aggRenew *delta.Renewal,
) (*delta.Renewal, error) {
	val, err := s.GetExistingValidation(validationID)
	if err != nil {
		return nil, err
	}

	// Update validator values
	// ensure a queued validator does not have locked vet
	if val.LockedVET.Sign() > 0 {
		return nil, errors.New("cannot activate validator with already locked vet")
	}
	// QueuedVET is now locked
	val.LockedVET = big.NewInt(0).Set(val.QueuedVET)
	// Reset QueuedVET - already locked-in
	val.QueuedVET = big.NewInt(0)

	weightedStake := stakes.NewWeightedStake(val.LockedVET, Multiplier)
	val.Weight = big.NewInt(0).Add(weightedStake.Weight(), aggRenew.NewLockedWeight)

	// Update validator status
	val.Status = StatusActive
	val.StartBlock = currentBlock

	// Add to the leader group list
	if err := s.leaderGroup.Add(validationID); err != nil {
		return nil, err
	}

	// Persist the updated validation state
	if err = s.repo.setValidation(validationID, val, false); err != nil {
		return nil, err
	}

	// Return delta representing the state changes
	validatorRenewal := &delta.Renewal{
		NewLockedVET:         val.LockedVET,
		NewLockedWeight:      val.Weight,
		QueuedDecrease:       val.LockedVET,
		QueuedDecreaseWeight: weightedStake.Weight(),
	}

	return validatorRenewal, nil
}

// UpdateOfflineBlock updates the offline block for a validator.
func (s *Service) UpdateOfflineBlock(validator thor.Address, block uint32, online bool) error {
	validation, err := s.GetExistingValidation(validator)
	if err != nil {
		return err
	}
	if online {
		validation.OfflineBlock = nil
	} else {
		validation.OfflineBlock = &block
	}

	return s.repo.setValidation(validator, validation, false)
}

func (s *Service) Renew(validator thor.Address, aggRenew *delta.Renewal, delegationWeight *big.Int) (*delta.Renewal, error) {
	validation, err := s.GetExistingValidation(validator)
	if err != nil {
		return nil, err
	}
	delta := validation.renew(aggRenew, delegationWeight)
	if err = s.repo.setValidation(validator, validation, false); err != nil {
		return nil, errors.Wrap(err, "failed to renew validator")
	}

	return delta, nil
}

//
// Repository methods
//

func (s *Service) GetValidation(validator thor.Address) (*Validation, error) {
	return s.repo.GetValidation(validator)
}

func (s *Service) GetExistingValidation(validator thor.Address) (*Validation, error) {
	v, err := s.GetValidation(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	if v.IsEmpty() {
		return nil, reverts.New("failed to get validator")
	}
	return v, nil
}

func (s *Service) LeaderGroupNext(prev thor.Address) (thor.Address, error) {
	return s.leaderGroup.Next(prev)
}

func (s *Service) ValidatorQueueNext(prev thor.Address) (thor.Address, error) {
	return s.validatorQueue.Next(prev)
}
