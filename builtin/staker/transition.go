package staker

import (
	"github.com/vechain/thor/v2/thor"
	"math/big"
)

// Transition activates the staker contract when sufficient validators are queued
func (s *Staker) Transition(currentBlock uint32) (bool, error) {
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

	// if the queue size is not AT LEAST 2/3 of the maxProposers, then return nil
	minimum := big.NewFloat(0).SetInt(maxProposers)
	minimum.Mul(minimum, big.NewFloat(2))
	minimum.Quo(minimum, big.NewFloat(3))
	if big.NewFloat(0).SetInt(queueSize).Cmp(minimum) < 0 {
		return false, nil
	}

	// Use existing activateValidators method for transition
	ids, err := s.activateValidators(currentBlock)
	if err != nil {
		return false, err
	}
	logger.Info("activated validations", "count", len(ids))

	return true, nil
}

// activateValidators is kept for the Transition method
func (s *Staker) activateValidators(currentBlock uint32) ([]*thor.Address, error) {
	queuedSize, err := s.QueuedGroupSize()
	if err != nil {
		return nil, err
	}
	leaderSize, err := s.LeaderGroupSize()
	if err != nil {
		return nil, err
	}
	maxSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, err
	}
	if leaderSize.Cmp(maxSize) >= 0 {
		return nil, nil
	}

	// no one is in the queue
	if queuedSize.Cmp(big.NewInt(0)) <= 0 {
		return nil, nil
	}

	queuedCount := queuedSize.Int64()
	leaderDelta := maxSize.Int64() - leaderSize.Int64()
	if leaderDelta > 0 {
		if leaderDelta < queuedCount {
			queuedCount = leaderDelta
		}
	} else {
		return nil, nil
	}

	activated := make([]*thor.Address, queuedCount)
	maxLeaderGroupSize, err := s.params.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, err
	}

	for i := int64(0); i < queuedCount; i++ {
		id, err := s.activateNextValidator(currentBlock, maxLeaderGroupSize)
		if err != nil {
			return nil, err
		}
		activated[i] = id
	}

	return activated, nil
}
