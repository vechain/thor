// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// Transition activates the staker contract when sufficient validators are queued
func (s *Staker) Transition(currentBlock uint32) (bool, error) {
	// TODO review how to change this elegantly for unit tests
	// if this check is enabled the epochLength is defaulted to 180 blocks
	// which breaks most of tests that rely on a HAYABUSA_TP = 1
	//
	//if currentBlock%epochLength != 0 {
	//	return false, nil // No transition needed
	//}

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

	// Transitions is not possible if the queue size is not AT LEAST 2/3 of the maxProposers
	minimum := big.NewFloat(0).SetInt(maxProposers)
	minimum.Mul(minimum, big.NewFloat(2))
	minimum.Quo(minimum, big.NewFloat(3))
	if big.NewFloat(0).SetInt(queueSize).Cmp(minimum) < 0 {
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
