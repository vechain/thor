// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"math/big"
	"sync"

	"github.com/vechain/thor/v2/thor"
)

const (
	// VotingThreshold represents the 68% threshold mentioned in the requirements
	VotingThreshold = 68
	// MaxRounds represents the maximum number of voting rounds (180)
	MaxRounds = 180
)

// VotingLayer implements the voting model requirements:
// - Weighted round robin: Each cycle, each validator can vote a number of times proportional to their stake
// - No back-to-back: No one can vote twice in a row
// - When everyone exhausts their quota, the cycle resets
// - The cycle is dynamic: calculated based on validators and their stake at the start of each cycle

type VotingLayer struct {
	mu sync.RWMutex

	// Voting state
	excludedVoter   thor.Address // voter excluded for next round (no back-to-back)
	totalVotedStake *big.Int     // cumulative stake that has voted
	totalStake      *big.Int     // total stake in the system
	rounds          uint32       // current round number

	// Configuration
	maxRounds uint32
	threshold *big.Int // 68% of total stake

	// Strict rotation
	validatorStakes map[thor.Address]*big.Int // stake of each validator
	voteQuota       map[thor.Address]int      // available quotas per cycle
	votesUsed       map[thor.Address]int      // votes used in current cycle
	cycleSize       int                       // total quotas in the cycle
	cycleVotes      int                       // votes cast in current cycle
}

// NewVotingLayer creates a new voting layer with strict rotation
func NewVotingLayerWithValidators(totalStake *big.Int, validatorStakes map[thor.Address]*big.Int) *VotingLayer {
	threshold := new(big.Int).Mul(totalStake, big.NewInt(VotingThreshold))
	threshold.Div(threshold, big.NewInt(100))

	voteQuota := make(map[thor.Address]int)
	cycleSize := 0

	// Calculate quotas more fairly
	// Each validator has at least 1 quota, and additional quotas are distributed proportionally
	totalValidators := len(validatorStakes)
	if totalValidators == 0 {
		return &VotingLayer{
			excludedVoter:   thor.Address{},
			totalVotedStake: big.NewInt(0),
			totalStake:      totalStake,
			rounds:          0,
			maxRounds:       MaxRounds,
			threshold:       threshold,
			validatorStakes: validatorStakes,
			voteQuota:       voteQuota,
			votesUsed:       make(map[thor.Address]int),
			cycleSize:       0,
			cycleVotes:      0,
		}
	}

	// Assign 1 base quota to each validator
	for addr := range validatorStakes {
		voteQuota[addr] = 1
		cycleSize++
	}

	// Distribute additional quotas proportionally to stake
	// Use a multiplication factor to ensure integer quotas
	multiplier := big.NewInt(100) // Factor to avoid decimals

	for addr, stake := range validatorStakes {
		// Calculate additional quotas proportional to stake
		additional := new(big.Int).Mul(stake, multiplier)
		additional.Div(additional, totalStake)
		additionalInt := int(additional.Int64())
		if additionalInt > 0 {
			voteQuota[addr] += additionalInt
			cycleSize += additionalInt
		}
	}

	// If there are no additional quotas, ensure at least 2 quotas per validator to allow rotation
	if cycleSize == totalValidators {
		for addr := range validatorStakes {
			voteQuota[addr] = 2
		}
		cycleSize = totalValidators * 2
	}

	return &VotingLayer{
		excludedVoter:   thor.Address{},
		totalVotedStake: big.NewInt(0),
		totalStake:      totalStake,
		rounds:          0,
		maxRounds:       MaxRounds,
		threshold:       threshold,
		validatorStakes: validatorStakes,
		voteQuota:       voteQuota,
		votesUsed:       make(map[thor.Address]int),
		cycleSize:       cycleSize,
		cycleVotes:      0,
	}
}

// ShouldAllowVote determines if a validator can vote in this round
func (layer *VotingLayer) ShouldAllowVote(signer thor.Address, weight *big.Int) bool {
	layer.mu.Lock()
	defer layer.mu.Unlock()

	if layer.rounds >= layer.maxRounds {
		return false
	}
	if layer.hasReachedThreshold() {
		return false
	}
	if layer.excludedVoter == signer {
		return false
	}
	// Does it have available quotas in the cycle?
	quota, ok := layer.voteQuota[signer]
	if !ok || layer.votesUsed[signer] >= quota {
		return false
	}
	return true
}

// RecordVote records the vote and updates the state
func (layer *VotingLayer) RecordVote(signer thor.Address, weight *big.Int) {
	layer.mu.Lock()
	defer layer.mu.Unlock()

	layer.totalVotedStake.Add(layer.totalVotedStake, weight)
	layer.rounds++
	layer.excludedVoter = signer
	layer.votesUsed[signer]++
	layer.cycleVotes++

	// If the cycle ends, reset quotas
	if layer.cycleVotes >= layer.cycleSize {
		for k := range layer.votesUsed {
			layer.votesUsed[k] = 0
		}
		layer.cycleVotes = 0
	}
}

// Reset resets the state for a new voting cycle
func (layer *VotingLayer) ResetWithValidators(totalStake *big.Int, validatorStakes map[thor.Address]*big.Int) {
	layer.mu.Lock()
	defer layer.mu.Unlock()

	layer.excludedVoter = thor.Address{}
	layer.totalVotedStake = big.NewInt(0)
	layer.totalStake = totalStake
	layer.rounds = 0
	layer.validatorStakes = validatorStakes
	layer.voteQuota = make(map[thor.Address]int)
	layer.votesUsed = make(map[thor.Address]int)
	layer.cycleSize = 0
	layer.cycleVotes = 0

	// Calculate quotas more fairly (same logic as NewVotingLayerWithValidators)
	totalValidators := len(validatorStakes)
	if totalValidators == 0 {
		layer.threshold = new(big.Int).Mul(totalStake, big.NewInt(VotingThreshold))
		layer.threshold.Div(layer.threshold, big.NewInt(100))
		return
	}

	// Assign 1 base quota to each validator
	for addr := range validatorStakes {
		layer.voteQuota[addr] = 1
		layer.cycleSize++
	}

	// Distribute additional quotas proportionally to stake
	multiplier := big.NewInt(100) // Factor to avoid decimals

	for addr, stake := range validatorStakes {
		// Calculate additional quotas proportional to stake
		additional := new(big.Int).Mul(stake, multiplier)
		additional.Div(additional, totalStake)
		additionalInt := int(additional.Int64())
		if additionalInt > 0 {
			layer.voteQuota[addr] += additionalInt
			layer.cycleSize += additionalInt
		}
	}

	// If there are no additional quotas, ensure at least 2 quotas per validator to allow rotation
	if layer.cycleSize == totalValidators {
		for addr := range validatorStakes {
			layer.voteQuota[addr] = 2
		}
		layer.cycleSize = totalValidators * 2
	}

	layer.threshold = new(big.Int).Mul(totalStake, big.NewInt(VotingThreshold))
	layer.threshold.Div(layer.threshold, big.NewInt(100))
}

// hasReachedThreshold checks if we've reached the 68% threshold
func (layer *VotingLayer) hasReachedThreshold() bool {
	return layer.totalVotedStake.Cmp(layer.threshold) >= 0
}

// GetStats returns current statistics of the layer
func (layer *VotingLayer) GetStats() (rounds uint32, votedStake *big.Int, threshold *big.Int, reached bool) {
	layer.mu.RLock()
	defer layer.mu.RUnlock()

	return layer.rounds,
		new(big.Int).Set(layer.totalVotedStake),
		new(big.Int).Set(layer.threshold),
		layer.hasReachedThreshold()
}
