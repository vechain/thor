// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"errors"

	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotLocked       = thor.BytesToBytes32([]byte(("total-weighted-stake")))
	slotQueued       = thor.BytesToBytes32([]byte(("queued-stake")))
	slotWithdrawable = thor.BytesToBytes32([]byte(("withdrawable-stake")))
	slotCooldown     = thor.BytesToBytes32([]byte(("cooldown-stake")))
)

// Service manages contract-wide staking totals.
// Tracks both locked stake (from active validators/delegations) and queued stake (pending activation).
type Service struct {
	locked       *solidity.Raw[*stakes.WeightedStake]
	queued       *solidity.Raw[uint64]
	withdrawable *solidity.Raw[uint64]
	cooldown     *solidity.Raw[uint64]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		locked:       solidity.NewRaw[*stakes.WeightedStake](sctx, slotLocked),
		queued:       solidity.NewRaw[uint64](sctx, slotQueued),
		withdrawable: solidity.NewRaw[uint64](sctx, slotWithdrawable),
		cooldown:     solidity.NewRaw[uint64](sctx, slotCooldown),
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

	if err := locked.Add(renewal.LockedIncrease); err != nil {
		return err
	}

	if err := locked.Sub(renewal.LockedDecrease); err != nil {
		return err
	}

	// for the initial state, use upsert to handle correct gas cost
	if err := s.locked.Upsert(locked); err != nil {
		return err
	}

	if err := s.RemoveQueued(renewal.QueuedDecrease); err != nil {
		return err
	}

	// move locked decrease to withdrawable, includes validation's pending unlock and delegation's exiting
	if err := s.AddWithdrawable(renewal.LockedDecrease.VET); err != nil {
		return err
	}

	return nil
}

func (s *Service) ApplyExit(validation *Exit, aggregation *Exit) error {
	// Track the unlock of validation + aggregations locked stakes
	totalExited := validation.ExitedTVL.Clone()
	if err := totalExited.Add(aggregation.ExitedTVL); err != nil {
		return err
	}

	if totalExited.VET > 0 {
		if err := s.RemoveLocked(totalExited); err != nil {
			return err
		}
	}

	// All queued values are now removed from the queued tracker
	queuedDecrease := validation.QueuedDecrease + aggregation.QueuedDecrease
	if queuedDecrease > 0 {
		if err := s.RemoveQueued(queuedDecrease); err != nil {
			return err
		}
	}

	// Unlocked validation stake is now on cooldown
	if validation.ExitedTVL.VET > 0 {
		if err := s.AddCooldown(validation.ExitedTVL.VET); err != nil {
			return err
		}
	}

	// Both queued (val + del) stakes and exited delegations are now withdrawable
	// exited delegations do not go on cooldown
	withdrawableIncrease := queuedDecrease + aggregation.ExitedTVL.VET
	if withdrawableIncrease > 0 {
		if err := s.AddWithdrawable(withdrawableIncrease); err != nil {
			return err
		}
	}

	return nil
}

// RemoveLocked decreases locked totals when stake is removed from the locked.
func (s *Service) RemoveLocked(weightedStake *stakes.WeightedStake) error {
	locked, err := s.getLocked()
	if err != nil {
		return err
	}

	if err := locked.Sub(weightedStake); err != nil {
		return err
	}

	// locked here is already touched by addLocked
	return s.locked.Update(locked)
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

	queued, underflow := math.SafeSub(queued, stake)
	if underflow {
		return errors.New("queued underflow occurred")
	}
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

// AddWithdrawable increases withdrawable totals when stake becomes withdrwable
func (s *Service) AddWithdrawable(stake uint64) error {
	withdrawable, err := s.withdrawable.Get()
	if err != nil {
		return err
	}

	withdrawable, overflow := math.SafeAdd(withdrawable, stake)
	if overflow {
		return errors.New("withdrawable overflow occurred")
	}
	// for the initial state, use upsert to handle correct gas cost
	return s.withdrawable.Upsert(withdrawable)
}

// RemoveWithdrawable decreases withdrawable totals when stake is withdrawn.
func (s *Service) RemoveWithdrawable(stake uint64) error {
	withdrawable, err := s.withdrawable.Get()
	if err != nil {
		return err
	}

	withdrawable, underflow := math.SafeSub(withdrawable, stake)
	if underflow {
		return errors.New("withdrawable underflow occurred")
	}
	// withdrawable here is already touched by addWithdrawable
	return s.withdrawable.Update(withdrawable)
}

// GetWithdrawableStake returns the total VET to be withdrawn.
func (s *Service) GetWithdrawableStake() (uint64, error) {
	return s.withdrawable.Get()
}

// AddCooldown increases cooldown totals when stake goes to cooldown
func (s *Service) AddCooldown(stake uint64) error {
	cooldown, err := s.cooldown.Get()
	if err != nil {
		return err
	}

	cooldown, overflow := math.SafeAdd(cooldown, stake)
	if overflow {
		return errors.New("cooldown overflow occurred")
	}
	// for the initial state, use upsert to handle correct gas cost
	return s.cooldown.Upsert(cooldown)
}

// RemoveCooldown decreases cooldown totals when stake goes to withdrawable.
func (s *Service) RemoveCooldown(stake uint64) error {
	cooldown, err := s.cooldown.Get()
	if err != nil {
		return err
	}

	cooldown, underflow := math.SafeSub(cooldown, stake)
	if underflow {
		return errors.New("cooldown underflow occurred")
	}
	// cooldown here is already touched by addCooldown
	return s.cooldown.Update(cooldown)
}

// GetCooldownStake returns the total VET in cooldown.
func (s *Service) GetCooldownStake() (uint64, error) {
	return s.cooldown.Get()
}
