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
	"github.com/vechain/thor/v2/thor"
)

type Service struct {
	leaderGroup         *LinkedList
	validatorQueue      *LinkedList
	lowStakingPeriod    uint32
	mediumStakingPeriod uint32
	highStakingPeriod   uint32
	cooldownPeriod      uint32

	minStake    *big.Int
	maxStake    *big.Int
	repo        *Repository
	epochLength uint32
}

var (
	// todo fix this
	pkgValidatorWeightMultiplier *big.Int

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
	validatorWeightMultiplier *big.Int,
	cooldownPeriod uint32,
	epochLength uint32,
	lowStakingPeriod uint32,
	mediumStakingPeriod uint32,
	highStakingPeriod uint32,
	minStake *big.Int,
	maxStake *big.Int,
) *Service {
	repo := NewRepository(sctx)
	pkgValidatorWeightMultiplier = validatorWeightMultiplier

	return &Service{
		repo: repo,

		leaderGroup:         NewLinkedList(sctx, repo, slotActiveHead, slotActiveTail, slotActiveGroupSize),
		validatorQueue:      NewLinkedList(sctx, repo, slotQueuedHead, slotQueuedTail, slotQueuedGroupSize),
		lowStakingPeriod:    lowStakingPeriod,
		mediumStakingPeriod: mediumStakingPeriod,
		highStakingPeriod:   highStakingPeriod,

		cooldownPeriod: cooldownPeriod,
		epochLength:    epochLength,

		minStake: minStake,
		maxStake: maxStake,
	}
}

func (s *Service) GetCompletedPeriods(validationID thor.Address) (uint32, error) {
	v, err := s.GetValidation(validationID)
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

func (s *Service) LeaderGroupIterator(callback func(thor.Address, *Validation) error) error {
	return s.leaderGroup.Iter(callback)
}

// IsActive returns true if there are active validations.
func (s *Service) IsActive() (bool, error) {
	activeCount, err := s.leaderGroup.Count()
	if err != nil {
		return false, err
	}
	return activeCount.Cmp(big.NewInt(0)) > 0, nil
}

// FirstActive returns validator address of first entry.
func (s *Service) FirstActive() (*thor.Address, error) {
	validationID, err := s.leaderGroup.Head()
	return &validationID, err
}

// FirstQueued returns validator address of first entry.
func (s *Service) FirstQueued() (*thor.Address, error) {
	validationID, err := s.validatorQueue.Head()
	return &validationID, err
}

func (s *Service) QueuedGroupSize() (*big.Int, error) {
	return s.validatorQueue.Len()
}

func (s *Service) LeaderGroupSize() (*big.Int, error) {
	return s.leaderGroup.Len()
}

func (s *Service) AddLeaderGroup(address thor.Address, val *Validation) (bool, error) {
	return s.leaderGroup.Add(address, val)
}

func (s *Service) GetLeaderGroupHead() (*Validation, error) {
	return s.leaderGroup.Peek()
}

// LeaderGroup lists all registered candidates.
func (s *Service) LeaderGroup() (map[thor.Address]*Validation, error) {
	group := make(map[thor.Address]*Validation)
	err := s.LeaderGroupIterator(func(id thor.Address, entry *Validation) error {
		group[id] = entry
		return nil
	})
	return group, err
}

func (s *Service) Add(
	endorsor thor.Address,
	node thor.Address,
	period uint32,
	stake *big.Int,
) error {
	if stake.Cmp(s.minStake) < 0 || stake.Cmp(s.maxStake) > 0 {
		return errors.New("stake is out of range")
	}
	val, err := s.GetValidation(node)
	if err != nil {
		return err
	}
	if !val.IsEmpty() {
		return errors.New("validator already exists")
	}
	if period != s.lowStakingPeriod && period != s.mediumStakingPeriod && period != s.highStakingPeriod {
		return errors.New("period is out of boundaries")
	}

	entry := &Validation{
		Endorsor:           endorsor,
		Period:             period,
		CompleteIterations: 0,
		Status:             StatusQueued,
		Online:             true,
		LockedVET:          big.NewInt(0),
		QueuedVET:          stake,
		CooldownVET:        big.NewInt(0),
		PendingUnlockVET:   big.NewInt(0),
		WithdrawableVET:    big.NewInt(0),
		Weight:             big.NewInt(0),
	}

	added, err := s.validatorQueue.Add(node, entry)
	if err != nil {
		return err
	}
	if !added {
		return errors.New("failed to add validator to queue")
	}

	return nil
}

func (s *Service) SignalExit(endorsor thor.Address, id thor.Address) error {
	validator, err := s.GetExistingValidation(id)
	if err != nil {
		return err
	}
	if validator.Endorsor != endorsor {
		return errors.New("invalid endorsor for node")
	}
	if validator.Status != StatusActive {
		return errors.New("can't signal exit while not active")
	}

	minBlock := validator.StartBlock + validator.Period*(validator.CurrentIteration())
	exitBlock, err := s.SetExitBlock(id, minBlock)
	if err != nil {
		return err
	}
	validator.ExitBlock = &exitBlock

	return s.repo.SetValidation(id, validator, false)
}

func (s *Service) IncreaseStake(id thor.Address, endorsor thor.Address, amount *big.Int) error {
	entry, err := s.GetExistingValidation(id)
	if err != nil {
		return err
	}
	if entry.Endorsor != endorsor {
		return errors.New("invalid endorser")
	}
	if entry.Status == StatusExit {
		return errors.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && entry.ExitBlock != nil {
		return errors.New("validator has signaled exit, cannot increase stake")
	}

	entry.QueuedVET = big.NewInt(0).Add(amount, entry.QueuedVET)

	return s.SetValidation(id, entry, false)
}

func (s *Service) DecreaseStake(id thor.Address, endorsor thor.Address, amount *big.Int) error {
	entry, err := s.GetExistingValidation(id)
	if err != nil {
		return err
	}
	if entry.Endorsor != endorsor {
		return errors.New("invalid endorser")
	}
	if entry.Status == StatusExit {
		return errors.New("validator status is not queued or active")
	}
	if entry.Status == StatusActive && entry.ExitBlock != nil {
		return errors.New("validator has signaled exit, cannot decrease stake")
	}

	if entry.Status == StatusActive {
		// We don't consider any increases, i.e., entry.QueuedVET. We only consider locked and current decreases.
		// The reason is that validator can instantly withdraw QueuedVET at any time.
		// We need to make sure the locked VET minus the sum of the current decreases is still above the minimum stake.
		nextPeriodTVL := big.NewInt(0).Sub(entry.LockedVET, entry.PendingUnlockVET)
		nextPeriodTVL = nextPeriodTVL.Sub(nextPeriodTVL, amount)
		if nextPeriodTVL.Cmp(s.minStake) < 0 {
			return errors.New("next period stake is too low for validator")
		}
		entry.PendingUnlockVET = big.NewInt(0).Add(entry.PendingUnlockVET, amount)
	}

	if entry.Status == StatusQueued {
		// All the validator's stake exists within QueuedVET, so we need to make sure it maintains a minimum of MinStake.
		nextPeriodTVL := big.NewInt(0).Sub(entry.QueuedVET, amount)
		if nextPeriodTVL.Cmp(s.minStake) < 0 {
			return errors.New("next period stake is too low for validator")
		}
		entry.QueuedVET = big.NewInt(0).Sub(entry.QueuedVET, amount)
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, amount)
	}

	return s.SetValidation(id, entry, false)
}

// WithdrawStake allows validations to withdraw any withdrawable stake.
// It also verifies the endorsor and updates the validator totals.
func (s *Service) WithdrawStake(endorsor thor.Address, id thor.Address, currentBlock uint32) (*big.Int, error) {
	val, err := s.GetExistingValidation(id)
	if err != nil {
		return nil, err
	}
	if val.Endorsor != endorsor {
		return big.NewInt(0), errors.New("invalid endorser")
	}

	// calculate currently available VET to withdraw
	withdrawable := val.CalculateWithdrawableVET(currentBlock, s.cooldownPeriod)

	// val has exited and waited for the cooldown period
	if val.ExitBlock != nil && *val.ExitBlock+s.cooldownPeriod <= currentBlock {
		val.CooldownVET = big.NewInt(0)
	}

	// if the validator is queued make sure to exit it
	if val.Status == StatusQueued {
		val.QueuedVET = big.NewInt(0)
		val.Status = StatusExit
		if _, err := s.validatorQueue.Remove(id, val); err != nil {
			return nil, err
		}
	}
	// remove any queued
	if val.QueuedVET.Sign() > 0 {
		val.QueuedVET = big.NewInt(0)
	}

	// no more withdraw after this
	val.WithdrawableVET = big.NewInt(0)
	if err := s.SetValidation(id, val, false); err != nil {
		return nil, err
	}

	return withdrawable, nil
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
	id, val, err := s.validatorQueue.Pop()
	if err != nil {
		return nil, err
	}
	if val.IsEmpty() {
		return nil, errors.New("no validator in the queue")
	}

	return &id, nil
}

// ExitValidator removes the validator from the active list and puts it in cooldown.
func (s *Service) ExitValidator(id thor.Address) (*big.Int, *big.Int, *big.Int, error) {
	entry, err := s.GetValidation(id)
	if err != nil {
		return nil, nil, nil, err
	}
	if entry.IsEmpty() {
		return nil, nil, nil, nil
	}

	releaseLockedTVL := big.NewInt(0).Set(entry.LockedVET)
	releaseLockedTVLWeight := big.NewInt(0).Set(entry.Weight)
	releaseQueuedTVL := big.NewInt(0).Set(entry.QueuedVET)

	// move locked to cooldown
	entry.Status = StatusExit
	entry.CooldownVET = big.NewInt(0).Set(entry.LockedVET)
	entry.LockedVET = big.NewInt(0)
	entry.PendingUnlockVET = big.NewInt(0)
	entry.Weight = big.NewInt(0)

	// unlock pending stake
	if entry.QueuedVET.Sign() == 1 {
		// pending never contributes to weight as it's not active
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, entry.QueuedVET)
		entry.QueuedVET = big.NewInt(0)
	}

	entry.CompleteIterations++
	if _, err = s.leaderGroup.Remove(id, entry); err != nil {
		return nil, nil, nil, err
	}

	return releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, nil
}

// SetExitBlock sets the exit block for a validator.
// It ensures that the exit block is not already set for another validator.
// A validator cannot consume several epochs at once.
func (s *Service) SetExitBlock(id thor.Address, minBlock uint32) (uint32, error) {
	start := minBlock
	for {
		existing, err := s.GetExitEpoch(start)
		if err != nil {
			return 0, err
		}
		if existing == id {
			return start, nil
		}
		if existing.IsZero() {
			if err = s.repo.SetExit(start, id); err != nil {
				return 0, errors.Wrap(err, "failed to set exit epoch")
			}
			return start, nil
		}
		start += s.epochLength
	}
}

func (s *Service) GetExitEpoch(block uint32) (thor.Address, error) {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	id, err := s.repo.GetExit(bigBlock)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get exit epoch")
	}
	return id, nil
}

func (s *Service) GetDelegatorRewards(validationID thor.Address, stakingPeriod uint32) (*big.Int, error) {
	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, stakingPeriod)
	key := thor.Blake2b([]byte("rewards"), validationID.Bytes(), periodBytes)

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
	val, err := s.GetValidation(validationID)
	if err != nil {
		return nil, err
	}
	if val.IsEmpty() {
		return nil, errors.New("validator not found")
	}

	// Update validator values
	validatorLocked := big.NewInt(0).Add(val.LockedVET, val.QueuedVET)
	val.QueuedVET = big.NewInt(0)
	val.LockedVET = validatorLocked
	// x2 multiplier for validator's stake
	validatorWeight := big.NewInt(0).Mul(validatorLocked, pkgValidatorWeightMultiplier)
	val.Weight = big.NewInt(0).Add(validatorWeight, aggRenew.NewLockedWeight)

	// Update validator status
	val.Status = StatusActive
	val.Online = true
	val.StartBlock = currentBlock

	// Add to active list
	added, err := s.AddLeaderGroup(validationID, val)
	if err != nil {
		return nil, err
	}
	if !added {
		return nil, errors.New("failed to add validator to active list")
	}

	// Persist the updated validation state
	if err = s.SetValidation(validationID, val, false); err != nil {
		return nil, err
	}

	// Return delta representing the state changes
	validatorRenewal := &delta.Renewal{
		NewLockedVET:         val.LockedVET,
		NewLockedWeight:      val.Weight,
		QueuedDecrease:       val.LockedVET,
		QueuedDecreaseWeight: big.NewInt(0).Mul(val.LockedVET, pkgValidatorWeightMultiplier),
	}

	return validatorRenewal, nil
}

//
// Repository methods
//

func (s *Service) GetValidation(id thor.Address) (*Validation, error) {
	return s.repo.GetValidation(id)
}

func (s *Service) GetExistingValidation(id thor.Address) (*Validation, error) {
	v, err := s.GetValidation(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	if v.IsEmpty() {
		return nil, errors.New("failed to get validator")
	}
	return v, nil
}

func (s *Service) SetValidation(id thor.Address, entry *Validation, isNew bool) error {
	return s.repo.SetValidation(id, entry, isNew)
}
