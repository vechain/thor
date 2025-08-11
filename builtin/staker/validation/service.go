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
	"github.com/vechain/thor/v2/thor"
)

type Service struct {
	leaderGroup         *linkedlist.LinkedList
	validatorQueue      *linkedlist.LinkedList


	repo        *Repository

}

func New(sctx *solidity.Context) *Service {
	repo := NewRepository(sctx)
	return &Service{
		repo: repo,
		validatorQueue: linkedlist.NewLinkedList(sctx, solidity.NumToSlot(5)),
		leaderGroup:    linkedlist.NewLinkedList(sctx, solidity.NumToSlot(9)),
	}
}


func (s *Service) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) error {
	val, err := s.GetValidation(node)
	if err != nil {
		return err
	}

	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, val.CurrentIteration())
	key := thor.Keccak256(node.Bytes(), periodBytes)

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


func (s *Service) QueuedGroupSize() (*big.Int, error) {
	return s.validatorQueue.Len()
}

func (s *Service) LeaderGroupSize() (*big.Int, error) {
	return s.leaderGroup.Len()
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
func (s *Service) ExitValidator(validator thor.Address) (*delta.Exit, error) {
	entry, err := s.GetValidation(validator)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return nil, nil
	}
	exit := entry.Exit()
	if err = s.leaderGroup.Remove(validator); err != nil {
		return nil, err
	}

	if err = s.SetValidation(validator, entry, false); err != nil {
		return nil, err
	}

	return exit, nil
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

	// x2 multiplier for validator's stake
	weightedStake := WeightedStake(val.LockedVET)
	val.Weight = big.NewInt(0).Add(weightedStake.Weight(), aggRenew.NewLockedWeight)

	// Update validator status
	val.Status = StatusActive
	val.Online = true
	val.StartBlock = currentBlock

	// Add to the leader group list
	if err := s.leaderGroup.Add(validationID); err != nil {
		return nil, err
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
		QueuedDecreaseWeight: weightedStake.Weight(),
	}

	return validatorRenewal, nil
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
		return nil, errors.New("failed to get validator")
	}
	return v, nil
}

func (s *Service) SetValidation(validator thor.Address, entry *Validation, isNew bool) error {
	return s.repo.SetValidation(validator, entry)
}
