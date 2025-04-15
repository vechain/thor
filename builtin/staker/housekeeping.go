// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math"
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// Housekeep iterates over validations, move to cooldown
// take the oldest validator and move to exited
func (s *Staker) Housekeep(currentBlock uint32) (bool, error) {
	if currentBlock%epochLength != 0 { // we only perform housekeeping on epoch blocks
		return false, nil
	}
	maxLeaderGroupSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return false, err
	}

	validatorExitID := thor.Bytes32{}
	validatorLowestExitBlock := uint32(math.MaxUint32)

	iterator := func(id thor.Bytes32, entry *Validation) error {
		isPeriodEnd := entry.IsPeriodEnd(currentBlock)

		// validator is in the middle of a staking period, no need to process
		if !isPeriodEnd && entry.Status != StatusCooldown {
			return nil
		}

		// check if the validator should be moved to cooldown / exit
		if entry.Expiry != nil && currentBlock >= *entry.Expiry {
			if entry.Status == StatusActive && !entry.AutoRenew {
				return s.performCooldownUpdates(id, entry)
			}
			if entry.Status == StatusActive && entry.AutoRenew {
				return s.performRenewalUpdates(id, entry)
			}
			// Find validator with the lowest exit tx block
			if entry.Status == StatusCooldown && validatorLowestExitBlock > entry.ExitTxBlock && currentBlock >= *entry.Expiry+cooldownPeriod {
				validatorExitID = id
				validatorLowestExitBlock = entry.ExitTxBlock
			}
		}

		return nil
	}

	if err = s.validations.LeaderGroupIterator(iterator); err != nil {
		return false, err
	}

	leaderGroupLength, err := s.validations.leaderGroupSize.Get()
	if err != nil {
		return false, err
	}
	// min leader group size is 2/3 of max leader group size
	minimum := big.NewFloat(0).SetInt(maxLeaderGroupSize)
	minimum = minimum.Mul(minimum, big.NewFloat(2))
	minimum = minimum.Quo(minimum, big.NewFloat(3))
	currentLessOne := big.NewFloat(0).SetInt(leaderGroupLength)
	currentLessOne = currentLessOne.Sub(currentLessOne, big.NewFloat(1))

	// TODO: We need to check if the queue size + leader group size is less than 2/3 of max leader group size
	// Not just the active leader group size

	if !validatorExitID.IsZero() && currentLessOne.Cmp(minimum) >= 0 {
		if err := s.validations.ExitValidator(validatorExitID, currentBlock); err != nil {
			return false, err
		}
	}
	if err := s.activateValidators(currentBlock); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Staker) performRenewalUpdates(id thor.Bytes32, entry *Validation) error {
	// Renew the validator
	expiry := *entry.Expiry + entry.Period
	entry.Expiry = &expiry
	entry.CompleteIterations++

	// change in total value locked, ie the amount of VET that is locked
	changeTVL := big.NewInt(0)
	// change in TVL with multipliers applied
	changeWeight := big.NewInt(0)

	// move cooldown to withdrawable
	if entry.CooldownVET.Sign() == 1 {
		changeTVL = changeTVL.Sub(changeTVL, entry.CooldownVET)
		changeWeight = changeWeight.Sub(changeWeight, entry.CooldownVET)
		entry.WithdrawableVET = big.NewInt(0).Add(entry.WithdrawableVET, entry.CooldownVET)
		entry.CooldownVET = big.NewInt(0)
	}
	// move pending locked to locked
	if entry.PendingLocked.Sign() == 1 {
		changeTVL = changeTVL.Add(changeTVL, entry.PendingLocked)
		changeWeight = changeWeight.Add(changeWeight, entry.PendingLocked)
		entry.LockedVET = big.NewInt(0).Add(entry.LockedVET, entry.PendingLocked)
		entry.PendingLocked = big.NewInt(0)
	}

	delegation, err := s.storage.GetDelegation(id)
	if err != nil {
		return err
	}
	delegatorChangeTVL, delegatorChangeWeight := delegation.RenewDelegations()
	if err := s.storage.SetDelegation(id, delegation); err != nil {
		return err
	}
	changeTVL = changeTVL.Add(changeTVL, delegatorChangeTVL)
	changeWeight = changeWeight.Add(changeWeight, delegatorChangeWeight)

	entry.Weight = big.NewInt(0).Add(entry.Weight, changeWeight)
	if err := s.lockedVET.Add(changeTVL); err != nil {
		return err
	}

	return s.storage.SetValidator(id, entry)
}

func (s *Staker) performCooldownUpdates(id thor.Bytes32, entry *Validation) error {
	// Put to cooldown
	entry.Status = StatusCooldown
	// move locked to cooldown
	entry.CooldownVET = big.NewInt(0).Add(entry.CooldownVET, entry.LockedVET)
	entry.LockedVET = big.NewInt(0)
	// unlock delegator's stakes and remove their weight
	entry.CompleteIterations++
	entry.Weight = big.NewInt(0).Set(entry.CooldownVET)

	delegation, err := s.storage.GetDelegation(id)
	if err != nil {
		return err
	}
	exitedTVL := delegation.Exit()
	if err := s.storage.SetDelegation(id, delegation); err != nil {
		return err
	}

	if err := s.lockedVET.Sub(exitedTVL); err != nil {
		return err
	}

	return s.storage.SetValidator(id, entry)
}

func (s *Staker) activateValidators(currentBlock uint32) error {
	queuedSize, err := s.QueuedGroupSize()
	if err != nil {
		return err
	}
	leaderSize, err := s.LeaderGroupSize()
	if err != nil {
		return err
	}
	maxSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return err
	}
	if leaderSize.Cmp(maxSize) >= 0 {
		return nil
	}

	if queuedSize.Cmp(big.NewInt(0)) > 0 {
		queuedCount := queuedSize.Int64()
		maxLeaderGroupSize, err := s.params.Get(thor.KeyMaxBlockProposers)
		if err != nil {
			return err
		}
		leaderDelta := maxLeaderGroupSize.Int64() - leaderSize.Int64()
		if leaderDelta > 0 {
			if leaderDelta < queuedCount {
				queuedCount = leaderDelta
			}
		} else {
			queuedCount = 0
		}

		for i := int64(0); i < queuedCount; i++ {
			err := s.validations.ActivateNext(currentBlock, s.params)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Staker) Transition(currentBlock uint32) (bool, error) {
	active, err := s.IsActive()
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

	queueSize, err := s.validations.queuedGroupSize.Get()
	if err != nil {
		return false, err
	}

	// if the queue size is not AT LEAST 2/3 of the maxProposers, then return nil
	minimum := big.NewFloat(0).SetInt(maxProposers)
	minimum.Mul(minimum, big.NewFloat(2))
	minimum.Quo(minimum, big.NewFloat(3))
	if big.NewFloat(0).SetInt(queueSize).Cmp(minimum) < 0 {
		return false, nil
	}

	if err := s.activateValidators(currentBlock); err != nil {
		return false, err
	}

	return true, nil
}
