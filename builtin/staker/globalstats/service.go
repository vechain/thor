// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotLocked = thor.BytesToBytes32([]byte(("total-weighted-stake")))
	slotQueued = thor.BytesToBytes32([]byte(("queued-weighted-stake")))
)

// Service manages contract-wide staking totals.
// Tracks both locked stake (from active validators/delegations) and queued stake (pending activation).
type Service struct {
	locked *solidity.Raw[*stakes.WeightedStake]
	queued *solidity.Raw[*stakes.WeightedStake]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		locked: solidity.NewRaw[*stakes.WeightedStake](sctx, slotLocked),
		queued: solidity.NewRaw[*stakes.WeightedStake](sctx, slotQueued),
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

func (s *Service) getQueued() (*stakes.WeightedStake, error) {
	queued, err := s.queued.Get()
	if err != nil {
		return nil, err
	}
	if queued == nil {
		queued = &stakes.WeightedStake{}
	}
	return queued, nil
}

// ApplyRenewal adjusts global totals during validator/delegation transitions.
// Called when validators are activated or delegations move between states.
func (s *Service) ApplyRenewal(renewal *delta.Renewal) error {
	locked, err := s.getLocked()
	if err != nil {
		return err
	}
	queued, err := s.getQueued()
	if err != nil {
		return err
	}

	locked.Add(renewal.LockedIncrease)
	locked.Sub(renewal.LockedDecrease)
	queued.Sub(renewal.QueuedDecrease)

	if err := s.locked.Set(locked); err != nil {
		return err
	}

	if err := s.queued.Set(queued); err != nil {
		return err
	}

	return nil
}

func (s *Service) ApplyExit(exit *delta.Exit) error {
	locked, err := s.getLocked()
	if err != nil {
		return err
	}

	queued, err := s.getQueued()
	if err != nil {
		return err
	}

	locked.Sub(exit.ExitedTVL)
	queued.Sub(exit.QueuedDecrease)

	if err := s.locked.Set(locked); err != nil {
		return err
	}

	if err := s.queued.Set(queued); err != nil {
		return err
	}

	return nil
}

// AddQueued increases queued totals when new stake is added to the queue.
func (s *Service) AddQueued(stake *stakes.WeightedStake) error {
	queued, err := s.getQueued()
	if err != nil {
		return err
	}

	queued.Add(stake)

	return s.queued.Set(queued)
}

// RemoveQueued decreases queued totals when stake is removed from the queue.
func (s *Service) RemoveQueued(stake *stakes.WeightedStake) error {
	queued, err := s.getQueued()
	if err != nil {
		return err
	}

	queued.Sub(stake)

	return s.queued.Set(queued)
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
	return s.getQueued()
}
