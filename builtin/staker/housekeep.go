// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

// State transition types
type EpochTransition struct {
	Block           uint32
	Renewals        []ValidatorRenewal
	ExitValidatorID *thor.Address
	ActivationCount int64
}

type ValidatorRenewal struct {
	ValidatorID     thor.Address
	NewState        *validation.Validation
	ValidatorDelta  *delta.Renewal
	DelegationDelta *delta.Renewal
}

// Housekeep performs epoch transitions at epoch boundaries
func (s *Staker) Housekeep(currentBlock uint32) (bool, map[thor.Address]*validation.Validation, error) {
	if currentBlock%epochLength != 0 {
		return false, nil, nil
	}

	logger.Info("🏠performing housekeeping", "block", currentBlock)

	transition, err := s.ComputeEpochTransition(currentBlock)
	if err != nil {
		return false, nil, err
	}

	if transition == nil || (len(transition.Renewals) == 0 && transition.ExitValidatorID == nil && transition.ActivationCount == 0) {
		return false, nil, nil
	}

	if err := s.ApplyEpochTransition(transition); err != nil {
		return false, nil, err
	}

	// Build active validators map
	activeValidators := s.buildActiveValidatorsFromTransition(transition)

	logger.Info("performed housekeeping", "block", currentBlock, "updates", true)
	return true, activeValidators, nil
}

// ComputeEpochTransition calculates all state changes needed for an epoch transition
func (s *Staker) ComputeEpochTransition(currentBlock uint32) (*EpochTransition, error) {
	var err error
	if currentBlock%epochLength != 0 {
		return nil, nil // No transition needed
	}

	transition := &EpochTransition{Block: currentBlock}

	// 1. Compute all renewals
	transition.Renewals, err = s.computeRenewals(currentBlock)
	if err != nil {
		return nil, err
	}

	// 2. Compute all exits
	transition.ExitValidatorID, err = s.computeExits(currentBlock)
	if err != nil {
		return nil, err
	}

	// 3. Compute all activations
	transition.ActivationCount, err = s.computeActivations()
	if err != nil {
		return nil, err
	}

	return transition, nil
}

func (s *Staker) computeRenewals(currentBlock uint32) ([]ValidatorRenewal, error) {
	var renewals []ValidatorRenewal

	// Collect all validators due for renewal
	err := s.validationService.LeaderGroupIterator(func(validationID thor.Address, entry *validation.Validation) error {
		// Skip validators due to exit
		if entry.ExitBlock != nil {
			return nil
		}

		// Check if period is ending
		if !entry.IsPeriodEnd(currentBlock) {
			return nil
		}

		// Compute renewal deltas
		validatorRenewal := entry.Renew()
		delegationsRenewal, err := s.aggregationService.Renew(validationID)
		if err != nil {
			return err
		}

		// Calculate new weight and locked VET
		changeWeight := big.NewInt(0).Add(validatorRenewal.NewLockedWeight, delegationsRenewal.NewLockedWeight)
		entry.LockedVET = big.NewInt(0).Add(entry.LockedVET, validatorRenewal.NewLockedVET)
		entry.Weight = big.NewInt(0).Add(entry.Weight, changeWeight)

		renewals = append(renewals, ValidatorRenewal{
			ValidatorID:     validationID,
			NewState:        entry,
			ValidatorDelta:  validatorRenewal,
			DelegationDelta: delegationsRenewal,
		})

		return nil
	})

	return renewals, err
}

func (s *Staker) computeExits(currentBlock uint32) (*thor.Address, error) {
	var exitValidatorID thor.Address

	// Find the last validator in iteration order that should exit this block
	// Do NOT call ExitValidator here - just identify which validator should exit
	err := s.validationService.LeaderGroupIterator(func(validationID thor.Address, entry *validation.Validation) error {
		if entry.ExitBlock != nil && currentBlock == *entry.ExitBlock {
			// should never be possible for two validators to exit at the same block
			if !exitValidatorID.IsZero() {
				return errors.Errorf("found more than one validator exit in the same block: ValidatorID: %s, ValidatorID: %s", exitValidatorID, validationID)
			}
			// Just record which validator should exit (matches original behavior)
			exitValidatorID = validationID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// If we found a validator to exit, prepare the exit
	if !exitValidatorID.IsZero() {
		return &exitValidatorID, nil
	}

	return nil, nil
}

func (s *Staker) computeActivations() (int64, error) {
	// Calculate how many validators can be activated
	queuedSize, err := s.QueuedGroupSize()
	if err != nil {
		return 0, err
	}
	leaderSize, err := s.LeaderGroupSize()
	if err != nil {
		return 0, err
	}
	maxSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return 0, err
	}

	// If full or nothing queued then no activations
	if leaderSize.Cmp(maxSize) >= 0 || queuedSize.Sign() <= 0 {
		return 0, nil
	}

	leaderDelta := maxSize.Int64() - leaderSize.Int64()
	if leaderDelta <= 0 {
		return 0, nil
	}

	queuedCount := queuedSize.Int64()
	if leaderDelta < queuedCount {
		return leaderDelta, nil
	}
	return queuedCount, nil
}

// ApplyEpochTransition applies all computed changes
func (s *Staker) ApplyEpochTransition(transition *EpochTransition) error {
	logger.Info("applying epoch transition", "block", transition.Block)

	accumulatedRenewal := delta.NewRenewal()
	// Apply renewals
	for _, renewal := range transition.Renewals {
		accumulatedRenewal.Add(renewal.ValidatorDelta)
		accumulatedRenewal.Add(renewal.DelegationDelta)

		// Update validator state
		if err := s.validationService.SetValidation(renewal.ValidatorID, renewal.NewState, false); err != nil {
			return err
		}
	}
	// Apply accumulated renewals to global stats
	if err := s.globalStatsService.ApplyRenewal(accumulatedRenewal); err != nil {
		return err
	}

	// Apply exits
	if transition.ExitValidatorID != nil {
		logger.Info("exiting validator", "id", transition.ExitValidatorID)

		// Now call ExitValidator to get the actual exit details and perform the exit
		exit, err := s.validationService.ExitValidator(*transition.ExitValidatorID)
		if err != nil {
			return err
		}

		aggExit, err := s.aggregationService.Exit(*transition.ExitValidatorID)
		if err != nil {
			return err
		}

		if err := s.globalStatsService.ApplyExit(exit.Add(aggExit)); err != nil {
			return err
		}
	}

	// Apply activations using existing method
	maxLeaderGroupSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return err
	}

	for range transition.ActivationCount {
		_, err := s.ActivateNextValidator(transition.Block, maxLeaderGroupSize)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Staker) buildActiveValidatorsFromTransition(transition *EpochTransition) map[thor.Address]*validation.Validation {
	if transition == nil {
		return nil
	}

	activeValidators := make(map[thor.Address]*validation.Validation)

	// After all transitions are applied, just read the current leader group
	// This captures renewed validators, excludes exited ones, and includes newly activated ones
	err := s.validationService.LeaderGroupIterator(func(validationID thor.Address, entry *validation.Validation) error {
		activeValidators[validationID] = entry
		return nil
	})
	if err != nil {
		logger.Error("failed to build active validators map", "error", err)
		return nil
	}

	return activeValidators
}

// Transition activates the staker contract when sufficient validators are queued
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

	queueSize, err := s.validationService.QueuedGroupSize()
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

	// Use existing activateValidators method for transition
	ids, err := s.activateValidators(currentBlock)
	if err != nil {
		return false, err
	}
	logger.Info("activated validations", "count", len(ids))

	return true, nil
}

// activateValidators is kept for the Transition method
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

	// no one is in the queue
	if queuedSize.Cmp(big.NewInt(0)) <= 0 {
		return nil, nil
	}

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
	maxLeaderGroupSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, err
	}

	for i := int64(0); i < queuedCount; i++ {
		id, err := s.ActivateNextValidator(currentBlock, maxLeaderGroupSize)
		if err != nil {
			return nil, err
		}
		activated[i] = id
	}

	return activated, nil
}

func (s *Staker) ActivateNextValidator(currentBlk uint32, maxLeaderGroupSize *big.Int) (*thor.Address, error) {
	validatorID, val, err := s.validationService.NextToActivate(maxLeaderGroupSize)
	if err != nil {
		return nil, err
	}
	logger.Debug("activating validator", "validatorID", validatorID, "block", currentBlk)

	aggRenew, err := s.aggregationService.Renew(*validatorID)
	if err != nil {
		return nil, err
	}

	// update the validator values
	// TODO move this to the validatorservice at some point
	validatorLocked := big.NewInt(0).Add(val.LockedVET, val.QueuedVET)
	val.QueuedVET = big.NewInt(0)
	val.LockedVET = validatorLocked
	// x2 multiplier for validator's stake
	validatorWeight := validation.WeightedStake(validatorLocked).Weight()
	val.Weight = big.NewInt(0).Add(validatorWeight, aggRenew.NewLockedWeight)

	// update the validator statuses
	val.Status = validation.StatusActive
	val.Online = true
	val.StartBlock = currentBlk
	// add to the active list
	added, err := s.validationService.AddLeaderGroup(*validatorID, val)
	if err != nil {
		return nil, err
	}
	if !added {
		return nil, errors.New("failed to add validator to active list")
	}

	validatorRenewal := &delta.Renewal{
		NewLockedVET:         val.LockedVET,
		NewLockedWeight:      val.Weight,
		QueuedDecrease:       val.LockedVET,
		QueuedDecreaseWeight: validatorWeight,
	}
	if err = s.globalStatsService.ApplyRenewal(validatorRenewal.Add(aggRenew)); err != nil {
		return nil, err
	}

	return validatorID, nil
}
