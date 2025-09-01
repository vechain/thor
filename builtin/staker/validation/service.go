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
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/linkedlist"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

type Leader struct {
	Address     thor.Address
	Endorser    thor.Address
	Beneficiary *thor.Address
	Active      bool
	Weight      uint64
}

type Service struct {
	leaderGroup    *linkedlist.LinkedList
	validatorQueue *linkedlist.LinkedList

	minStake uint64
	maxStake uint64

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
	minStake uint64,
	maxStake uint64,
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

	rewards, err := s.repo.getReward(key)
	if err != nil {
		return err
	}

	return s.repo.setReward(key, big.NewInt(0).Add(rewards, reward), false)
}

func (s *Service) LeaderGroupIterator(callbacks ...func(thor.Address, *Validation) error) error {
	return s.leaderGroup.Iter(func(address thor.Address) error {
		// Fetch the validation object for this address
		validation, err := s.repo.getValidation(address)
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
func (s *Service) LeaderGroup() ([]Leader, error) {
	group := make([]Leader, 0, thor.InitialMaxBlockProposers)
	err := s.LeaderGroupIterator(func(validator thor.Address, entry *Validation) error {
		group = append(group, Leader{
			Address:     validator,
			Endorser:    entry.Endorser,
			Beneficiary: entry.Beneficiary,
			Active:      entry.IsOnline(),
			Weight:      entry.Weight,
		})

		return nil
	})
	return group, err
}

func (s *Service) Add(
	validator thor.Address,
	endorser thor.Address,
	period uint32,
	stake uint64,
) error {
	entry := &Validation{
		Endorser:           endorser,
		Period:             period,
		CompleteIterations: 0,
		Status:             StatusQueued,
		LockedVET:          0,
		QueuedVET:          stake,
		CooldownVET:        0,
		PendingUnlockVET:   0,
		WithdrawableVET:    0,
		Weight:             0,
	}

	if err := s.validatorQueue.Add(validator); err != nil {
		return err
	}

	return s.repo.setValidation(validator, entry, true)
}

func (s *Service) SignalExit(validator thor.Address, validation *Validation) error {
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

func (s *Service) IncreaseStake(validator thor.Address, validation *Validation, amount uint64) error {
	validation.QueuedVET += amount

	return s.repo.setValidation(validator, validation, false)
}

func (s *Service) SetBeneficiary(validator thor.Address, validation *Validation, beneficiary thor.Address) error {
	if beneficiary.IsZero() {
		validation.Beneficiary = nil
	} else {
		validation.Beneficiary = &beneficiary
	}
	if err := s.repo.setValidation(validator, validation, false); err != nil {
		return errors.Wrap(err, "failed to set beneficiary")
	}
	return nil
}

func (s *Service) DecreaseStake(validator thor.Address, validation *Validation, amount uint64) error {
	if validation.Status == StatusActive {
		validation.PendingUnlockVET += amount
	}

	if validation.Status == StatusQueued {
		validation.QueuedVET -= amount
		validation.WithdrawableVET += amount
	}

	return s.repo.setValidation(validator, validation, false)
}

// WithdrawStake allows validations to withdraw any withdrawable stake.
// It also verifies the endorser and updates the validator totals.
func (s *Service) WithdrawStake(
	validator thor.Address,
	validation *Validation,
	currentBlock uint32,
) (uint64, uint64, error) {
	// calculate currently available VET to withdraw
	withdrawable := validation.CalculateWithdrawableVET(currentBlock)

	// val has exited and waited for the cooldown period
	if validation.ExitBlock != nil && *validation.ExitBlock+thor.CooldownPeriod() <= currentBlock {
		validation.CooldownVET = 0
	}

	// if the validator is queued make sure to exit it
	if validation.Status == StatusQueued {
		validation.QueuedVET = 0
		validation.Status = StatusExit
		if err := s.validatorQueue.Remove(validator); err != nil {
			return 0, 0, err
		}
	}
	queuedVET := validation.QueuedVET
	// remove any queued
	if validation.QueuedVET > 0 {
		validation.QueuedVET = 0
	}

	// no more withdraw after this
	validation.WithdrawableVET = 0
	if err := s.repo.setValidation(validator, validation, false); err != nil {
		return 0, 0, err
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
func (s *Service) ExitValidator(validator thor.Address) (*globalstats.Exit, error) {
	entry, err := s.GetValidation(validator)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return nil, nil
	}
	exit := entry.exit()
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
			if err = s.repo.setExit(start, validator); err != nil {
				return 0, errors.Wrap(err, "failed to set exit epoch")
			}
			return start, nil
		}
		start += thor.EpochLength()
	}
}

func (s *Service) GetExitEpoch(block uint32) (thor.Address, error) {
	validator, err := s.repo.getExit(block)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get exit epoch")
	}
	return validator, nil
}

func (s *Service) GetDelegatorRewards(validator thor.Address, stakingPeriod uint32) (*big.Int, error) {
	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, stakingPeriod)
	key := thor.Blake2b([]byte("rewards"), validator.Bytes(), periodBytes)

	return s.repo.getReward(key)
}

// ActivateValidator transitions a validator from queued to active status.
// It updates the validator's state and adds it to the leader group.
// Returns a delta object representing the state changes.
func (s *Service) ActivateValidator(
	validationID thor.Address,
	currentBlock uint32,
	aggRenew *globalstats.Renewal,
) (*globalstats.Renewal, error) {
	val, err := s.GetExistingValidation(validationID)
	if err != nil {
		return nil, err
	}

	// Update validator values
	// ensure a queued validator does not have locked vet
	if val.LockedVET > 0 {
		return nil, errors.New("cannot activate validator with already locked vet")
	}

	queuedDecrease := val.QueuedVET

	// QueuedVET is now locked
	val.LockedVET = val.QueuedVET
	// Reset QueuedVET - already locked-in
	val.QueuedVET = 0

	mul := Multiplier
	if aggRenew.LockedIncrease.VET-aggRenew.LockedDecrease.VET > 0 {
		// if validator has delegations, multiplier is 200%
		mul = MultiplierWithDelegations
	}
	lockedIncrease := stakes.NewWeightedStakeWithMultiplier(val.LockedVET, mul)

	// attach all delegation's weight
	val.Weight = lockedIncrease.Weight + aggRenew.LockedIncrease.Weight - aggRenew.LockedDecrease.Weight

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

	// Return renewal that only representing the state changes of this validator
	validatorRenewal := &globalstats.Renewal{
		LockedIncrease: lockedIncrease,
		LockedDecrease: stakes.NewWeightedStake(0, 0), // New validator does not have locked decrease
		QueuedDecrease: queuedDecrease,
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

func (s *Service) Renew(validator thor.Address, delegationWeight uint64) (*globalstats.Renewal, error) {
	validation, err := s.GetExistingValidation(validator)
	if err != nil {
		return nil, err
	}
	delta, err := validation.renew(delegationWeight)
	if err != nil {
		return nil, err
	}
	if err = s.repo.setValidation(validator, validation, false); err != nil {
		return nil, errors.Wrap(err, "failed to renew validator")
	}

	return delta, nil
}

//
// Repository methods
//

func (s *Service) GetValidation(validator thor.Address) (*Validation, error) {
	return s.repo.getValidation(validator)
}

func (s *Service) GetExistingValidation(validator thor.Address) (*Validation, error) {
	v, err := s.GetValidation(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	if v.IsEmpty() {
		return nil, errors.New("failed to get existing validator")
	}
	return v, nil
}

func (s *Service) LeaderGroupNext(prev thor.Address) (thor.Address, error) {
	return s.leaderGroup.Next(prev)
}

func (s *Service) ValidatorQueueNext(prev thor.Address) (thor.Address, error) {
	return s.validatorQueue.Next(prev)
}
