// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/math"

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
	ExitValidator   thor.Address
	Evictions       []thor.Address
	ActivationCount uint64
}

func (et *EpochTransition) HasUpdates() bool {
	return len(et.Renewals) > 0 || // renewing existing staking periods
		!et.ExitValidator.IsZero() || // exiting 1 validator
		len(et.Evictions) > 0 || // forcing eviction of offline validators
		et.ActivationCount > 0 // activating new validators
}

// Housekeep performs epoch transitions at epoch boundaries
func (s *Staker) Housekeep(currentBlock uint32) (bool, error) {
	if currentBlock%thor.EpochLength() != 0 {
		return false, nil
	}

	transition, err := s.computeEpochTransition(currentBlock)
	if err != nil {
		return false, err
	}

	logger.Info("🏠housekeeping", "block", currentBlock, "updates", transition.HasUpdates())

	if !transition.HasUpdates() {
		return false, nil
	}

	if err := s.applyEpochTransition(transition); err != nil {
		return false, err
	}

	if err := s.ContractBalanceCheck(0); err != nil {
		return false, err
	}

	return true, nil
}

// computeEpochTransition calculates all state changes needed for an epoch transition
func (s *Staker) computeEpochTransition(currentBlock uint32) (*EpochTransition, error) {
	var evictions []thor.Address
	if currentBlock != 0 && currentBlock%thor.EvictionCheckInterval() == 0 {
		if err := s.validationService.LeaderGroupIterator(
			s.evictionCallback(currentBlock, &evictions),
		); err != nil {
			return nil, err
		}
	}

	renewals, err := s.validationService.UpdateGroup(currentBlock)
	if err != nil {
		return nil, err
	}

	exitValidator, err := s.validationService.GetValidatorForExitBlock(currentBlock)
	if err != nil {
		return nil, err
	}

	transition := &EpochTransition{Block: currentBlock, Renewals: renewals, Evictions: evictions}

	if !exitValidator.IsZero() {
		transition.ExitValidator = exitValidator
	}

	// 3. Compute all activations
	transition.ActivationCount, err = s.computeActivationCount(!transition.ExitValidator.IsZero())
	if err != nil {
		return nil, err
	}

	return transition, nil
}

func (s *Staker) evictionCallback(currentBlock uint32, evictions *[]thor.Address) func(thor.Address, *validation.Validation) error {
	return func(validator thor.Address, entry *validation.Validation) error {
		if entry.ShouldEvict(currentBlock) {
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

	maxBlockProposers, err := thor.GetMaxBlockProposers(s.params, false)
	if err != nil {
		return 0, err
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
		// handle the renewals for aggregations
		aggRenewal, delegationWeight, err := s.aggregationService.Renew(validator)
		if err != nil {
			return err
		}
		accumulatedRenewal, err = accumulatedRenewal.Add(aggRenewal)
		if err != nil {
			return err
		}

		// Update validator state
		valRenewal, err := s.validationService.Renew(validator, delegationWeight)
		if err != nil {
			return err
		}
		accumulatedRenewal, err = accumulatedRenewal.Add(valRenewal)
		if err != nil {
			return err
		}

		if err = s.validationService.RemoveFromRenewalList(validator); err != nil {
			return err
		}
	}
	// Apply accumulated renewals to global stats
	if err := s.globalStatsService.ApplyRenewal(accumulatedRenewal); err != nil {
		return err
	}

	// Apply exits
	if !transition.ExitValidator.IsZero() {
		logger.Info("exiting validator", "validator", transition.ExitValidator)

		// Now call ExitValidator to get the actual exit details and perform the exit
		valExit, err := s.validationService.ExitValidator(transition.ExitValidator)
		if err != nil {
			return err
		}

		aggExit, err := s.aggregationService.Exit(transition.ExitValidator)
		if err != nil {
			return err
		}

		if err := s.globalStatsService.ApplyExit(valExit, aggExit); err != nil {
			return err
		}
	}

	// Apply evictions
	for _, validator := range transition.Evictions {
		logger.Info("evicting validator", "validator", validator)
		// signal exit to the eviction validator for the next epoch, since exit process is already done in this flow
		if err := s.validationService.SignalExit(validator, transition.Block, transition.Block+thor.EpochLength(), int(thor.InitialMaxBlockProposers)); err != nil {
			return err
		}
	}

	// Apply activations using existing method
	maxBlockProposers, err := thor.GetMaxBlockProposers(s.params, false)
	if err != nil {
		return err
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

	globalRenewal, err := validatorRenewal.Add(aggRenew)
	if err != nil {
		return thor.Address{}, err
	}

	// Update global stats with both validator and delegation renewals
	if err = s.globalStatsService.ApplyRenewal(globalRenewal); err != nil {
		return thor.Address{}, err
	}

	return validator, nil
}

// ContractBalanceCheck ensures that locked + queued + withdrawable VET equals the total VET in the staker account address.
// pendingWithdraw is the amount that is about to be withdrawn from the staker contract. It has not yet been deducted from the contract.
func (s *Staker) ContractBalanceCheck(pendingWithdraw uint64) error {
	// Sum all locked, queued, and withdrawable VET for all validations
	lockedStake, _, err := s.globalStatsService.GetLockedStake()
	if err != nil {
		return fmt.Errorf("error while retrieving GetLockedStake: %w", err)
	}
	queuedStake, err := s.globalStatsService.GetQueuedStake()
	if err != nil {
		return fmt.Errorf("error while retrieving GetQueuedStake: %w", err)
	}
	withdrawableStake, err := s.globalStatsService.GetWithdrawableStake()
	if err != nil {
		return fmt.Errorf("error while retrieving GetWithdrawableStake: %w", err)
	}
	cooldownStake, err := s.globalStatsService.GetCooldownStake()
	if err != nil {
		return fmt.Errorf("error while retrieving GetCooldownStake: %w", err)
	}

	counterTotal := uint64(0)
	for _, stake := range []uint64{lockedStake, queuedStake, withdrawableStake, cooldownStake, pendingWithdraw} {
		partialSum, overflow := math.SafeAdd(counterTotal, stake)
		if overflow {
			return fmt.Errorf(
				"counterTotal overflow occurred while adding locked(%d) + queued(%d) + withdrawable(%d) + cooldown(%d) + pendingWithdraw(%d)",
				lockedStake,
				queuedStake,
				withdrawableStake,
				cooldownStake,
				pendingWithdraw,
			)
		}
		counterTotal = partialSum
	}

	// Get the staker contract's account balance
	stakerAddr := s.Address()
	balance, err := s.state.GetBalance(stakerAddr)
	if err != nil {
		return err
	}
	balanceVET, err := ToVET(balance)
	if err != nil {
		return err
	}

	// Get the Effective VET tracked
	effectiveVET, err := s.GetEffectiveVET()
	if err != nil {
		return err
	}
	// The Staker will always have money to pay withdrawals
	if balanceVET < effectiveVET {
		logger.Error("balance check failed: not enough vet in account balance",
			"locked", lockedStake,
			"queued", queuedStake,
			"withdrawable", withdrawableStake,
			"cooldown", cooldownStake,
			"pendingWithdraw", pendingWithdraw,
			"total", counterTotal,
			"balance", balanceVET,
			"effectiveVET", effectiveVET,
		)
		return fmt.Errorf("balance check failed: balanceVET(%d) < effectiveVET(%d)", balanceVET, effectiveVET)
	}

	// The Effective VET is always the same as the SumCounters
	if effectiveVET != counterTotal {
		logger.Error("balance check failed: mismatched effective vet and counter total",
			"locked", lockedStake,
			"queued", queuedStake,
			"withdrawable", withdrawableStake,
			"cooldown", cooldownStake,
			"pendingWithdraw", pendingWithdraw,
			"total", counterTotal,
			"balance", balanceVET,
			"effectiveVET", effectiveVET,
		)
		return fmt.Errorf("balance check failed: effectiveVET(%d) != counterTotal(%d)", effectiveVET, counterTotal)
	}

	return nil
}
