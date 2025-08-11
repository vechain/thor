// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
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
		lockedVET:    solidity.NewUint256(sctx, solidity.NumToSlot(1)),
		lockedWeight: solidity.NewUint256(sctx, solidity.NumToSlot(2)),
		queuedVET:    solidity.NewUint256(sctx, solidity.NumToSlot(3)),
		queuedWeight: solidity.NewUint256(sctx, solidity.NumToSlot(4)),
	}
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
