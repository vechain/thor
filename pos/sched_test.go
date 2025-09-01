// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"encoding/binary"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func createParams() ([]Proposer, uint64) {
	validators := make([]Proposer, 0)
	totalStake := uint64(0)
	for _, acc := range genesis.DevAccounts() {
		stake := binary.BigEndian.Uint64(acc.Address[2:]) // use the last 4 bytes to create semi random, but deterministic stake
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
	s1, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	s2, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed2"))
	assert.NoError(t, err)

	time1 := s1.Schedule(20)
	time2 := s2.Schedule(20)
	assert.NotEqual(t, time1, time2)

	assert.NotEqual(t, s1.sequence[0], s2.sequence[0])
}

func TestNewScheduler_Schedule_ShouldNotPanic(t *testing.T) {
	validators, _ := createParams()
	parentTime := uint64(10)
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, parentTime, []byte("seed1"))
	assert.NoError(t, err)

	for i := range uint64(1000) {
		next := parentTime + thor.BlockInterval()*(i+1)
		sched.Schedule(next)
	}
}

func TestScheduler_IsScheduled(t *testing.T) {
	validators, _ := createParams()
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	assert.True(t, sched.IsScheduled(110, genesis.DevAccounts()[2].Address))
}

func TestScheduler_Distribution(t *testing.T) {
	// Reduce tolerance my increasing iterations to achieve a higher level of accuracy
	// e.g., 1 million usually gets all tolerances down to about 2% (i.e., 0.02)
	iterations := 100_000
	type stakeFunc func(index int, acc thor.Address) uint64
	rnd := rand.New(rand.NewSource(412342)) //nolint:gosec

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
					return 2000
				}
				return 1000
			},
		},
		{
			name:      "all_same",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) uint64 {
				return 1000
			},
		},
		{
			name:      "pseudo_random_weight",
			tolerance: 0.04,
			stakes: func(index int, acc thor.Address) uint64 {
				millionEth := uint64(1e6)
				maxWeight := uint64(1200) * millionEth // max with multipliers
				minWeight := uint64(50) * millionEth   // min with multipliers
				diff := maxWeight - minWeight

				// Generate random number in [0, diff)
				n := rnd.Intn(int(diff)) //nolint:gosec
				// Add min to shift range to [min, max)
				randomValue := minWeight + uint64(n)

				return randomValue
			},
		},
		{
			name:      "increasing",
			tolerance: 0.02,
			stakes: func(index int, acc thor.Address) uint64 {
				return uint64((index + 1) * 1000)
			},
		},
		{
			name:      "some whales",
			tolerance: 0.07, // less than 0.01 with 10m iterations
			stakes: func(index int, acc thor.Address) uint64 {
				if index < 3 {
					return 1200
				}
				return 50
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]Proposer, 0)
			totalStake := uint64(0)

			var weightMap = make(map[thor.Address]uint64)
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

				sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, parent, seed[:])
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
			sched, err := NewScheduler(acc.Address, validators, 1, parentTime, []byte("seed1"))
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
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, parentTime, []byte("seed1"))
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

	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
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
	sched, err := NewScheduler(lowStakeAcc, validators, 1, parent, []byte("seed1"))
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

	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	updates, score := sched.Updates(30, totalStake)
	assert.Equal(t, 1, len(updates), "There should be one update")

	onlineWeight := weight * uint64(len(validators)-1)

	expectedScore := onlineWeight * thor.MaxPosScore
	expectedScore = expectedScore / totalStake
	assert.Equal(t, expectedScore, score, "Score should be equal to the expected score")
}
