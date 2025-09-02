// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

//
// State transition types
//

type EpochTransition struct {
	Block           uint32
	Renewals        []thor.Address
	ExitValidator   *thor.Address
	Evictions       []thor.Address
	ActivationCount uint64
}

func (et *EpochTransition) HasUpdates() bool {
	return len(et.Renewals) > 0 || // renewing existing staking periods
		(et.ExitValidator != nil && !et.ExitValidator.IsZero()) || // exiting 1 validator
		len(et.Evictions) > 0 || // forcing eviction of offline validators
		et.ActivationCount > 0 // activating new validators
}

// Housekeep performs epoch transitions at epoch boundaries
func (s *Staker) Housekeep(currentBlock uint32) (bool, error) {
	if currentBlock%thor.EpochLength() != 0 {
		return false, nil
	}

	logger.Info("🏠performing housekeeping", "block", currentBlock)

	transition, err := s.computeEpochTransition(currentBlock)
	if err != nil {
		return false, err
	}

	if !transition.HasUpdates() {
		return false, nil
	}

	if err := s.applyEpochTransition(transition); err != nil {
		return false, err
	}

	logger.Info("performed housekeeping", "block", currentBlock, "updates", true)
	return true, nil
}

// computeEpochTransition calculates all state changes needed for an epoch transition
func (s *Staker) computeEpochTransition(currentBlock uint32) (*EpochTransition, error) {
	var err error

	var renewals []thor.Address
	var evictions []thor.Address
	exitValidator := thor.Address{}
	err = s.validationService.LeaderGroupIterator(
		s.renewalCallback(currentBlock, &renewals),
		s.exitsCallback(currentBlock, &exitValidator),
		s.evictionCallback(currentBlock, &evictions),
	)
	if err != nil {
		return nil, err
	}

	transition := &EpochTransition{Block: currentBlock, Renewals: renewals, Evictions: evictions}

	if !exitValidator.IsZero() {
		transition.ExitValidator = &exitValidator
	}

	// 3. Compute all activations
	transition.ActivationCount, err = s.computeActivationCount(transition.ExitValidator != nil)
	if err != nil {
		return nil, err
	}

	return transition, nil
}

func (s *Staker) renewalCallback(currentBlock uint32, renewals *[]thor.Address) func(thor.Address, *validation.Validation) error {
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

		*renewals = append(*renewals, validator)

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

func (s *Staker) evictionCallback(currentBlock uint32, evictions *[]thor.Address) func(thor.Address, *validation.Validation) error {
	return func(validator thor.Address, entry *validation.Validation) error {
		if entry.OfflineBlock != nil && currentBlock > *entry.OfflineBlock+thor.ValidatorEvictionThreshold() && entry.ExitBlock == nil {
			*evictions = append(*evictions, validator)
			return nil
		}
		return nil
	}
}

// computeActivationCount calculates how many validators can be activated
func (s *Staker) computeActivationCount(hasValidatorExited bool) (uint64, error) {
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
		leaderSize = leaderSize - 1
	}

	mbp, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return 0, err
	}
	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}

	// If full or nothing queued then no activations
	if leaderSize >= maxBlockProposers || queuedSize <= 0 {
		return 0, nil
	}

	leaderDelta := maxBlockProposers - leaderSize
	if leaderDelta <= 0 {
		return 0, nil
	}

	queuedCount := queuedSize
	if leaderDelta < queuedCount {
		return leaderDelta, nil
	}
	return queuedCount, nil
}

// applyEpochTransition applies all computed changes
func (s *Staker) applyEpochTransition(transition *EpochTransition) error {
	logger.Info("applying epoch transition", "block", transition.Block)

	accumulatedRenewal := globalstats.NewRenewal()
	// Apply renewals
	for _, validator := range transition.Renewals {
		aggRenewal, delegationWeight, err := s.aggregationService.Renew(validator)
		if err != nil {
			return err
		}
		accumulatedRenewal.Add(aggRenewal)
		// Update validator state
		valRenewal, err := s.validationService.Renew(validator, delegationWeight)
		if err != nil {
			return err
		}
		accumulatedRenewal.Add(valRenewal)
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
	}

	// Apply evictions
	for _, validator := range transition.Evictions {
		logger.Info("evicting validator", "validator", validator)
		if err := s.validationService.Evict(validator, transition.Block); err != nil {
			return err
		}
	}

	// Apply activations using existing method
	mbp, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return err
	}
	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}

	for range transition.ActivationCount {
		_, err := s.activateNextValidation(transition.Block, maxBlockProposers)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Staker) activateNextValidation(currentBlk uint32, maxLeaderGroupSize uint64) (thor.Address, error) {
	validator, validation, err := s.validationService.NextToActivate(maxLeaderGroupSize)
	if err != nil {
		return thor.Address{}, err
	}
	logger.Debug("activating validator", "validator", validator, "block", currentBlk)

	// renew the current delegations aggregation
	aggRenew, _, err := s.aggregationService.Renew(validator)
	if err != nil {
		return thor.Address{}, err
	}

	// Activate the validator using the validation service
	validatorRenewal, err := s.validationService.ActivateValidator(validator, validation, currentBlk, aggRenew)
	if err != nil {
		return thor.Address{}, err
	}

	// Update global stats with both validator and delegation renewals
	if err = s.globalStatsService.ApplyRenewal(validatorRenewal.Add(aggRenew)); err != nil {
		return thor.Address{}, err
	}

	return validator, nil
}
