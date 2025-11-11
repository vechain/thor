// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package scheduler

import (
	"encoding/binary"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func createParams() ([]Proposer, uint64) {
	validators := make([]Proposer, 0)
	totalStake := uint64(0)
	for _, acc := range genesis.DevAccounts() {
		stake := binary.BigEndian.Uint64(acc.Address[4:]) // use the last 4 bytes to create semi random, but deterministic stake
		validators = append(validators, Proposer{
			Address: acc.Address,
			Active:  true,
			Weight:  stake,
		})
		totalStake += stake
	}

	return validators, totalStake
}

func TestNewScheduler_Seed(t *testing.T) {
	validators, _ := createParams()
	s1, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	s2, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed2"))
	assert.NoError(t, err)

	time1 := s1.Schedule(20)
	time2 := s2.Schedule(20)
	assert.NotEqual(t, time1, time2)

	assert.NotEqual(t, s1.sequence[0], s2.sequence[0])
}

func TestNewScheduler_Schedule_ShouldNotPanic(t *testing.T) {
	validators, _ := createParams()
	parentTime := uint64(10)
	sched, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, parentTime, []byte("seed1"))
	assert.NoError(t, err)

	for i := range uint64(1000) {
		next := parentTime + thor.BlockInterval()*(i+1)
		sched.Schedule(next)
	}
}

func TestScheduler_IsScheduled(t *testing.T) {
	validators, _ := createParams()
	sched, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	assert.True(t, sched.IsScheduled(130, genesis.DevAccounts()[2].Address))
}

func TestScheduler_Distribution(t *testing.T) {
	// Reduce tolerance my increasing iterations to achieve a higher level of accuracy
	// e.g., 1 million usually gets all tolerances down to about 2% (i.e., 0.02)
	iterations := 100_000
	type stakeFunc func(index int, acc thor.Address) uint64
	rnd := rand.New(rand.NewPCG(412342, 0)) //nolint:gosec

	testCases := []struct {
		name      string
		stakes    stakeFunc
		tolerance float64
	}{
		{
			name:      "some_big_some_small",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) uint64 {
				if index%2 == 0 {
					return 50e6
				}
				return 25e6
			},
		},
		{
			name:      "all_same",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) uint64 {
				return 25e6
			},
		},
		{
			name:      "pseudo_random_weight",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) uint64 {
				millionEth := uint64(1e6)
				maxWeight := uint64(1200) * millionEth // max with multipliers
				minWeight := uint64(50) * millionEth   // min with multipliers
				diff := maxWeight - minWeight

				// Generate random number in [0, diff)
				n := rnd.IntN(int(diff)) //nolint:gosec
				// Add min to shift range to [min, max)
				randomValue := minWeight + uint64(n)

				return randomValue
			},
		},
		{
			name:      "increasing",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) uint64 {
				return uint64(index * 1e6)
			},
		},
		{
			name:      "some whales",
			tolerance: 0.03, // less than 0.01 with 10m iterations
			stakes: func(index int, acc thor.Address) uint64 {
				if index < 3 {
					return 100e6
				}
				return 25e6
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]Proposer, 0)
			totalStake := uint64(0)

			weightMap := make(map[thor.Address]uint64)
			for i, acc := range genesis.DevAccounts() {
				stake := tc.stakes(i, acc.Address)
				validators = append(validators, Proposer{
					Address: acc.Address,
					Weight:  stake,
					Active:  true,
				})
				weightMap[acc.Address] = stake
				totalStake += stake
			}

			distribution := make(map[thor.Address]int)

			for i := uint64(1); i <= uint64(iterations); i++ {
				parent := i * thor.BlockInterval()
				next := parent + thor.BlockInterval()
				seed := big.NewInt(int64(i)).Bytes()

				sched, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, parent, seed[:])
				assert.NoError(t, err)

				for _, acc := range genesis.DevAccounts() {
					if sched.IsScheduled(next, acc.Address) {
						distribution[acc.Address]++
					}
				}
			}

			for id, count := range distribution {
				weight := float64(weightMap[id])
				weight = weight / float64(totalStake)
				expectedCount := weight * float64(iterations)

				diff := float64(expectedCount) * tc.tolerance
				diffPercent := (float64(expectedCount) - float64(count)) / float64(expectedCount)
				addr := thor.BytesToAddress(id[:])

				assert.InDeltaf(t, float64(count), float64(expectedCount), diff,
					"Validation %s has a distribution of %d, expected %d, diff %v",
					addr.String(), count, expectedCount, diffPercent,
				)
			}
		})
	}
}

func TestScheduler_Schedule(t *testing.T) {
	parentTime := uint64(10)

	validators, _ := createParams()
	addr := thor.Address{}

	for i := uint64(1); i <= 1000; i++ {
		expectedNext := parentTime + thor.BlockInterval()*i
		for _, acc := range genesis.DevAccounts() {
			sched, err := NewPoSScheduler(acc.Address, validators, 1, parentTime, []byte("seed1"))
			assert.NoError(t, err)
			newBlockTime := sched.Schedule(20)
			if newBlockTime == expectedNext {
				addr = acc.Address
			}
		}
		// we're checking all validators, so we should always find one that is scheduled
		assert.False(t, addr.IsZero())
	}
}

func TestScheduler_Updates(t *testing.T) {
	parentTime := uint64(10)
	nowTime := uint64(30)

	validators, _ := createParams()
	sched, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, parentTime, []byte("seed1"))
	assert.NoError(t, err)

	weights := make(map[thor.Address]uint64)

	totalWeight := uint64(0)
	for _, validator := range validators {
		weights[validator.Address] = validator.Weight
		totalWeight += validator.Weight
	}

	updates, score := sched.Updates(nowTime, totalWeight)

	offline := 0
	offlineWeight := uint64(0)
	for _, u := range updates {
		if !u.Active {
			offline++
			offlineWeight += weights[u.Address]
		}
	}

	scaledScore := totalWeight - offlineWeight
	scaledScore = scaledScore * thor.MaxPosScore
	scaledScore = scaledScore / totalWeight

	assert.Equal(t, 1, offline)
	assert.Equal(t, scaledScore, score)
}

func TestScheduler_TotalPlacements(t *testing.T) {
	validators, totalStake := createParams()

	weightMap := make(map[thor.Address]uint64)
	otherAcc := genesis.DevAccounts()[1].Address
	for i, validator := range validators {
		if validator.Address == otherAcc {
			validators[i].Active = false
		}
		weightMap[validator.Address] = validator.Weight
	}

	sched, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	assert.Equal(t, 9, len(sched.sequence))

	// check total stake in scheduler, should only use online validators
	total := uint64(0)
	for _, p := range sched.sequence {
		total += weightMap[p.address]
	}

	expectedStake := totalStake - weightMap[otherAcc]

	assert.Equal(t, expectedStake, total)
}

func TestScheduler_AllValidatorsScheduled(t *testing.T) {
	validators := make([]Proposer, 0)
	lowStakeAcc := genesis.DevAccounts()[0].Address
	for _, acc := range genesis.DevAccounts() {
		var stake uint64
		// this ensures the first account will be last in the list
		if acc.Address == lowStakeAcc {
			stake = 1
		} else {
			stake = 1e18
		}
		validators = append(validators, Proposer{
			Address: acc.Address,
			Weight:  stake,
			Active:  true,
		})
	}

	parent := uint64(10)
	sched, err := NewPoSScheduler(lowStakeAcc, validators, 1, parent, []byte("seed1"))
	assert.NoError(t, err)

	lowStakeBlockTime := sched.Schedule(20)
	diff := int(thor.BlockInterval()) * len(validators)
	assert.Equal(t, int(parent)+diff, int(lowStakeBlockTime))

	seen := make(map[thor.Address]bool)
	for _, id := range sched.sequence {
		if seen[id.address] {
			t.Fatalf("Validation %s is scheduled multiple times", id.address)
		}
		seen[id.address] = true
	}
	assert.Equal(t, len(seen), len(validators))
}

func TestScheduler_Schedule_TotalScore(t *testing.T) {
	validators := make([]Proposer, 0)
	totalStake := uint64(0)
	weight := uint64(10_000)
	for _, acc := range genesis.DevAccounts() {
		validators = append(validators, Proposer{
			Address: acc.Address,
			Weight:  weight,
			Active:  true,
		})
		totalStake += weight
	}

	sched, err := NewPoSScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	updates, score := sched.Updates(30, totalStake)
	assert.Equal(t, 1, len(updates), "There should be one update")

	onlineWeight := weight * uint64(len(validators)-1)

	expectedScore := onlineWeight * thor.MaxPosScore
	expectedScore = expectedScore / totalStake
	assert.Equal(t, expectedScore, score, "Score should be equal to the expected score")
}

func TestScheduler_ScoreComparison_DifferentWeights(t *testing.T) {
	proposers := []Proposer{
		{Address: thor.BytesToAddress([]byte("min_stake")), Weight: 25_000_000, Active: true},  // 25M VET (Min)
		{Address: thor.BytesToAddress([]byte("low_stake")), Weight: 40_000_000, Active: true},  // 40M VET
		{Address: thor.BytesToAddress([]byte("mid_low")), Weight: 50_000_000, Active: true},    // 50M VET (Mid)
		{Address: thor.BytesToAddress([]byte("mid_high")), Weight: 66_000_000, Active: true},   // 50M + 16M*1.0
		{Address: thor.BytesToAddress([]byte("high_stake")), Weight: 82_000_000, Active: true}, // 50M + 16M*2.0
		{Address: thor.BytesToAddress([]byte("very_high")), Weight: 150_000_000, Active: true}, // 150M VET
		{Address: thor.BytesToAddress([]byte("extreme")), Weight: 300_000_000, Active: true},   // 300M VET
		{Address: thor.BytesToAddress([]byte("max_stake")), Weight: 600_000_000, Active: true}, // 600M VET (Max)
	}

	sched, err := NewPoSScheduler(proposers[0].Address, proposers, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	t.Log("=== Priority Score Comparison with Real Network Weights ===")
	for i, entry := range sched.sequence {
		t.Logf("Validator %d: Weight=%d VET, Priority Score=%.10f",
			i+1, entry.weight, entry.score)
	}

	// Verify score differences
	scores := make([]float64, len(sched.sequence))
	for i, entry := range sched.sequence {
		scores[i] = entry.score
	}

	// Check if there are score differences
	hasDifference := false
	for i := 1; i < len(scores); i++ {
		if scores[i] != scores[0] {
			hasDifference = true
			break
		}
	}

	if hasDifference {
		t.Log("✅ Different weights produced different priority scores")
		// Calculate score difference range
		minScore := scores[0]
		maxScore := scores[0]
		for _, score := range scores {
			if score < minScore {
				minScore = score
			}
			if score > maxScore {
				maxScore = score
			}
		}
		scoreDiff := maxScore - minScore
		t.Logf("📊 Score difference range: %.10f (from %.10f to %.10f)", scoreDiff, minScore, maxScore)
	} else {
		t.Log("⚠️  All validators have the same priority score, weight differences not reflected")
	}

	// Analyze the relationship between weights and scores
	t.Log("\n📈 Weight vs Score Relationship Analysis:")
	for i, entry := range sched.sequence {
		if i > 0 {
			prevWeight := sched.sequence[i-1].weight
			prevScore := sched.sequence[i-1].score
			weightRatio := float64(entry.weight) / float64(prevWeight)
			scoreRatio := entry.score / prevScore
			t.Logf("  Weight ratio: %.2fx (%.0f → %.0f), Score ratio: %.6f (%.10f → %.10f)",
				weightRatio, float64(prevWeight), float64(entry.weight), scoreRatio, prevScore, entry.score)
		}
	}
}

func TestNewScheduler_UnauthorizedProposer(t *testing.T) {
	validators, _ := createParams()
	unauthorized := thor.BytesToAddress([]byte("not_in_list"))
	_, err := NewPoSScheduler(unauthorized, validators, 1, 10, []byte("seed1"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized block proposer")
}

func TestScheduler(t *testing.T) {
	validators, _ := createParams()
	sched, err := NewPoSScheduler(validators[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)
	T := thor.BlockInterval()

	tests := []struct {
		name      string
		action    func(*PoSScheduler) bool
		wantValue bool
	}{
		{
			name: "IsTheTime matches IsScheduled",
			action: func(s *PoSScheduler) bool {
				blockTime := s.Schedule(20)
				return s.IsScheduled(blockTime, s.proposer.Address) == s.IsTheTime(blockTime)
			},
			wantValue: true,
		},
		{
			name: "IsScheduled blockTime == parentBlockTime",
			action: func(s *PoSScheduler) bool {
				return s.IsScheduled(10, s.proposer.Address)
			},
			wantValue: false,
		},
		{
			name: "IsScheduled blockTime < parentBlockTime",
			action: func(s *PoSScheduler) bool {
				return s.IsScheduled(5, s.proposer.Address)
			},
			wantValue: false,
		},
		{
			name: "IsScheduled blockTime not aligned",
			action: func(s *PoSScheduler) bool {
				return s.IsScheduled(10+T+1, s.proposer.Address)
			},
			wantValue: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.action(sched)
			assert.Equal(t, tc.wantValue, got)
		})
	}
}

func TestScheduler_Updates_InactiveProposer(t *testing.T) {
	validators, _ := createParams()
	inactive := validators[0]
	inactive.Active = false
	validators[0] = inactive
	sched, err := NewPoSScheduler(inactive.Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)
	updates, _ := sched.Updates(30, 100)
	found := false
	for _, u := range updates {
		if u.Address == inactive.Address && u.Active {
			found = true
		}
	}
	assert.True(t, found, "Inactive proposer should be reactivated in updates")
}

func TestScheduler_Updates_ZeroTotalWeight(t *testing.T) {
	validators, _ := createParams()
	sched, err := NewPoSScheduler(validators[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)
	updates, score := sched.Updates(30, 0)
	assert.Equal(t, 0, int(score))
	assert.NotNil(t, updates)
}

func TestScheduler_Schedule_Panic(t *testing.T) {
	validators, _ := createParams()
	sched, err := NewPoSScheduler(validators[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)
	// Set the sequence to empty to force panic
	sched.sequence = []entry{}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Schedule should panic if proposer is not found in sequence")
		}
	}()
	sched.Schedule(20)
}
