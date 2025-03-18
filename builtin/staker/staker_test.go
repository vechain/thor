// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// #nosec G404
package staker

import (
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
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

func addAuthorities(t *testing.T, auth *authority.Authority, authorities int) {
	for range authorities {
		nodeMaster := datagen.RandAddress()
		endorsor := datagen.RandAddress()
		identity := datagen.RandomHash()

		ok, err := auth.Add(nodeMaster, endorsor, identity)
		assert.NoError(t, err)
		assert.True(t, ok)
	}
}

func newStaker(t *testing.T, authorities int, maxValidators int64) *Staker {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	auth := authority.New(thor.BytesToAddress([]byte("auth")), st)
	addAuthorities(t, auth, authorities)
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators)))
	staker := New(thor.BytesToAddress([]byte("stkr")), st)
	assert.NoError(t, staker.Initialise(auth, param, 0))
	return staker
}

func TestStaker(t *testing.T) {
	validatorAcc := thor.BytesToAddress([]byte("v1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	zeroStake := big.NewInt(0).SetBytes(thor.Bytes32{}.Bytes())

	stkr := newStaker(t, 0, 101)

	tests := []struct {
		ret      any
		expected any
	}{
		{M(stkr.TotalStake()), M(zeroStake, nil)},
		{stkr.AddValidator(0, validatorAcc, uint32(360)*24*14, stakeAmount), nil},
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
	staker := newStaker(t, 0, 101)

	totalStaked := big.NewInt(0)
	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		stakes[addr] = stakeAmount
		assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stakeAmount))
		//assert.NoError(t, staker.ActivateNextValidator())
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
		staked, err := staker.TotalStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}

	for range stakes {
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
	staker := newStaker(t, 0, 101)

	totalStaked := big.NewInt(0)
	activeStaked := big.NewInt(0)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		stakes[addr] = stakeAmount
		assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stakeAmount))
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
	}

	actual, err := staker.ActiveStake()
	assert.Nil(t, err)
	assert.True(t, activeStaked.Cmp(actual) == 0)

	for range stakers {
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

func TestStaker_ActiveStake_FullFlow(t *testing.T) {
	staker := newStaker(t, 0, 101)

	addr := datagen.RandAddress()
	stake := RandomStake()

	// active and total should be 0
	activeStake, err := staker.ActiveStake()
	assert.NoError(t, err)
	assert.True(t, activeStake.Sign() == 0)

	totalStake, err := staker.TotalStake()
	assert.NoError(t, err)
	assert.True(t, totalStake.Sign() == 0)

	// add 1 validator, active should be 0, total should be stake
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
	activeStake, err = staker.ActiveStake()
	assert.NoError(t, err)
	assert.True(t, activeStake.Sign() == 0)

	totalStake, err = staker.TotalStake()
	assert.NoError(t, err)
	assert.True(t, totalStake.Cmp(stake) == 0)

	// activate the validator, active should be stake, total should be stake
	assert.NoError(t, staker.ActivateNextValidator())
	activeStake, err = staker.ActiveStake()
	assert.NoError(t, err)
	assert.True(t, activeStake.Cmp(stake) == 0)

	totalStake, err = staker.TotalStake()
	assert.NoError(t, err)
	assert.True(t, totalStake.Cmp(stake) == 0)

	// remove the validator, active should be 0, total should be 0
	assert.NoError(t, staker.RemoveValidator(addr))
	activeStake, err = staker.ActiveStake()
	assert.NoError(t, err)
	assert.True(t, activeStake.Sign() == 0)

	totalStake, err = staker.TotalStake()
	assert.NoError(t, err)
	assert.True(t, totalStake.Sign() == 0)

	// withdraw, active and total should remain unchanged
	withdrawAmount, err := staker.WithdrawStake(addr)
	assert.NoError(t, err)
	assert.True(t, withdrawAmount.Cmp(stake) == 0)

	activeStake, err = staker.ActiveStake()
	assert.NoError(t, err)
	assert.True(t, activeStake.Sign() == 0)
}

func TestStaker_AddValidator_MinimumStake(t *testing.T) {
	staker := newStaker(t, 0, 101)

	tooLow := big.NewInt(0).Sub(minStake, big.NewInt(1))
	assert.ErrorContains(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, tooLow), "stake is out of range")
	assert.NoError(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, minStake))
}

func TestStaker_AddValidator_MaximumStake(t *testing.T) {
	staker := newStaker(t, 0, 101)

	tooHigh := big.NewInt(0).Add(maxStake, big.NewInt(1))
	assert.ErrorContains(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, tooHigh), "stake is out of range")
	assert.NoError(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, maxStake))
}

func TestStaker_AddValidator_MaximumStakingPeriod(t *testing.T) {
	staker := newStaker(t, 101, 101)

	assert.ErrorContains(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*400, minStake), "expiry is out of boundaries")
	assert.NoError(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, minStake))
}

func TestStaker_AddValidator_MinimumStakingPeriod(t *testing.T) {
	staker := newStaker(t, 101, 101)

	assert.ErrorContains(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*1, minStake), "expiry is out of boundaries")
	assert.ErrorContains(t, staker.AddValidator(uint32(360)*24*1, datagen.RandAddress(), 100, minStake), "expiry is out of boundaries")
	assert.NoError(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, minStake))
}

func TestStaker_AddValidator_Duplicate(t *testing.T) {
	staker := newStaker(t, 0, 101)

	addr := datagen.RandAddress()
	stake := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
	assert.ErrorContains(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake), "validator already exists")
}

func TestStaker_AddValidator_QueueOrder(t *testing.T) {
	staker := newStaker(t, 0, 101)

	// add 100 validators to the queue
	stakers := make([]thor.Address, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
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
		assert.Equal(t, stakers[i], next)
		loopAddr = next
	}

	// activating validators should continue to set the correct head of the queue
	for i := range 99 {
		assert.NoError(t, staker.ActivateNextValidator())
		first, err = staker.FirstQueued()
		assert.NoError(t, err)
		assert.Equal(t, stakers[i+1], first)
	}
}

func TestStaker_AddValidator(t *testing.T) {
	staker := newStaker(t, 101, 101)

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)
}

func TestStaker_Get_NonExistent(t *testing.T) {
	staker := newStaker(t, 101, 101)

	addr := datagen.RandAddress()
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get(t *testing.T) {
	staker := newStaker(t, 0, 101)

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))

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

func TestStaker_Get_FullFlow(t *testing.T) {
	staker := newStaker(t, 0, 101)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// add the validator
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// activate the validator
	assert.NoError(t, staker.ActivateNextValidator())
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// housekeep the validator
	_, err = staker.Housekeep(uint32(360)*24*14, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// remove the validator
	assert.NoError(t, staker.RemoveValidator(addr))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker := newStaker(t, 0, 101)

	// fill 101 validators to leader group
	for range 101 {
		assert.NoError(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, RandomStake()))
		assert.NoError(t, staker.ActivateNextValidator())
	}

	// try to add one more to the leadergroup
	assert.NoError(t, staker.AddValidator(0, datagen.RandAddress(), uint32(360)*24*14, RandomStake()))
	assert.ErrorContains(t, staker.ActivateNextValidator(), "leader group is full")
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker := newStaker(t, 101, 101)
	assert.ErrorContains(t, staker.ActivateNextValidator(), "leader group is full")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker := newStaker(t, 0, 101)

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
	assert.NoError(t, staker.ActivateNextValidator())

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker := newStaker(t, 101, 101)

	addr := datagen.RandAddress()
	assert.NoError(t, staker.RemoveValidator(addr))
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker := newStaker(t, 0, 101)

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
	assert.NoError(t, staker.ActivateNextValidator())
	assert.NoError(t, staker.RemoveValidator(addr))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, stake, validator.Stake)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker := newStaker(t, 0, 101)

	stakes := make(map[thor.Address]*big.Int)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
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
	staker := newStaker(t, 101, 101)

	addr := datagen.RandAddress()
	next, err := staker.Next(addr)
	assert.Nil(t, err)
	assert.Equal(t, thor.Address{}, next)
}

func TestStaker_Next(t *testing.T) {
	staker := newStaker(t, 0, 101)

	leaderGroup := make([]thor.Address, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
		assert.NoError(t, staker.ActivateNextValidator())
		leaderGroup = append(leaderGroup, addr)
	}

	queued := make([]thor.Address, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
		queued = append(queued, addr)
	}

	firstLeader, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, leaderGroup[0], firstLeader)

	for i := range 99 {
		next, err := staker.Next(leaderGroup[i])
		assert.NoError(t, err)
		assert.Equal(t, leaderGroup[i+1], next)
	}

	firstQueued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued[0], firstQueued)

	for i := range 99 {
		next, err := staker.Next(queued[i])
		assert.NoError(t, err)
		assert.Equal(t, queued[i+1], next)
	}
}

func TestStaker_GetStake(t *testing.T) {
	staker := newStaker(t, 0, 101)

	addr := datagen.RandAddress()
	stake := RandomStake()

	balance, err := staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Nil(t, balance)

	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
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
	staker := newStaker(t, 0, 101)
	addr := datagen.RandAddress()

	withdrawAmount, err := staker.WithdrawStake(addr)
	assert.NoError(t, err)
	assert.True(t, withdrawAmount.Sign() == 0)

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, stake))
	withdrawAmount, err = staker.WithdrawStake(addr)
	assert.ErrorContains(t, err, "validator is not inactive")
	assert.Nil(t, withdrawAmount)

	assert.NoError(t, staker.ActivateNextValidator())
	withdrawAmount, err = staker.WithdrawStake(addr)
	assert.ErrorContains(t, err, "validator is not inactive")
	assert.Nil(t, withdrawAmount)

	assert.NoError(t, staker.RemoveValidator(addr))
	withdrawAmount, err = staker.WithdrawStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
}

func TestStaker_Initialise(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := datagen.RandAddress()

	staker := New(thor.BytesToAddress([]byte("stkr")), st)

	auth := authority.New(thor.BytesToAddress([]byte("auth")), st)
	param := params.New(thor.BytesToAddress([]byte("params")), st)

	assert.NoError(t, staker.Initialise(auth, param, 0)) // should succeed
	// should be able to add validators after initialisation
	assert.NoError(t, staker.AddValidator(0, addr, uint32(360)*24*14, minStake))

	staker = newStaker(t, 101, 101)
	first, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.False(t, first.IsZero())

	expectedLength := big.NewInt(101)
	length, err := staker.leaderGroupSize.Get()
	assert.NoError(t, err)
	assert.True(t, expectedLength.Cmp(length) == 0)
}

func TestStaker_Housekeep_TooEarly(t *testing.T) {
	staker := newStaker(t, 0, 101)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(0, addr1, uint32(360)*24*14, stake))
	assert.NoError(t, staker.ActivateNextValidator())
	assert.NoError(t, staker.AddValidator(0, addr2, uint32(360)*24*15, stake))
	assert.NoError(t, staker.ActivateNextValidator())

	_, err := staker.Housekeep(0, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_Housekeep_ExitOne(t *testing.T) {
	staker := newStaker(t, 0, 101)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(0, addr1, uint32(360)*24*14, stake))
	assert.NoError(t, staker.ActivateNextValidator())
	assert.NoError(t, staker.AddValidator(0, addr2, uint32(360)*24*15, stake))
	assert.NoError(t, staker.ActivateNextValidator())

	_, err := staker.Housekeep(uint32(360)*24*14, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_Housekeep_Cooldown(t *testing.T) {
	staker := newStaker(t, 0, 101)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(0, addr1, uint32(360)*24*14, stake))
	assert.NoError(t, staker.ActivateNextValidator())
	assert.NoError(t, staker.AddValidator(0, addr2, uint32(360)*24*15, stake))
	assert.NoError(t, staker.ActivateNextValidator())

	_, err := staker.Housekeep(uint32(360)*24*15, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
}

func TestStaker_Housekeep_CooldownToExited(t *testing.T) {
	staker := newStaker(t, 0, 101)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(0, addr1, uint32(360)*24*14, stake))
	assert.NoError(t, staker.ActivateNextValidator())
	assert.NoError(t, staker.AddValidator(0, addr2, uint32(360)*24*15, stake))
	assert.NoError(t, staker.ActivateNextValidator())

	_, err := staker.Housekeep(uint32(360)*24*15, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(uint32(360)*24*20, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
}
