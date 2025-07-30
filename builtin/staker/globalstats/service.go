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
func (s *Service) AddQueued(stake *big.Int, multiplier uint8) error {
	if err := s.queuedVET.Add(stake); err != nil {
		return err
	}
	weight := stakes.CalculateWeight(stake, multiplier)
	if err := s.queuedWeight.Add(weight); err != nil {
		return err
	}

	return nil
}

// RemoveQueued decreases queued totals when stake is removed from the queue.
func (s *Service) RemoveQueued(amount *big.Int, multiplier uint8) error {
	if err := s.queuedVET.Sub(amount); err != nil {
		return err
	}
	weight := stakes.CalculateWeight(amount, multiplier)
	return s.queuedWeight.Sub(weight)
}

// GetQueuedStake returns the total VET and weight waiting to be activated.
func (s *Service) GetQueuedStake() (*big.Int, *big.Int, error) {
	queuedVet, err := s.queuedVET.Get()
	if err != nil {
		return nil, nil, err
	}

	queuedWeight, err := s.queuedWeight.Get()
	return queuedVet, queuedWeight, err
}
