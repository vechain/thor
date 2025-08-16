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
	if currentBlock%thor.EpochLength != 0 {
		return false, nil // No transition needed
	}

	active, err := s.IsPoSActive()
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	maxProposers, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return false, err
	}

	if maxProposers.Cmp(big.NewInt(0)) == 0 {
		maxProposers = big.NewInt(0).SetUint64(thor.InitialMaxBlockProposers)
	}

	queueSize, err := s.validationService.QueuedGroupSize()
	if err != nil {
		return false, err
	}

	// Transitions is not possible if the queue size is not AT LEAST 2/3 of the maxProposers
	// queueSize >= 2/3 * maxProposers
	// queueSize * 3 >= maxProposers * 2
	if queueSize.Int64()*3 < maxProposers.Int64()*2 { // these figures will never surpass int64
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
