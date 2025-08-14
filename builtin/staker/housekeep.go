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

//
// State transition types
//

type EpochTransition struct {
	Block            uint32
	Renewals         []ValidatorRenewal
	ExitValidator    *thor.Address
	ActivationCount  int64
	ActiveValidators map[thor.Address]*validation.Validation
}

type ValidatorRenewal struct {
	Validator       thor.Address
	NewState        *validation.Validation
	ValidatorDelta  *delta.Renewal
	DelegationDelta *delta.Renewal
}

// Housekeep performs epoch transitions at epoch boundaries
func (s *Staker) Housekeep(currentBlock uint32) (bool, map[thor.Address]*validation.Validation, error) {
	if currentBlock%EpochLength.Get() != 0 {
		return false, nil, nil
	}

	logger.Info("🏠performing housekeeping", "block", currentBlock)

	transition, err := s.computeEpochTransition(currentBlock)
	if err != nil {
		return false, nil, err
	}

	if transition == nil || (len(transition.Renewals) == 0 && transition.ExitValidator == nil && transition.ActivationCount == 0) {
		return false, nil, nil
	}

	if err := s.applyEpochTransition(transition); err != nil {
		return false, nil, err
	}

	// Build active validators map
	activeValidators := transition.ActiveValidators

	logger.Info("performed housekeeping", "block", currentBlock, "updates", true)
	return true, activeValidators, nil
}

// computeEpochTransition calculates all state changes needed for an epoch transition
func (s *Staker) computeEpochTransition(currentBlock uint32) (*EpochTransition, error) {
	var err error

	transition := &EpochTransition{Block: currentBlock}

	var renewals []ValidatorRenewal
	active := make(map[thor.Address]*validation.Validation)
	exitValidator := thor.Address{}
	err = s.validationService.LeaderGroupIterator(
		s.renewalCallback(currentBlock, &renewals),
		s.exitsCallback(currentBlock, &exitValidator),
		s.collectActiveCallback(active),
		s.evictionCallback(currentBlock),
	)
	if err != nil {
		return nil, err
	}

	transition.Renewals = renewals

	if !exitValidator.IsZero() {
		transition.ExitValidator = &exitValidator
	}

	// 3. Compute all activations
	transition.ActivationCount, err = s.computeActivationCount(transition.ExitValidator != nil)
	if err != nil {
		return nil, err
	}
	transition.ActiveValidators = active

	return transition, nil
}

func (s *Staker) renewalCallback(currentBlock uint32, renewals *[]ValidatorRenewal) func(thor.Address, *validation.Validation) error {
	// Collect all validators due for renewal
	return func(validator thor.Address, entry *validation.Validation) error {
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
		delegationsRenewal, err := s.aggregationService.Renew(validator)
		if err != nil {
			return err
		}

		// Calculate new weight and locked VET
		changeWeight := big.NewInt(0).Add(validatorRenewal.NewLockedWeight, delegationsRenewal.NewLockedWeight)
		entry.LockedVET = big.NewInt(0).Add(entry.LockedVET, validatorRenewal.NewLockedVET)
		entry.Weight = big.NewInt(0).Add(entry.Weight, changeWeight)

		*renewals = append(*renewals, ValidatorRenewal{
			Validator:       validator,
			NewState:        entry,
			ValidatorDelta:  validatorRenewal,
			DelegationDelta: delegationsRenewal,
		})

		return nil
	}
}

func (s *Staker) exitsCallback(currentBlock uint32, exitAddress *thor.Address) func(thor.Address, *validation.Validation) error {
	// Find the last validator in iteration order that should exit this block
	// Do NOT call ExitValidator here - just identify which validator should exit
	return func(validator thor.Address, entry *validation.Validation) error {
		if entry.ExitBlock != nil && currentBlock == *entry.ExitBlock {
			// should never be possible for two validators to exit at the same block
			if !exitAddress.IsZero() {
				return errors.Errorf("found more than one validator exit in the same block: ValidatorID: %s, ValidatorID: %s", exitAddress, validator)
			}
			// Just record which validator should exit (matches original behavior)
			*exitAddress = validator
		}
		return nil
	}
}

func (s *Staker) evictionCallback(currentBlock uint32) func(thor.Address, *validation.Validation) error {
	return func(validator thor.Address, entry *validation.Validation) error {
		if entry.OfflineBlock != nil && currentBlock > *entry.OfflineBlock+(thor.OfflineValidatorEvictionThresholdEpochs*EpochLength.Get()) {
			exitBlock, err := s.validationService.SetExitBlock(validator, currentBlock+EpochLength.Get())
			if err != nil {
				return err
			}
			if entry.ExitBlock != nil && *entry.ExitBlock < exitBlock {
				exitBlock = *entry.ExitBlock
			}
			entry.ExitBlock = &exitBlock

			return s.validationService.SetValidation(validator, entry, false)
		}
		return nil
	}
}

func (s *Staker) collectActiveCallback(active map[thor.Address]*validation.Validation) func(thor.Address, *validation.Validation) error {
	return func(id thor.Address, v *validation.Validation) error { active[id] = v; return nil }
}

// computeActivationCount calculates how many validators can be activated
func (s *Staker) computeActivationCount(hasValidatorExited bool) (int64, error) {
	// Calculate how many validators can be activated
	queuedSize, err := s.QueuedGroupSize()
	if err != nil {
		return 0, err
	}
	leaderSize, err := s.LeaderGroupSize()
	if err != nil {
		return 0, err
	}
	// the current leaderSize might have changed for the next activations
	if hasValidatorExited {
		leaderSize = big.NewInt(0).Sub(leaderSize, big.NewInt(1))
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

// applyEpochTransition applies all computed changes
func (s *Staker) applyEpochTransition(transition *EpochTransition) error {
	logger.Info("applying epoch transition", "block", transition.Block)

	accumulatedRenewal := delta.NewRenewal()
	// Apply renewals
	for _, renewal := range transition.Renewals {
		accumulatedRenewal.Add(renewal.ValidatorDelta)
		accumulatedRenewal.Add(renewal.DelegationDelta)

		// Update validator state
		if err := s.validationService.SetValidation(renewal.Validator, renewal.NewState, false); err != nil {
			return err
		}
	}
	// Apply accumulated renewals to global stats
	if err := s.globalStatsService.ApplyRenewal(accumulatedRenewal); err != nil {
		return err
	}

	// Apply exits
	if transition.ExitValidator != nil {
		logger.Info("exiting validator", "validator", transition.ExitValidator)

		// Now call ExitValidator to get the actual exit details and perform the exit
		exit, err := s.validationService.ExitValidator(*transition.ExitValidator)
		if err != nil {
			return err
		}

		aggExit, err := s.aggregationService.Exit(*transition.ExitValidator)
		if err != nil {
			return err
		}

		if err := s.globalStatsService.ApplyExit(exit.Add(aggExit)); err != nil {
			return err
		}
		delete(transition.ActiveValidators, *transition.ExitValidator)
	}

	// Apply activations using existing method
	maxLeaderGroupSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return err
	}

	for range transition.ActivationCount {
		address, err := s.activateNextValidation(transition.Block, maxLeaderGroupSize)
		if err != nil {
			return err
		}
		validator, err := s.Get(*address)
		if err != nil {
			return err
		}
		transition.ActiveValidators[*address] = validator
	}

	return nil
}

func (s *Staker) activateNextValidation(currentBlk uint32, maxLeaderGroupSize *big.Int) (*thor.Address, error) {
	validatorID, err := s.validationService.NextToActivate(maxLeaderGroupSize)
	if err != nil {
		return nil, err
	}
	logger.Debug("activating validator", "validatorID", validatorID, "block", currentBlk)

	// renew the current delegations aggregation
	aggRenew, err := s.aggregationService.Renew(*validatorID)
	if err != nil {
		return nil, err
	}

	// Activate the validator using the validation service
	validatorRenewal, err := s.validationService.ActivateValidator(*validatorID, currentBlk, aggRenew)
	if err != nil {
		return nil, err
	}

	// Update global stats with both validator and delegation renewals
	if err = s.globalStatsService.ApplyRenewal(validatorRenewal.Add(aggRenew)); err != nil {
		return nil, err
	}

	return validatorID, nil
}
