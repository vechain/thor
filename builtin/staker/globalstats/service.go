// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotLockedVET    = thor.BytesToBytes32([]byte(("total-stake")))
	slotLockedWeight = thor.BytesToBytes32([]byte(("total-weight")))
	slotQueuedVET    = thor.BytesToBytes32([]byte(("queued-stake")))
	slotQueuedWeight = thor.BytesToBytes32([]byte(("queued-weight")))
)

// Service manages contract-wide staking totals.
// Tracks both locked stake (from active validators/delegations) and queued stake (pending activation).
type Service struct {
	lockedVET    *solidity.Uint256
	lockedWeight *solidity.Uint256

	queuedVET    *solidity.Uint256
	queuedWeight *solidity.Uint256
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		lockedVET:    solidity.NewUint256(sctx, slotLockedVET),
		lockedWeight: solidity.NewUint256(sctx, slotLockedWeight),
		queuedVET:    solidity.NewUint256(sctx, slotQueuedVET),
		queuedWeight: solidity.NewUint256(sctx, slotQueuedWeight),
	}
}

// QueuedStake returns the total VET and weight waiting to be activated.
func (s *Service) QueuedStake() (*big.Int, *big.Int, error) {
	queuedVet, err := s.queuedVET.Get()
	if err != nil {
		return nil, nil, err
	}

	queuedWeight, err := s.queuedWeight.Get()
	return queuedVet, queuedWeight, err
}

// ApplyRenewal adjusts global totals during validator/delegation transitions.
// Called when validators are activated or delegations move between states.
func (s *Service) ApplyRenewal(renewal *delta.Renewal) error {
	if renewal.NewLockedVET == nil {
		renewal.NewLockedVET = new(big.Int)
	}
	if renewal.NewLockedWeight == nil {
		renewal.NewLockedWeight = new(big.Int)
	}
	if renewal.QueuedDecreaseWeight == nil {
		renewal.QueuedDecreaseWeight = new(big.Int)
	}
	if renewal.QueuedDecrease == nil {
		renewal.QueuedDecrease = new(big.Int)
	}

	if err := s.lockedVET.Add(renewal.NewLockedVET); err != nil {
		return err
	}
	if err := s.lockedWeight.Add(renewal.NewLockedWeight); err != nil {
		return err
	}
	if err := s.queuedVET.Sub(renewal.QueuedDecrease); err != nil {
		return err
	}
	if err := s.queuedWeight.Sub(renewal.QueuedDecreaseWeight); err != nil {
		return err
	}

	return nil
}

func (s *Service) ApplyExit(exit *delta.Exit) error {
	if exit.ExitedTVL == nil {
		exit.ExitedTVL = new(big.Int)
	}
	if exit.ExitedTVLWeight == nil {
		exit.ExitedTVLWeight = new(big.Int)
	}
	if exit.QueuedDecreaseWeight == nil {
		exit.QueuedDecreaseWeight = new(big.Int)
	}
	if exit.QueuedDecrease == nil {
		exit.QueuedDecrease = new(big.Int)
	}

	if err := s.lockedVET.Sub(exit.ExitedTVL); err != nil {
		return err
	}
	if err := s.lockedWeight.Sub(exit.ExitedTVLWeight); err != nil {
		return err
	}
	if err := s.queuedVET.Sub(exit.QueuedDecrease); err != nil {
		return err
	}
	if err := s.queuedWeight.Sub(exit.QueuedDecreaseWeight); err != nil {
		return err
	}

	return nil
}

// GetLockedVET returns the total VET and weight currently locked in active staking.
func (s *Service) GetLockedVET() (*big.Int, *big.Int, error) {
	lockedVet, err := s.lockedVET.Get()
	if err != nil {
		return nil, nil, err
	}

	lockedWeight, err := s.lockedWeight.Get()
	return lockedVet, lockedWeight, err
}

// AddQueued increases queued totals when new stake is added to the queue.
func (s *Service) AddQueued(stake *stakes.WeightedStake) error {
	if err := s.queuedVET.Add(stake.VET()); err != nil {
		return err
	}
	if err := s.queuedWeight.Add(stake.Weight()); err != nil {
		return err
	}

	return nil
}

// RemoveQueued decreases queued totals when stake is removed from the queue.
func (s *Service) RemoveQueued(stake *stakes.WeightedStake) error {
	if err := s.queuedVET.Sub(stake.VET()); err != nil {
		return err
	}
	return s.queuedWeight.Sub(stake.Weight())
}

// GetQueuedStake returns the total VET waiting to be activated.
func (s *Service) GetQueuedStake() (*big.Int, error) {
	return s.queuedVET.Get()
}
