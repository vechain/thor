// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/thor"
)

// Housekeep iterates over validations, move to cooldown
// take the oldest validator and move to exited
func (s *Staker) Housekeep(currentBlock uint32) (bool, map[thor.Address]*Validation, error) {
	// we only perform housekeeping at the start of epochs
	if currentBlock%epochLength != 0 {
		return false, nil, nil
	}

	logger.Info("performing housekeeping", "block", currentBlock)

	hasUpdates := false
	validatorExitID := thor.Address{}
	activeValidators := make(map[thor.Address]*Validation)
	renewal := &Renewal{
		ChangeTVL:            big.NewInt(0),
		ChangeWeight:         big.NewInt(0),
		QueuedDecrease:       big.NewInt(0),
		QueuedDecreaseWeight: big.NewInt(0),
	}

	iteratorLeaderGroup := func(id thor.Address, entry *Validation) error {
		if entry.ExitBlock != nil && currentBlock == *entry.ExitBlock {
			validatorExitID = id
			return nil
		}

		isPeriodEnd := entry.IsPeriodEnd(currentBlock)
		if !isPeriodEnd { // early exit - validator is not due for renewal
			activeValidators[id] = entry
			return nil
		}

		if entry.ExitBlock != nil { // early exit, validator is due to exit but has not reached exit block
			activeValidators[id] = entry
			logger.Debug("validator exit delayed", "node", id.String(), "exit-block", entry.ExitBlock)
			return nil
		}

		// validator has auto renew enabled and is due for renewal
		if err := s.performRenewalUpdates(id, entry, renewal); err != nil {
			return err
		}
		hasUpdates = true
		activeValidators[id] = entry
		return nil
	}

	// perform the iteration
	if err := s.validations.LeaderGroupIterator(iteratorLeaderGroup); err != nil {
		return false, nil, err
	}

	// update totals
	if err := s.storage.lockedVET.Add(renewal.ChangeTVL); err != nil {
		return false, nil, err
	}
	if err := s.storage.lockedWeight.Add(renewal.ChangeWeight); err != nil {
		return false, nil, err
	}
	if err := s.storage.queuedVET.Sub(renewal.QueuedDecrease); err != nil {
		return false, nil, err
	}
	if err := s.storage.queuedWeight.Sub(renewal.QueuedDecreaseWeight); err != nil {
		return false, nil, err
	}

	if !validatorExitID.IsZero() {
		hasUpdates = true
		logger.Info("exiting validator", "id", validatorExitID)
		if err := s.validations.ExitValidator(validatorExitID); err != nil {
			return false, nil, err
		}
	}

	// fill any remaining leader group slots with validations from the queue
	activated, err := s.activateValidators(currentBlock)
	if err != nil {
		return false, nil, err
	}
	if len(activated) > 0 {
		hasUpdates = true
		if activated == nil {
			return false, nil, errors.New("activate validators returned `nil`")
		}
		for _, active := range activated {
			validation, err := s.Get(*active)
			if err != nil {
				return false, nil, err
			}
			activeValidators[*active] = validation
		}
	}

	logger.Info("performed housekeeping", "block", currentBlock, "updates", hasUpdates, "activated", activated)

	if hasUpdates {
		return hasUpdates, activeValidators, nil
	}
	return hasUpdates, nil, nil
}

func (s *Staker) performRenewalUpdates(id thor.Address, validator *Validation, renewal *Renewal) error {
	aggregation, err := s.storage.GetAggregation(id)
	if err != nil {
		return err
	}

	// renew the validator & delegations
	validatorRenewal := validator.Renew()
	delegationsRenewal := aggregation.Renew()
	if err := s.storage.SetAggregation(id, aggregation, false); err != nil {
		return err
	}

	// calculate the new totals for validator + delegations
	changeTVL := big.NewInt(0).Add(validatorRenewal.ChangeTVL, delegationsRenewal.ChangeTVL)
	changeWeight := big.NewInt(0).Add(validatorRenewal.ChangeWeight, delegationsRenewal.ChangeWeight)
	queuedDecrease := big.NewInt(0).Add(validatorRenewal.QueuedDecrease, delegationsRenewal.QueuedDecrease)
	queuedWeight := big.NewInt(0).Add(validatorRenewal.QueuedDecreaseWeight, delegationsRenewal.QueuedDecreaseWeight)

	// set the new totals
	validator.LockedVET = big.NewInt(0).Add(validator.LockedVET, validatorRenewal.ChangeTVL)
	validator.Weight = big.NewInt(0).Add(validator.Weight, changeWeight)
	renewal.ChangeTVL = big.NewInt(0).Add(renewal.ChangeTVL, changeTVL)
	renewal.ChangeWeight = big.NewInt(0).Add(renewal.ChangeWeight, changeWeight)
	renewal.QueuedDecrease = big.NewInt(0).Add(renewal.QueuedDecrease, queuedDecrease)
	renewal.QueuedDecreaseWeight = big.NewInt(0).Add(renewal.QueuedDecreaseWeight, queuedWeight)
	return s.storage.SetValidation(id, validator, false)
}

func (s *Staker) activateValidators(currentBlock uint32) ([]*thor.Address, error) {
	queuedSize, err := s.QueuedGroupSize()
	if err != nil {
		return nil, err
	}
	leaderSize, err := s.LeaderGroupSize()
	if err != nil {
		return nil, err
	}
	maxSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, err
	}
	if leaderSize.Cmp(maxSize) >= 0 {
		return nil, nil
	}

	if queuedSize.Cmp(big.NewInt(0)) > 0 {
		queuedCount := queuedSize.Int64()
		leaderDelta := maxSize.Int64() - leaderSize.Int64()
		if leaderDelta > 0 {
			if leaderDelta < queuedCount {
				queuedCount = leaderDelta
			}
		} else {
			return nil, nil
		}

		activated := make([]*thor.Address, queuedCount)

		for i := int64(0); i < queuedCount; i++ {
			id, err := s.validations.ActivateNext(currentBlock, s.params)
			if err != nil {
				return nil, err
			}
			activated[i] = id
		}

		return activated, nil
	}

	return nil, nil
}

func (s *Staker) Transition(currentBlock uint32) (bool, error) {
	active, err := s.IsPoSActive()
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	maxProposers, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil || maxProposers.Cmp(big.NewInt(0)) == 0 {
		maxProposers = big.NewInt(0).SetUint64(thor.InitialMaxBlockProposers)
	}

	queueSize, err := s.validations.validatorQueue.Len()
	if err != nil {
		return false, err
	}

	// if the queue size is not AT LEAST 2/3 of the maxProposers, then return nil
	minimum := big.NewFloat(0).SetInt(maxProposers)
	minimum.Mul(minimum, big.NewFloat(2))
	minimum.Quo(minimum, big.NewFloat(3))
	if big.NewFloat(0).SetInt(queueSize).Cmp(minimum) < 0 {
		return false, nil
	}
	ids, err := s.activateValidators(currentBlock)
	if err != nil {
		return false, err
	}
	logger.Info("activated validations", "count", len(ids))

	return true, nil
}
