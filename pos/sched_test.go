// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"math/big"
	mathrand "math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func createParams() (map[thor.Address]*staker.Validation, *big.Int) {
	validators := make(map[thor.Address]*staker.Validation)
	totalStake := big.NewInt(0)
	for _, acc := range genesis.DevAccounts() {
		stake := big.NewInt(0).SetBytes(acc.Address[10:]) // use the last 10 bytes to create semi random, but deterministic stake
		validator := &staker.Validation{
			Weight: stake,
			Online: true,
		}
		validators[acc.Address] = validator
		totalStake.Add(totalStake, validator.Weight)
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
		next := parentTime + thor.BlockInterval*(i+1)
		sched.Schedule(next)
	}
}

func TestScheduler_IsScheduled(t *testing.T) {
	validators, _ := createParams()
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	assert.True(t, sched.IsScheduled(20, genesis.DevAccounts()[2].Address))
}

func TestScheduler_Distribution(t *testing.T) {
	// Reduce tolerance my increasing iterations to achieve a higher level of accuracy
	// e.g., 1 million usually gets all tolerances down to about 2% (i.e., 0.02)
	iterations := 100_000
	type stakeFunc func(index int, acc thor.Address) *big.Int

	//  pseudo-random number generator
	randReader := mathrand.New(mathrand.NewSource(412342)) //nolint:gosec

	testCases := []struct {
		name      string
		stakes    stakeFunc
		tolerance float64
	}{
		{
			name:      "some_big_some_small",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) *big.Int {
				if index%2 == 0 {
					return big.NewInt(2000)
				}
				return big.NewInt(1000)
			},
		},
		{
			name:      "all_same",
			tolerance: 0.03,
			stakes: func(index int, acc thor.Address) *big.Int {
				return big.NewInt(1000)
			},
		},
		{
			name:      "pseudo_random_weight",
			tolerance: 0.04,
			stakes: func(index int, acc thor.Address) *big.Int {
				eth := big.NewInt(1)
				millionEth := new(big.Int).Mul(big.NewInt(1e6), eth)
				maxWeight := new(big.Int).Mul(big.NewInt(1200), millionEth) // max with multipliers
				minWeight := new(big.Int).Mul(big.NewInt(50), millionEth)   // min with multipliers
				diff := new(big.Int).Sub(maxWeight, minWeight)

				// Generate random number in [0, diff)
				n := new(big.Int).Rand(randReader, diff)
				// Add min to shift range to [min, max)
				randomValue := new(big.Int).Add(minWeight, n)

				return randomValue
			},
		},
		{
			name:      "increasing",
			tolerance: 0.02,
			stakes: func(index int, acc thor.Address) *big.Int {
				return new(big.Int).SetInt64((int64(index) + 1) * 1000)
			},
		},
		{
			name:      "some whales",
			tolerance: 0.07, // less than 0.01 with 10m iterations
			stakes: func(index int, acc thor.Address) *big.Int {
				if index < 3 {
					return big.NewInt(1200)
				}
				return big.NewInt(50)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validators := make(map[thor.Address]*staker.Validation)
			totalStake := big.NewInt(0)

			for i, acc := range genesis.DevAccounts() {
				stake := tc.stakes(i, acc.Address)
				stake = stake.Mul(stake, big.NewInt(1e18)) // convert to wei
				validators[acc.Address] = &staker.Validation{
					Weight: stake,
					Online: true,
				}
				totalStake.Add(totalStake, stake)
			}

			distribution := make(map[thor.Address]int)

			for i := uint64(1); i <= uint64(iterations); i++ {
				parent := i * thor.BlockInterval
				next := parent + thor.BlockInterval
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
				weight := new(big.Float).SetInt(validators[id].Weight)
				weight.Quo(weight, new(big.Float).SetInt(totalStake))
				expectedCountFloat := new(big.Float).Mul(weight, big.NewFloat(float64(iterations)))
				expectedCount, _ := expectedCountFloat.Int64()

				diff := float64(expectedCount) * tc.tolerance
				diffPercent := (float64(expectedCount) - float64(count)) / float64(expectedCount)
				addr := thor.BytesToAddress(id[:])

				assert.InDeltaf(t, float64(count), float64(expectedCount), diff,
					"Validator %s has a distribution of %d, expected %d, diff %v",
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
		expectedNext := parentTime + thor.BlockInterval*i
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

	updates, score := sched.Updates(nowTime)

	offline := 0
	for _, online := range updates {
		if !online {
			offline++
		}
	}

	assert.Equal(t, 1, offline)
	assert.Equal(t, 9, int(score))
}

func TestScheduler_TotalPlacements(t *testing.T) {
	validators, totalStake := createParams()

	otherAcc := genesis.DevAccounts()[1].Address
	validators[otherAcc].Online = false

	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	assert.Equal(t, 9, len(sched.sequence))

	// check total stake in scheduler, should only use online validators
	total := big.NewInt(0)
	for _, p := range sched.sequence {
		total.Add(total, validators[p].Weight)
	}

	expectedStake := totalStake.Sub(totalStake, validators[otherAcc].Weight)

	assert.True(t, total.Cmp(expectedStake) == 0)
}

func TestScheduler_AllValidatorsScheduled(t *testing.T) {
	validators := make(map[thor.Address]*staker.Validation)
	lowStakeAcc := genesis.DevAccounts()[0].Address
	for _, acc := range genesis.DevAccounts() {
		var stake *big.Int
		// this ensures the first account will be last in the list
		if acc.Address == lowStakeAcc {
			stake = big.NewInt(1)
		} else {
			eth := big.NewInt(1e18)
			stake = new(big.Int).Mul(eth, eth)
		}
		validator := &staker.Validation{
			Weight: stake,
			Online: true,
		}
		validators[acc.Address] = validator
	}

	parent := uint64(10)
	sched, err := NewScheduler(lowStakeAcc, validators, 1, parent, []byte("seed1"))
	assert.NoError(t, err)

	lowStakeBlockTime := sched.Schedule(20)
	diff := int(thor.BlockInterval) * len(validators)
	assert.Equal(t, int(parent)+diff, int(lowStakeBlockTime))

	seen := make(map[thor.Address]bool)
	for _, id := range sched.sequence {
		if seen[id] {
			t.Fatalf("Validator %s is scheduled multiple times", id)
		}
		seen[id] = true
	}
	assert.Equal(t, len(seen), len(validators))
}
