// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package globalstats

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
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

// UpdateTotals adjusts global totals during validator/delegation transitions.
// Called when validators are activated or delegations move between states.
func (s *Service) UpdateTotals(validatorRenewal *delta.Renewal, delegatorRenewal *delta.Renewal) error {
	// calculate the new totals for validator + delegations
	changeTVL := big.NewInt(0).Add(validatorRenewal.ChangeTVL, delegatorRenewal.ChangeTVL)
	changeWeight := big.NewInt(0).Add(validatorRenewal.ChangeWeight, delegatorRenewal.ChangeWeight)
	queuedDecrease := big.NewInt(0).Add(validatorRenewal.QueuedDecrease, delegatorRenewal.QueuedDecrease)
	queuedWeight := big.NewInt(0).Add(validatorRenewal.QueuedDecreaseWeight, delegatorRenewal.QueuedDecreaseWeight)

	if err := s.lockedVET.Add(changeTVL); err != nil {
		return err
	}
	if err := s.lockedWeight.Add(changeWeight); err != nil {
		return err
	}
	if err := s.queuedVET.Sub(queuedDecrease); err != nil {
		return err
	}
	if err := s.queuedWeight.Sub(queuedWeight); err != nil {
		return err
	}

	return nil
}

// GetLocketVET returns the total VET and weight currently locked in active staking.
func (s *Service) GetLocketVET() (*big.Int, *big.Int, error) {
	lockedVet, err := s.lockedVET.Get()
	if err != nil {
		return nil, nil, err
	}

	lockedWeight, err := s.lockedWeight.Get()
	return lockedVet, lockedWeight, err
}

// AddQueued increases queued totals when new stake is added to the queue.
func (s *Service) AddQueued(stake *big.Int, weight *big.Int) error {
	if err := s.queuedVET.Add(stake); err != nil {
		return err
	}

	if err := s.queuedWeight.Add(weight); err != nil {
		return err
	}

	return nil
}

// RemoveQueued decreases queued totals when stake is removed from the queue.
func (s *Service) RemoveQueued(amount *big.Int, weight *big.Int) error {
	if err := s.queuedVET.Sub(amount); err != nil {
		return err
	}
	return s.queuedWeight.Sub(weight)
}

// RemoveLocked decreases locked totals when validators exit the active set.
// Also removes any pending delegations that were queued for the exiting validator.
func (s *Service) RemoveLocked(unlockedTVL *big.Int, unlockedTVLWeight *big.Int, aggExit *delta.Exit) error {
	// validator.PendingVET + agg.ExitVET are now unlocked
	// unlockedTVL here means that it's not contributing to TVL
	// values for the validator are still locked
	totalUnlockedVET := big.NewInt(0).Add(unlockedTVL, aggExit.ExitedTVL)

	if err := s.lockedVET.Sub(totalUnlockedVET); err != nil {
		return err
	}
	// unlockedTVLWeight already has the sum of the agg weights - LockedVET x2 + total weight from delegators
	if err := s.lockedWeight.Sub(unlockedTVLWeight); err != nil {
		return err
	}
	if err := s.queuedVET.Sub(aggExit.QueuedDecrease); err != nil {
		return err
	}
	if err := s.queuedWeight.Sub(aggExit.QueuedDecreaseWeight); err != nil {
		return err
	}

	return nil
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
