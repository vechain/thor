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
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

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
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

	// Set rounds to max
	layer.rounds = MaxRounds

	signer := thor.Address{0x01}
	weight := big.NewInt(100000)

	// Should not allow vote when max rounds reached
	assert.False(t, layer.ShouldAllowVote(signer, weight))
}

func TestVotingLayer_ShouldAllowVote_Threshold(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

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
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

	signer := thor.Address{0x01}

	// Exclude the voter (back-to-back prevention)
	layer.excludedVoter = signer

	weight := big.NewInt(100000)

	// Should not allow vote when excluded
	assert.False(t, layer.ShouldAllowVote(signer, weight))
}

func TestVotingLayer_RecordVote(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

	signer := thor.Address{0x01}
	weight := big.NewInt(100000)

	initialRounds := layer.rounds
	initialVotedStake := new(big.Int).Set(layer.totalVotedStake)

	// Record a vote
	layer.RecordVote(signer, weight)

	// Check that rounds increased
	assert.Equal(t, initialRounds+1, layer.rounds)

	// Check that voted stake increased
	expectedVotedStake := new(big.Int).Add(initialVotedStake, weight)
	assert.Equal(t, expectedVotedStake, layer.totalVotedStake)

	// Check that voter was excluded for next round
	assert.Equal(t, signer, layer.excludedVoter)
}

func TestVotingLayer_Reset(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

	// Add some state
	signer := thor.Address{0x01}
	weight := big.NewInt(100000)
	layer.RecordVote(signer, weight)

	// Reset with new total stake
	newTotalStake := big.NewInt(2000000)
	layer.ResetWithValidators(newTotalStake, map[thor.Address]*big.Int{})

	// Check that state was reset
	assert.Equal(t, uint32(0), layer.rounds)
	assert.Equal(t, big.NewInt(0), layer.totalVotedStake)
	assert.Equal(t, newTotalStake, layer.totalStake)
	assert.Equal(t, thor.Address{}, layer.excludedVoter)

	// Check that threshold was recalculated
	expectedThreshold := new(big.Int).Mul(newTotalStake, big.NewInt(68))
	expectedThreshold.Div(expectedThreshold, big.NewInt(100))
	assert.Equal(t, expectedThreshold, layer.threshold)
}

func TestVotingLayer_GetStats(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

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
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

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
	validatorStakes := map[thor.Address]*big.Int{
		{0x01}: big.NewInt(100000),
	}
	layer := NewVotingLayerWithValidators(totalStake, validatorStakes)

	// Test with zero weight
	assert.False(t, layer.ShouldAllowVote(thor.Address{0x01}, big.NewInt(0)))

	// Test with negative weight
	assert.False(t, layer.ShouldAllowVote(thor.Address{0x01}, big.NewInt(-1)))

	// Test with valid weight in first round
	weight := big.NewInt(100000) // 10% of total stake
	result := layer.ShouldAllowVote(thor.Address{0x01}, weight)
	// In first round, should be selected
	assert.True(t, result)
}

func TestVotingLayer_DeterministicSelection_Proportional(t *testing.T) {
	totalStake := big.NewInt(1000000)
	validatorStakes := map[thor.Address]*big.Int{
		{0x01}: big.NewInt(500000), // 50%
		{0x02}: big.NewInt(100000), // 10%
	}
	layer := NewVotingLayerWithValidators(totalStake, validatorStakes)

	largeValidator := thor.Address{0x01}
	largeStake := big.NewInt(500000)

	smallValidator := thor.Address{0x02}
	smallStake := big.NewInt(100000)

	largeSelections := 0
	smallSelections := 0

	for round := 0; round < 20; round++ {
		layer.rounds = uint32(round)
		if layer.ShouldAllowVote(largeValidator, largeStake) {
			largeSelections++
		}
		if layer.ShouldAllowVote(smallValidator, smallStake) {
			smallSelections++
		}
	}

	assert.Greater(t, largeSelections, smallSelections, "Large validator should be selected more often")
	assert.Greater(t, smallSelections, 0, "Small validator should also be selected")

	expectedRatio := float64(largeStake.Int64()) / float64(smallStake.Int64())
	actualRatio := float64(largeSelections) / float64(smallSelections)
	assert.Greater(t, actualRatio, 1.0, "Large validator should be selected more often")
	assert.Less(t, actualRatio, expectedRatio*2, "Ratio should not be too skewed")
}

func TestVotingLayer_WhaleProtection(t *testing.T) {
	// Scenario: One whale with 60% stake, many small validators with 40% total
	totalStake := big.NewInt(1000000)
	whaleStake := big.NewInt(600000) // 60% stake
	smallStake := big.NewInt(10000)  // 1% stake each

	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{
		{0x01}: whaleStake,
		{0x02}: smallStake,
		{0x03}: smallStake,
		{0x04}: smallStake,
		{0x05}: smallStake,
		{0x06}: smallStake,
		{0x07}: smallStake,
		{0x08}: smallStake,
		{0x09}: smallStake,
		{0x0A}: smallStake,
	})

	whaleAddr := thor.Address{0x01}
	smallAddrs := []thor.Address{
		{0x02}, {0x03}, {0x04}, {0x05}, {0x06},
		{0x07}, {0x08}, {0x09}, {0x0A}, {0x0B},
		{0x0C}, {0x0D}, {0x0E}, {0x0F}, {0x10},
		{0x11}, {0x12}, {0x13}, {0x14}, {0x15},
		{0x16}, {0x17}, {0x18}, {0x19}, {0x1A},
		{0x1B}, {0x1C}, {0x1D}, {0x1E}, {0x1F},
		{0x20}, {0x21}, {0x22}, {0x23}, {0x24},
		{0x25}, {0x26}, {0x27}, {0x28}, {0x29},
	}

	// Test that whale cannot vote back-to-back
	assert.True(t, layer.ShouldAllowVote(whaleAddr, whaleStake))
	layer.RecordVote(whaleAddr, whaleStake)

	// Whale should be excluded in next round
	assert.False(t, layer.ShouldAllowVote(whaleAddr, whaleStake))

	// Small validators should be able to vote
	votesAllowed := 0
	for _, addr := range smallAddrs {
		if layer.ShouldAllowVote(addr, smallStake) {
			votesAllowed++
			layer.RecordVote(addr, smallStake)
		}
	}

	// Should allow some small validators to vote
	assert.Greater(t, votesAllowed, 0, "Small validators should be able to vote")

	// After some small validators vote, whale should be able to vote again
	// (but not immediately due to back-to-back exclusion)
	rounds, _, _, _ := layer.GetStats()
	assert.Greater(t, rounds, uint32(1), "Should have multiple rounds")

	// Test that whale cannot dominate voting
	whaleVotes := 0
	smallVotes := 0

	// Reset for new test
	layer.ResetWithValidators(totalStake, map[thor.Address]*big.Int{})

	// Simulate multiple rounds
	for round := 0; round < 50; round++ {
		// Try whale first
		if layer.ShouldAllowVote(whaleAddr, whaleStake) {
			layer.RecordVote(whaleAddr, whaleStake)
			whaleVotes++
			continue
		}

		// Try small validators
		voted := false
		for _, addr := range smallAddrs {
			if layer.ShouldAllowVote(addr, smallStake) {
				layer.RecordVote(addr, smallStake)
				smallVotes++
				voted = true
				break
			}
		}

		if !voted {
			// No one could vote, might have reached threshold
			break
		}
	}

	// Whale should not dominate (should have fewer votes than stake proportion)
	// Whale has 60% stake but should not have 60% of votes due to back-to-back exclusion
	totalVotes := whaleVotes + smallVotes
	if totalVotes > 0 {
		whaleVotePercentage := float64(whaleVotes) / float64(totalVotes) * 100
		assert.Less(t, whaleVotePercentage, 60.0, "Whale should not dominate voting")
	}

	// Small validators should have significant participation
	assert.Greater(t, smallVotes, 0, "Small validators should participate")
}

func TestVotingLayer_DeterministicSelection_Fairness(t *testing.T) {
	totalStake := big.NewInt(1000000)
	layer := NewVotingLayerWithValidators(totalStake, map[thor.Address]*big.Int{})

	// Two validators with different stakes
	largeValidator := thor.Address{0x01}
	largeStake := big.NewInt(400000) // 40% stake

	smallValidator := thor.Address{0x02}
	smallStake := big.NewInt(100000) // 10% stake

	// Track selections over multiple rounds
	largeSelections := 0
	smallSelections := 0

	// Simulate 20 rounds
	for round := 0; round < 20; round++ {
		layer.rounds = uint32(round)

		// Check if large validator can vote
		if layer.ShouldAllowVote(largeValidator, largeStake) {
			largeSelections++
		}

		// Check if small validator can vote
		if layer.ShouldAllowVote(smallValidator, smallStake) {
			smallSelections++
		}
	}

	// Large validator should be selected more often (but not exclusively)
	assert.Greater(t, largeSelections, smallSelections, "Large validator should be selected more often")
	assert.Greater(t, smallSelections, 0, "Small validator should also be selected")

	// The ratio should be roughly proportional to stake (but not exact due to back-to-back exclusion)
	expectedRatio := float64(largeStake.Int64()) / float64(smallStake.Int64()) // 4:1
	actualRatio := float64(largeSelections) / float64(smallSelections)

	// Allow some variance due to back-to-back exclusion
	assert.Greater(t, actualRatio, 1.0, "Large validator should be selected more often")
	assert.Less(t, actualRatio, expectedRatio*2, "Ratio should not be too skewed")
}

func TestVotingLayer_WhaleProtection_StrictRotation(t *testing.T) {
	totalStake := big.NewInt(1000000)
	whaleStake := big.NewInt(600000) // 60% stake
	smallStake := big.NewInt(10000)  // 1% stake each

	validatorStakes := map[thor.Address]*big.Int{
		{0x01}: whaleStake,
		{0x02}: smallStake,
		{0x03}: smallStake,
		{0x04}: smallStake,
		{0x05}: smallStake,
		{0x06}: smallStake,
		{0x07}: smallStake,
		{0x08}: smallStake,
		{0x09}: smallStake,
		{0x0A}: smallStake,
	}

	layer := NewVotingLayerWithValidators(totalStake, validatorStakes)

	// El whale solo puede votar su cupo por ciclo
	whaleAddr := thor.Address{0x01}
	whaleQuota := layer.voteQuota[whaleAddr]
	whaleVotes := 0
	prevVoter := thor.Address{}

	for i := 0; i < whaleQuota+2; i++ { // Intentar votar más veces de su cupo
		if layer.ShouldAllowVote(whaleAddr, whaleStake) && whaleAddr != prevVoter {
			layer.RecordVote(whaleAddr, whaleStake)
			whaleVotes++
			prevVoter = whaleAddr
		} else {
			// Buscar un pequeño que pueda votar
			for addr := range validatorStakes {
				if addr == whaleAddr || addr == prevVoter {
					continue
				}
				if layer.ShouldAllowVote(addr, smallStake) {
					layer.RecordVote(addr, smallStake)
					prevVoter = addr
					break
				}
			}
		}
	}

	// El whale no puede votar más de su cupo por ciclo
	assert.Equal(t, whaleQuota, whaleVotes)

	// Todos los pequeños deben poder votar al menos una vez por ciclo
	for addr := range validatorStakes {
		if addr == whaleAddr {
			continue
		}
		assert.GreaterOrEqual(t, layer.votesUsed[addr], 1)
	}
}

func TestVotingLayer_StrictRotationCycle(t *testing.T) {
	totalStake := big.NewInt(1000000)
	stakeA := big.NewInt(500000) // 50%
	stakeB := big.NewInt(300000) // 30%
	stakeC := big.NewInt(200000) // 20%

	validatorStakes := map[thor.Address]*big.Int{
		{0x01}: stakeA,
		{0x02}: stakeB,
		{0x03}: stakeC,
	}

	layer := NewVotingLayerWithValidators(totalStake, validatorStakes)

	// Guardar cupos
	quotaA := layer.voteQuota[thor.Address{0x01}]
	quotaB := layer.voteQuota[thor.Address{0x02}]
	quotaC := layer.voteQuota[thor.Address{0x03}]

	votesA, votesB, votesC := 0, 0, 0
	prevVoter := thor.Address{}

	// Simular un ciclo completo
	for i := 0; i < layer.cycleSize; i++ {
		for addr, stake := range validatorStakes {
			if addr == prevVoter {
				continue
			}
			if layer.ShouldAllowVote(addr, stake) {
				layer.RecordVote(addr, stake)
				prevVoter = addr
				switch addr {
				case thor.Address{0x01}:
					votesA++
				case thor.Address{0x02}:
					votesB++
				case thor.Address{0x03}:
					votesC++
				}
				break
			}
		}
	}

	// Cada uno debe haber votado exactamente su cupo
	assert.Equal(t, quotaA, votesA)
	assert.Equal(t, quotaB, votesB)
	assert.Equal(t, quotaC, votesC)

	// Al terminar el ciclo, todos los cupos deben reiniciarse
	for addr := range validatorStakes {
		assert.Equal(t, 0, layer.votesUsed[addr])
	}
}
