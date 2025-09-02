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
	minStake uint64
	maxStake uint64

	repo *Repository
}

var (
	exitMaxTry = 20 // revert transaction if after these attempts an exit block is not found
)

func New(sctx *solidity.Context,
	minStake uint64,
	maxStake uint64,
) *Service {
	repo := NewRepository(sctx)

	return &Service{
		repo:     repo,
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
	return s.repo.iterateActive(callbacks...)
}

// IsActive returns true if there are active validations.
func (s *Service) IsActive() (bool, error) {
	activeCount, err := s.repo.activeListSize()
	if err != nil {
		return false, err
	}
	return activeCount > 0, nil
}

// FirstActive returns validator address of first entry.
func (s *Service) FirstActive() (*thor.Address, error) {
	validator, err := s.repo.activeListHead()
	return &validator, err
}

// FirstQueued returns validator address of first entry.
func (s *Service) FirstQueued() (*thor.Address, error) {
	validator, err := s.repo.queuedListHead()
	return &validator, err
}

func (s *Service) QueuedGroupSize() (uint64, error) {
	return s.repo.queuedListSize()
}

func (s *Service) LeaderGroupSize() (uint64, error) {
	return s.repo.activeListSize()
}

func (s *Service) GetLeaderGroupHead() (*Validation, error) {
	validatorID, err := s.repo.activeListHead()
	if err != nil {
		return nil, err
	}

	return s.GetValidation(validatorID)
}

// LeaderGroup lists all registered candidates.
func (s *Service) LeaderGroup() ([]Leader, error) {
	group := make([]Leader, 0, thor.InitialMaxBlockProposers)
	if err := s.repo.iterateActive(func(validator thor.Address, entry *Validation) error {
		group = append(group, Leader{
			Address:     validator,
			Endorser:    entry.Endorser,
			Beneficiary: entry.Beneficiary,
			Active:      entry.IsOnline(),
			Weight:      entry.Weight,
		})

		return nil
	}); err != nil {
		return nil, err
	}
	return group, nil
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

	return s.repo.addValidation(validator, entry)
}

func (s *Service) SignalExit(validator thor.Address, validation *Validation) error {
	minBlock := validation.StartBlock + validation.Period*(validation.CurrentIteration())
	return s.markValidatorExit(validator, minBlock, exitMaxTry)
}

func (s *Service) Evict(validator thor.Address, currentBlock uint32) error {
	return s.markValidatorExit(validator, currentBlock+thor.EpochLength(), int(thor.InitialMaxBlockProposers))
}

func (s *Service) markValidatorExit(validator thor.Address, minblock uint32, maxTry int) error {
	validation, err := s.GetExistingValidation(validator)
	if err != nil {
		return err
	}

	exitBlock, err := s.SetExitBlock(validator, minblock, maxTry)
	if err != nil {
		return err
	}

	validation.ExitBlock = &exitBlock

	return s.repo.updateValidation(validator, validation)
}

func (s *Service) IncreaseStake(validator thor.Address, validation *Validation, amount uint64) error {
	validation.QueuedVET += amount

	return s.repo.updateValidation(validator, validation)
}

func (s *Service) SetBeneficiary(validator thor.Address, validation *Validation, beneficiary thor.Address) error {
	if beneficiary.IsZero() {
		validation.Beneficiary = nil
	} else {
		validation.Beneficiary = &beneficiary
	}
	if err := s.repo.updateValidation(validator, validation); err != nil {
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

	return s.repo.updateValidation(validator, validation)
}

// WithdrawStake allows validations to withdraw any withdrawable stake.
// It also verifies the endorser and updates the validator totals.
func (s *Service) WithdrawStake(
	validator thor.Address,
	validation *Validation,
	currentBlock uint32,
) (uint64, uint64, error) {
	// if the validator is queued make sure to exit it
	if validation.Status == StatusQueued {
		withdrawable := validation.WithdrawableVET
		queuedVET := validation.QueuedVET
		withdrawable += queuedVET

		validation.QueuedVET = 0
		validation.WithdrawableVET = 0
		validation.Status = StatusExit
		if err := s.repo.removeQueued(validator, validation); err != nil {
			return 0, 0, err
		}

		return withdrawable, queuedVET, nil
	}

	withdrawable := validation.WithdrawableVET
	queuedVET := validation.QueuedVET
	withdrawable += validation.QueuedVET

	// reset queued and withdrawable
	validation.QueuedVET = 0
	validation.WithdrawableVET = 0

	// validator has exited and waited for the cooldown period
	if validation.ExitBlock != nil && *validation.ExitBlock+thor.CooldownPeriod() <= currentBlock {
		withdrawable += validation.CooldownVET
		validation.CooldownVET = 0
	}

	if err := s.repo.updateValidation(validator, validation); err != nil {
		return 0, 0, err
	}

	return withdrawable, queuedVET, nil
}

func (s *Service) NextToActivate(maxLeaderGroupSize uint64) (thor.Address, *Validation, error) {
	leaderGroupLength, err := s.repo.activeListSize()
	if err != nil {
		return thor.Address{}, nil, err
	}
	if leaderGroupLength >= maxLeaderGroupSize {
		return thor.Address{}, nil, errors.New("leader group is full")
	}
	// Check if queue is empty
	queuedSize, err := s.repo.queuedListSize()
	if err != nil {
		return thor.Address{}, nil, err
	}
	if queuedSize <= 0 {
		return thor.Address{}, nil, errors.New("no validator in the queue")
	}
	// pop the head from the queue
	validator, validation, err := s.repo.popQueued()
	if err != nil {
		return thor.Address{}, nil, err
	}

	return validator, validation, nil
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

	if err = s.repo.removeActive(validator, entry); err != nil {
		return nil, err
	}

	return exit, nil
}

// SetExitBlock sets the exit block for a validator.
// It ensures that the exit block is not already set for another validator.
// A validator cannot consume several epochs at once.
func (s *Service) SetExitBlock(validator thor.Address, minBlock uint32, maxTry int) (uint32, error) {
	start := minBlock
	for range maxTry {
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

	return 0, ErrMaxTryReached
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
	validator thor.Address,
	validation *Validation,
	currentBlock uint32,
	aggRenew *globalstats.Renewal,
) (*globalstats.Renewal, error) {
	// Update validator values
	// ensure a queued validator does not have locked vet
	if validation.LockedVET > 0 {
		return nil, errors.New("cannot activate validator with already locked vet")
	}

	queuedDecrease := validation.QueuedVET

	// QueuedVET is now locked
	validation.LockedVET = validation.QueuedVET
	// Reset QueuedVET - already locked-in
	validation.QueuedVET = 0

	mul := Multiplier
	if aggRenew.LockedIncrease.VET-aggRenew.LockedDecrease.VET > 0 {
		// if validator has delegations, multiplier is 200%
		mul = MultiplierWithDelegations
	}
	lockedIncrease := stakes.NewWeightedStakeWithMultiplier(validation.LockedVET, mul)

	// attach all delegation's weight
	validation.Weight = lockedIncrease.Weight + aggRenew.LockedIncrease.Weight - aggRenew.LockedDecrease.Weight

	// Update validator status
	validation.Status = StatusActive
	validation.StartBlock = currentBlock

	// Add to the leader group list and update the validation
	if err := s.repo.addActive(validator, validation); err != nil {
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

	return s.repo.updateValidation(validator, validation)
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
	if err = s.repo.updateValidation(validator, validation); err != nil {
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

func (s *Service) NextEntry(prev thor.Address) (thor.Address, error) {
	return s.repo.nextEntry(prev)
}
