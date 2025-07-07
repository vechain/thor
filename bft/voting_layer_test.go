// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestNewVotingLayer(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	assert.NotNil(t, layer)
	assert.Equal(t, totalStake, layer.totalStake)
	assert.Equal(t, uint32(0), layer.rounds)
	assert.Equal(t, MaxRounds, layer.maxRounds)

	// Check threshold calculation (68% of total stake)
	expectedThreshold := new(big.Int).Mul(totalStake, big.NewInt(68))
	expectedThreshold.Div(expectedThreshold, big.NewInt(100))
	assert.Equal(t, expectedThreshold, layer.threshold)
}

func TestVotingLayer_ShouldAllowVote_MaxRounds(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	// Set rounds to max
	layer.rounds = MaxRounds

	signer := thor.Address{0x01}
	weight := big.NewInt(100000)

	// Should not allow vote when max rounds reached
	assert.False(t, layer.ShouldAllowVote(signer, weight))
}

func TestVotingLayer_ShouldAllowVote_Threshold(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	// Set voted stake to threshold (68%)
	layer.totalVotedStake = new(big.Int).Mul(totalStake, big.NewInt(68))
	layer.totalVotedStake.Div(layer.totalVotedStake, big.NewInt(100))

	signer := thor.Address{0x01}
	weight := big.NewInt(100000)

	// Should not allow vote when threshold reached
	assert.False(t, layer.ShouldAllowVote(signer, weight))
}

func TestVotingLayer_ShouldAllowVote_Excluded(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	signer := thor.Address{0x01}

	// Exclude the voter (back-to-back prevention)
	layer.excludedVoter = signer

	weight := big.NewInt(100000)

	// Should not allow vote when excluded
	assert.False(t, layer.ShouldAllowVote(signer, weight))
}

func TestVotingLayer_RecordVote(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	signer := thor.Address{0x01}
	weight := big.NewInt(100000)

	initialRounds := layer.rounds
	initialVotedStake := new(big.Int).Set(layer.totalVotedStake)
	initialAccumulatedStake := new(big.Int).Set(layer.accumulatedStake)

	// Record a vote
	layer.RecordVote(signer, weight)

	// Check that rounds increased
	assert.Equal(t, initialRounds+1, layer.rounds)

	// Check that voted stake increased
	expectedVotedStake := new(big.Int).Add(initialVotedStake, weight)
	assert.Equal(t, expectedVotedStake, layer.totalVotedStake)

	// Check that accumulated stake increased
	expectedAccumulatedStake := new(big.Int).Add(initialAccumulatedStake, weight)
	assert.Equal(t, expectedAccumulatedStake, layer.accumulatedStake)

	// Check that voter was excluded for next round
	assert.Equal(t, signer, layer.excludedVoter)
}

func TestVotingLayer_Reset(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	// Add some state
	signer := thor.Address{0x01}
	weight := big.NewInt(100000)
	layer.RecordVote(signer, weight)

	// Reset with new total stake
	newTotalStake := big.NewInt(2000000)
	layer.Reset(newTotalStake)

	// Check that state was reset
	assert.Equal(t, uint32(0), layer.rounds)
	assert.Equal(t, big.NewInt(0), layer.totalVotedStake)
	assert.Equal(t, newTotalStake, layer.totalStake)
	assert.Equal(t, thor.Address{}, layer.excludedVoter)
	assert.Equal(t, big.NewInt(0), layer.accumulatedStake)

	// Check that threshold was recalculated
	expectedThreshold := new(big.Int).Mul(newTotalStake, big.NewInt(68))
	expectedThreshold.Div(expectedThreshold, big.NewInt(100))
	assert.Equal(t, expectedThreshold, layer.threshold)
}

func TestVotingLayer_GetStats(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	signer := thor.Address{0x01}
	weight := big.NewInt(100000)
	layer.RecordVote(signer, weight)

	rounds, votedStake, threshold, reached := layer.GetStats()

	assert.Equal(t, uint32(1), rounds)
	assert.Equal(t, weight, votedStake)
	assert.Equal(t, layer.threshold, threshold)
	assert.False(t, reached) // Should not have reached threshold yet
}

func TestVotingLayer_HasReachedThreshold(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	// Set voted stake to exactly 68%
	layer.totalVotedStake = new(big.Int).Mul(totalStake, big.NewInt(68))
	layer.totalVotedStake.Div(layer.totalVotedStake, big.NewInt(100))

	assert.True(t, layer.hasReachedThreshold())

	// Set voted stake to 67%
	layer.totalVotedStake = new(big.Int).Mul(totalStake, big.NewInt(67))
	layer.totalVotedStake.Div(layer.totalVotedStake, big.NewInt(100))

	assert.False(t, layer.hasReachedThreshold())
}

func TestVotingLayer_DeterministicSelection(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	// Test with zero weight
	assert.False(t, layer.deterministicSelection(big.NewInt(0)))

	// Test with negative weight
	assert.False(t, layer.deterministicSelection(big.NewInt(-1)))

	// Test with valid weight in first round
	weight := big.NewInt(100000) // 10% of total stake
	result := layer.deterministicSelection(weight)
	// In first round, should be selected
	assert.True(t, result)
}

func TestVotingLayer_DeterministicSelection_Proportional(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayer(totalStake)

	// Simulate multiple rounds to test proportional selection
	largeWeight := big.NewInt(500000) // 50% stake
	smallWeight := big.NewInt(100000) // 10% stake

	// First round - large validator should be selected
	layer.rounds = 1
	assert.True(t, layer.deterministicSelection(largeWeight))
	assert.False(t, layer.deterministicSelection(smallWeight))

	// Second round - small validator should be selected
	layer.rounds = 2
	layer.accumulatedStake = big.NewInt(500000) // Previous large vote
	assert.False(t, layer.deterministicSelection(largeWeight))
	assert.True(t, layer.deterministicSelection(smallWeight))
}
