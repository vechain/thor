// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/thor"
)

// transition activates the staker contract when sufficient validators are queued
func (s *Staker) transition(currentBlock uint32) (bool, error) {
	if currentBlock%thor.EpochLength() != 0 {
		return false, nil // No transition needed
	}

	active, err := s.IsPoSActive()
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	mbp, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return false, err
	}

	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}

	queueSize, err := s.validationService.QueuedGroupSize()
	if err != nil {
		return false, err
	}

	// Transitions is not possible if the queue size is not AT LEAST 2/3 of the maxProposers
	// queueSize >= 2/3 * maxProposers
	// queueSize * 3 >= maxProposers * 2
	if queueSize*3 < maxBlockProposers*2 { // these figures will never surpass int64
		return false, nil
	}

	// Use the epoch transition pattern as housekeeping
	transition, err := s.computeEpochTransition(currentBlock)
	if err != nil {
		return false, err
	}

	// Apply the transition
	if err := s.applyEpochTransition(transition); err != nil {
		return false, err
	}

	logger.Info("activated validations", "count", transition.ActivationCount)

	return true, nil
}

var bigE18 = big.NewInt(1e18)

// TransitionPeriodBalanceCheck returns a BalanceChecker function that checks if an endorser has enough VET soft staked
// for the transition period whereby the endorser can leverage queued VET to meet the requirement.
// It defaults to checking the account balance first and then checks the queued VET if in transition period.
func (s *Staker) TransitionPeriodBalanceCheck(fc *thor.ForkConfig, currentBlock uint32, endorsement *big.Int) authority.BalanceChecker {
	return func(validator, endorser thor.Address) (bool, error) {
		balance, err := s.state.GetBalance(endorser)
		if err != nil {
			return false, err
		}
		if balance.Cmp(endorsement) >= 0 {
			return true, nil
		}
		if currentBlock < fc.HAYABUSA { // before HAYABUSA fork, we only check the account balance
			return false, nil
		}
		validation, err := s.validationService.GetValidation(validator)
		if err != nil {
			return false, err
		}
		if validation.IsEmpty() {
			return false, nil
		}
		if validation.Endorser != endorser {
			return false, nil // endorser mismatch
		}
		queuedVET := big.NewInt(0).SetUint64(validation.QueuedVET)
		queuedVET.Mul(queuedVET, bigE18) // convert to wei

		return queuedVET.Cmp(endorsement) >= 0, nil
	}
}
