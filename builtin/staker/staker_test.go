// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...interface{}) []interface{} {
	return a
}

// RandomStake returns a random number between minStake and maxStake
func RandomStake() *big.Int {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Calculate the range (maxStake - minStake)
	rangeStake := new(big.Int).Sub(maxStake, minStake)

	// Generate a random number within the range
	randomOffset := new(big.Int).Rand(rand.New(rand.NewSource(time.Now().UnixNano())), rangeStake)

	// Add minStake to ensure the value is within the desired range
	return new(big.Int).Add(minStake, randomOffset)
}

func newStaker() *Staker {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	return New(thor.BytesToAddress([]byte("stkr")), st)
}

func TestStaker(t *testing.T) {
	validatorAcc := thor.BytesToAddress([]byte("v1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	zeroStake := big.NewInt(0).SetBytes(thor.Bytes32{}.Bytes())

	stkr := newStaker()

	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{M(stkr.TotalStake()), M(zeroStake, nil)},
		{stkr.AddValidator(validatorAcc, stakeAmount), nil},
		{M(stkr.TotalStake()), M(stakeAmount, nil)},
		{M(stkr.FirstQueued()), M(validatorAcc, nil)},
		{M(stkr.ActivateNextValidator()), M(nil)},
		{M(stkr.FirstActive()), M(validatorAcc, nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestStaker_TotalStake(t *testing.T) {
	staker := newStaker()

	totalStaked := big.NewInt(0)
	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		stakes[addr] = stakeAmount
		assert.NoError(t, staker.AddValidator(addr, stakeAmount))
		//assert.NoError(t, staker.ActivateNextValidator())
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
		staked, err := staker.TotalStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}

	for _ = range stakes {
		assert.NoError(t, staker.ActivateNextValidator())
		staked, err := staker.TotalStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}

	for addr, stake := range stakes {
		assert.NoError(t, staker.RemoveValidator(addr))
		totalStaked = totalStaked.Sub(totalStaked, stake)
		staked, err := staker.TotalStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}
}

func TestStaker_ActiveStake(t *testing.T) {
	staker := newStaker()

	totalStaked := big.NewInt(0)
	activeStaked := big.NewInt(0)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		stakes[addr] = stakeAmount
		assert.NoError(t, staker.AddValidator(addr, stakeAmount))
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
	}

	actual, err := staker.ActiveStake()
	assert.Nil(t, err)
	assert.True(t, activeStaked.Cmp(actual) == 0)

	for _ = range stakers {
		head, err := staker.FirstQueued()
		assert.NoError(t, err)
		stake := stakes[head]
		assert.NoError(t, staker.ActivateNextValidator())
		activeStaked = activeStaked.Add(activeStaked, stake)
		actual, err = staker.ActiveStake()
		assert.Nil(t, err)
		assert.True(t, activeStaked.Cmp(actual) == 0)
	}
}

func TestStaker_AddValidator_MinimumStake(t *testing.T) {
	staker := newStaker()

	tooLow := big.NewInt(0).Sub(minStake, big.NewInt(1))
	assert.Error(t, staker.AddValidator(datagen.RandAddress(), tooLow))
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), minStake))
}

func TestStaker_AddValidator_MaximumStake(t *testing.T) {
	staker := newStaker()

	tooHigh := big.NewInt(0).Add(maxStake, big.NewInt(1))
	assert.Error(t, staker.AddValidator(datagen.RandAddress(), tooHigh))
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), maxStake))
}

func TestStaker_AddValidator_Duplicate(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	stake := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	assert.NoError(t, staker.AddValidator(addr, stake))
	assert.Error(t, staker.AddValidator(addr, stake))
}

func TestStaker_AddValidator_QueueOrder(t *testing.T) {
	staker := newStaker()

	// add 100 validators to the queue
	stakers := make([]thor.Address, 0)
	for i := 0; i < 100; i++ {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, stake))
		stakers = append(stakers, addr)
	}

	first, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, stakers[0], first)

	// iterating using the `Next` method should return the same order
	loopAddr := first
	for i := 1; i < 100; i++ {
		next, err := staker.Next(loopAddr)
		assert.NoError(t, err)
		assert.Equal(t, stakers[i], *next)
		loopAddr = *next
	}

	// activating validators should continue to set the correct head of the queue
	for i := 0; i < 99; i++ {
		assert.NoError(t, staker.ActivateNextValidator())
		first, err = staker.FirstQueued()
		assert.NoError(t, err)
		assert.Equal(t, stakers[i+1], first)
	}
}

func TestStaker_AddValidator(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr, stake))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)
}

func TestStaker_Get_NonExistent(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr, stake))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)

	assert.NoError(t, staker.ActivateNextValidator())

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker := newStaker()

	// fill 101 validators to leader group
	for i := 0; i < 101; i++ {
		assert.NoError(t, staker.AddValidator(datagen.RandAddress(), RandomStake()))
		assert.NoError(t, staker.ActivateNextValidator())
	}

	// try to add one more to the leadergroup
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), RandomStake()))
	assert.Error(t, staker.ActivateNextValidator())
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker := newStaker()
	assert.Error(t, staker.ActivateNextValidator())
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr, stake))
	assert.NoError(t, staker.ActivateNextValidator())

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	assert.Error(t, staker.RemoveValidator(addr))
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr, stake))
	assert.NoError(t, staker.ActivateNextValidator())
	assert.NoError(t, staker.RemoveValidator(addr))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, stake, validator.Stake)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker := newStaker()

	stakes := make(map[thor.Address]*big.Int)
	for i := 0; i < 10; i++ {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, stake))
		assert.NoError(t, staker.ActivateNextValidator())
		stakes[addr] = stake
	}

	leaderGroup, err := staker.LeaderGroup()
	assert.NoError(t, err)

	for addr, stake := range stakes {
		assert.Contains(t, leaderGroup, addr)
		assert.Equal(t, stake, leaderGroup[addr].Stake)
	}
}

func TestStaker_Next_Empty(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	next, err := staker.Next(addr)
	assert.Error(t, err)
	assert.Nil(t, next)
}

func TestStaker_Next(t *testing.T) {
	staker := newStaker()

	leaderGroup := make([]thor.Address, 0)
	for i := 0; i < 100; i++ {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, stake))
		assert.NoError(t, staker.ActivateNextValidator())
		leaderGroup = append(leaderGroup, addr)
	}

	queued := make([]thor.Address, 0)
	for i := 0; i < 100; i++ {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, stake))
		queued = append(queued, addr)
	}

	firstLeader, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, leaderGroup[0], firstLeader)

	for i := 0; i < 99; i++ {
		next, err := staker.Next(leaderGroup[i])
		assert.NoError(t, err)
		assert.Equal(t, leaderGroup[i+1], *next)
	}

	firstQueued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued[0], firstQueued)

	for i := 0; i < 99; i++ {
		next, err := staker.Next(queued[i])
		assert.NoError(t, err)
		assert.Equal(t, queued[i+1], *next)
	}
}

func TestStaker_GetStake(t *testing.T) {
	staker := newStaker()

	addr := datagen.RandAddress()
	stake := RandomStake()

	balance, err := staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Nil(t, balance)

	assert.NoError(t, staker.AddValidator(addr, stake))
	balance, err = staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, balance)

	assert.NoError(t, staker.ActivateNextValidator())
	balance, err = staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, balance)

	assert.NoError(t, staker.RemoveValidator(addr))
	balance, err = staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, balance)
}

func TestStaker_WithdrawStake(t *testing.T) {
	staker := newStaker()
	addr := datagen.RandAddress()

	withdrawAmount, err := staker.WithdrawStake(addr)
	assert.NoError(t, err)
	assert.Nil(t, withdrawAmount)

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(addr, stake))
	withdrawAmount, err = staker.WithdrawStake(addr)
	assert.Error(t, err)
	assert.Nil(t, withdrawAmount)

	assert.NoError(t, staker.ActivateNextValidator())
	withdrawAmount, err = staker.WithdrawStake(addr)
	assert.Error(t, err)
	assert.Nil(t, withdrawAmount)

	assert.NoError(t, staker.RemoveValidator(addr))
	withdrawAmount, err = staker.WithdrawStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
}
