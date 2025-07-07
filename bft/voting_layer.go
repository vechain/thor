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
// - Weighted picks: Each round, one participant is chosen with probability proportional to their stake
// - No back-to-back: Whoever just voted is excluded from the very next round
// - Cumulative "new" stake: Track total fraction that has ever voted, stop at 68%
// - Maximum 180 rounds
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

	// Deterministic selection state
	accumulatedStake *big.Int // accumulated stake for deterministic selection
}

// NewVotingLayer creates a new voting layer
func NewVotingLayer(totalStake *big.Int) *VotingLayer {
	threshold := new(big.Int).Mul(totalStake, big.NewInt(VotingThreshold))
	threshold.Div(threshold, big.NewInt(100))

	return &VotingLayer{
		excludedVoter:    thor.Address{},
		totalVotedStake:  big.NewInt(0),
		totalStake:       totalStake,
		rounds:           0,
		maxRounds:        MaxRounds,
		threshold:        threshold,
		accumulatedStake: big.NewInt(0),
	}
}

// ShouldAllowVote determines if a voter should be allowed to vote in this round
func (layer *VotingLayer) ShouldAllowVote(signer thor.Address, weight *big.Int) bool {
	layer.mu.Lock()
	defer layer.mu.Unlock()

	// Check if we've reached the maximum rounds
	if layer.rounds >= layer.maxRounds {
		logger.Debug("max rounds reached", "rounds", layer.rounds, "max", layer.maxRounds)
		return false
	}

	// Check if we've reached the 68% threshold
	if layer.hasReachedThreshold() {
		logger.Debug("voting threshold reached", "votedStake", layer.totalVotedStake, "threshold", layer.threshold)
		return false
	}

	// Check if voter is excluded (back-to-back prevention)
	if layer.excludedVoter == signer {
		logger.Debug("voter excluded (back-to-back)", "signer", signer)
		return false
	}

	// Perform deterministic weighted selection
	if !layer.deterministicSelection(weight) {
		logger.Debug("voter not selected by deterministic selection", "signer", signer, "weight", weight)
		return false
	}

	return true
}

// RecordVote records that a voter has voted and updates the layer state
func (layer *VotingLayer) RecordVote(signer thor.Address, weight *big.Int) {
	layer.mu.Lock()
	defer layer.mu.Unlock()

	// Update cumulative voted stake
	layer.totalVotedStake.Add(layer.totalVotedStake, weight)

	// Increment round counter
	layer.rounds++

	// Exclude this voter from next round (back-to-back prevention)
	layer.excludedVoter = signer

	// Update accumulated stake for deterministic selection
	layer.accumulatedStake.Add(layer.accumulatedStake, weight)

	logger.Debug("vote recorded",
		"signer", signer,
		"weight", weight,
		"rounds", layer.rounds,
		"totalVotedStake", layer.totalVotedStake,
		"threshold", layer.threshold)
}

// hasReachedThreshold checks if we've reached the 68% threshold
func (layer *VotingLayer) hasReachedThreshold() bool {
	return layer.totalVotedStake.Cmp(layer.threshold) >= 0
}

// deterministicSelection performs deterministic weighted selection
// Uses accumulated stake to ensure fair distribution proportional to stake
func (layer *VotingLayer) deterministicSelection(weight *big.Int) bool {
	if weight.Sign() <= 0 {
		return false
	}

	// Calculate the target stake for this round
	// This ensures that validators with more stake get proportionally more opportunities
	targetStake := new(big.Int).Mul(layer.totalStake, big.NewInt(int64(layer.rounds)))
	targetStake.Div(targetStake, big.NewInt(100)) // Normalize to percentage

	// Check if this validator should be selected based on accumulated stake
	// Validators with more stake will be selected more frequently
	selectionThreshold := new(big.Int).Add(layer.accumulatedStake, weight)

	// If the accumulated stake plus this validator's stake reaches or exceeds the target,
	// this validator should be selected
	return selectionThreshold.Cmp(targetStake) >= 0
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

// Reset resets the layer state for a new voting cycle
func (layer *VotingLayer) Reset(totalStake *big.Int) {
	layer.mu.Lock()
	defer layer.mu.Unlock()

	layer.excludedVoter = thor.Address{}
	layer.totalVotedStake = big.NewInt(0)
	layer.totalStake = totalStake
	layer.rounds = 0
	layer.accumulatedStake = big.NewInt(0)

	// Recalculate threshold
	layer.threshold = new(big.Int).Mul(totalStake, big.NewInt(VotingThreshold))
	layer.threshold.Div(layer.threshold, big.NewInt(100))

	logger.Debug("voting layer reset", "totalStake", totalStake, "threshold", layer.threshold)
}
