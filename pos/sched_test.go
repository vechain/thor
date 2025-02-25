// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func createParams() (map[thor.Address]*staker.Validator, *big.Int) {
	validators := make(map[thor.Address]*staker.Validator)
	totalStake := big.NewInt(0)
	for _, acc := range genesis.DevAccounts() {
		stake := big.NewInt(0).SetBytes(acc.Address[10:]) // use the last 10 bytes to create semi random, but deterministic stake
		validator := &staker.Validator{
			Weight: stake,
		}
		validators[acc.Address] = validator
		totalStake.Add(totalStake, validator.Weight)
	}

	return validators, totalStake
}

func TestNewScheduler_Seed(t *testing.T) {
	validators, totalStake := createParams()
	s1, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, totalStake, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	s2, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, totalStake, 1, 10, []byte("seed2"))
	assert.NoError(t, err)

	for i := range s1.placements {
		assert.NotEqual(t, s1.placements[i].hash, s2.placements[i].hash)
		v1 := s1.placements[i].addr
		v2 := s2.placements[i].addr
		assert.NotEqual(t, v1, v2)
	}
}

func TestScheduler_IsScheduled(t *testing.T) {
	validators, totalStake := createParams()
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, totalStake, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	assert.True(t, sched.IsScheduled(20, thor.MustParseAddress("0xf370940abdbd2583bc80bfc19d19bc216c88ccf0")))
}

func TestScheduler_Distribution(t *testing.T) {
	validators, totalStake := createParams()
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, totalStake, 1, 10, []byte("seed1"))
	assert.NoError(t, err)

	distribution := make(map[thor.Address]int)

	for i := uint64(1); i <= 100_000; i++ {
		addr := sched.expectedValidator(thor.BlockInterval * i)
		distribution[addr]++
	}

	for addr, count := range distribution {
		expectedWeight := new(big.Float).SetInt(validators[addr].Weight)
		expectedWeight.Quo(expectedWeight, new(big.Float).SetInt(totalStake))
		expectedCountFloat := new(big.Float).Mul(expectedWeight, big.NewFloat(100_000))

		expectedCount, _ := expectedCountFloat.Int64()

		tolerance := float64(expectedCount) * 0.05
		assert.InDeltaf(t, float64(count), float64(expectedCount), tolerance, "Distribution is not within tolerance for validator %v", addr)
	}
}

func TestScheduler_Schedule(t *testing.T) {
	parentTime := uint64(10)

	validators, totalStake := createParams()
	addr := thor.Address{}

	for i := uint64(1); i <= 1000; i++ {
		expectedNext := parentTime + thor.BlockInterval*i
		for _, acc := range genesis.DevAccounts() {
			sched, err := NewScheduler(acc.Address, validators, totalStake, 1, parentTime, []byte("seed1"))
			assert.NoError(t, err)
			newBlockTime, _ := sched.Schedule(20)
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
	nowTime := uint64(100)

	validators, totalStake := createParams()
	sched, err := NewScheduler(genesis.DevAccounts()[0].Address, validators, totalStake, 1, parentTime, []byte("seed1"))
	assert.NoError(t, err)

	missedSlots, score := sched.Updates(nowTime)

	assert.Equal(t, 8, len(missedSlots))
	assert.Equal(t, uint64(10), score)
}
