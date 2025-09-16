// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotLocked       = thor.BytesToBytes32([]byte(("total-weighted-stake")))
	slotQueued       = thor.BytesToBytes32([]byte(("queued-stake")))
	slotWithdrawable = thor.BytesToBytes32([]byte(("withdrawable-stake")))
)

// Service manages contract-wide staking totals.
// Tracks both locked stake (from active validators/delegations) and queued stake (pending activation).
type Service struct {
	locked       *solidity.Raw[*stakes.WeightedStake]
	queued       *solidity.Raw[uint64]
	withdrawable *solidity.Raw[uint64]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		locked:       solidity.NewRaw[*stakes.WeightedStake](sctx, slotLocked),
		queued:       solidity.NewRaw[uint64](sctx, slotQueued),
		withdrawable: solidity.NewRaw[uint64](sctx, slotWithdrawable),
	}
}

func (s *Service) getLocked() (*stakes.WeightedStake, error) {
	locked, err := s.locked.Get()
	if err != nil {
		return nil, err
	}
	if locked == nil {
		locked = &stakes.WeightedStake{}
	}
	return locked, nil
}

// ApplyRenewal adjusts global totals during validator/delegation transitions.
// Called when validators are activated or delegations move between states.
func (s *Service) ApplyRenewal(renewal *Renewal) error {
	locked, err := s.getLocked()
	if err != nil {
		return err
	}
	queued, err := s.queued.Get()
	if err != nil {
		return err
	}

	locked.Add(renewal.LockedIncrease)
	if err := locked.Sub(renewal.LockedDecrease); err != nil {
		return err
	}
	queued -= renewal.QueuedDecrease

	// for the initial state, use upsert to handle correct gas cost
	if err := s.locked.Upsert(locked); err != nil {
		return err
	}

	// queued here is already touched by addQueued
	if err := s.queued.Update(queued); err != nil {
		return err
	}
	if err := s.AddWithdrawable(renewal.LockedDecrease.VET); err != nil {
		return err
	}

	return nil
}

func (s *Service) ApplyExit(exit *Exit) error {
	locked, err := s.getLocked()
	if err != nil {
		return err
	}

	queued, err := s.queued.Get()
	if err != nil {
		return err
	}

	if err := locked.Sub(exit.ExitedTVL); err != nil {
		return err
	}
	queued -= exit.QueuedDecrease

	if err := s.locked.Update(locked); err != nil {
		return err
	}

	if err := s.queued.Update(queued); err != nil {
		return err
	}
	if err := s.AddWithdrawable(exit.ExitedTVL.VET); err != nil {
		return err
	}

	return nil
}

// AddQueued increases queued totals when new stake is added to the queue.
func (s *Service) AddQueued(stake uint64) error {
	queued, err := s.queued.Get()
	if err != nil {
		return err
	}

	queued += stake
	// for the initial state, use upsert to handle correct gas cost
	return s.queued.Upsert(queued)
}

// RemoveQueued decreases queued totals when stake is removed from the queue.
func (s *Service) RemoveQueued(stake uint64) error {
	queued, err := s.queued.Get()
	if err != nil {
		return err
	}

	if queued < stake {
		return errors.New("stake cannot be grater than queued")
	}
	queued -= stake
	// queued here is already touched by addQueued
	return s.queued.Update(queued)
}

// GetLockedStake returns the total VET and weight currently locked in active staking.
func (s *Service) GetLockedStake() (uint64, uint64, error) {
	locked, err := s.getLocked()
	if err != nil {
		return 0, 0, err
	}

	return locked.VET, locked.Weight, nil
}

// GetQueuedStake returns the total VET and weight waiting to be activated.
func (s *Service) GetQueuedStake() (uint64, error) {
	return s.queued.Get()
}

// AddWithdravable increases withdrawable totals when stake becomes withdrwable
func (s *Service) AddWithdrawable(stake uint64) error {
	withdrawable, err := s.withdrawable.Get()
	if err != nil {
		return err
	}

	withdrawable += stake
	// for the initial state, use upsert to handle correct gas cost
	return s.withdrawable.Upsert(withdrawable)
}

// RemoveWithdrawable decreases withdrawable totals when stake is withdrawn.
func (s *Service) RemoveWithdrawable(stake uint64) error {
	withdrawable, err := s.withdrawable.Get()
	if err != nil {
		return err
	}

	if withdrawable < stake {
		println("with", withdrawable, stake)
		return errors.New("stake cannot be grater than withdrawable")
	}
	withdrawable -= stake
	// witdrawable here is already touched by addWithdrawable
	return s.withdrawable.Update(withdrawable)
}

// GetWithdrawableStake returns the total VET to be withdrawn.
func (s *Service) GetWithdrawableStake() (uint64, error) {
	return s.withdrawable.Get()
}
