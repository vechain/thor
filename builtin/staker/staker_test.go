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
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Calculate the range (maxStake - minStake)
	rangeStake := new(big.Int).Sub(maxStake, minStake)

	// Generate a random number within the range
	randomOffset := new(big.Int).Rand(rng, rangeStake)

	// Add minStake to ensure the value is within the desired range
	return new(big.Int).Add(minStake, randomOffset)
}

type keySet struct {
	endorsor thor.Address
	master   thor.Address
}

func addAuthorities(t *testing.T, auth *authority.Authority, authorities int) map[thor.Address]keySet {
	keys := make(map[thor.Address]keySet)
	for range authorities {
		nodeMaster := datagen.RandAddress()
		endorsor := datagen.RandAddress()

		keys[nodeMaster] = keySet{
			endorsor: endorsor,
			master:   nodeMaster,
		}

		identity := datagen.RandomHash()

		ok, err := auth.Add(nodeMaster, endorsor, identity)
		assert.NoError(t, err)
		assert.True(t, ok)
	}
	return keys
}

func newStaker(t *testing.T, authorities int, maxValidators int64, initialise bool) (*Staker, *big.Int) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	auth := authority.New(thor.BytesToAddress([]byte("auth")), st)
	keys := addAuthorities(t, auth, authorities)
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators))
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators)))
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param)
	totalStake := big.NewInt(0)
	if initialise {
		for _, key := range keys {
			stake := RandomStake()
			totalStake = totalStake.Add(totalStake, stake)
			assert.Nil(t, staker.AddValidator(key.endorsor, key.master, uint32(360)*24*14, stake, true))
		}
		transitioned, err := staker.Transition()
		assert.NoError(t, err)
		assert.True(t, transitioned)
	}
	return staker, totalStake
}

func TestStaker(t *testing.T) {
	validator1 := thor.BytesToAddress([]byte("v1"))
	validator2 := thor.BytesToAddress([]byte("v2"))
	validator3 := thor.BytesToAddress([]byte("v3"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	zeroStake := big.NewInt(0).SetBytes(thor.Bytes32{}.Bytes())

	stkr, _ := newStaker(t, 0, 3, false)

	totalStake := big.NewInt(0).Mul(stakeAmount, big.NewInt(2))

	tests := []struct {
		ret      any
		expected any
	}{
		{M(stkr.TotalStake()), M(zeroStake, nil)},
		{stkr.AddValidator(validator1, validator1, uint32(360)*24*14, stakeAmount, true), nil},
		{stkr.AddValidator(validator2, validator2, uint32(360)*24*14, stakeAmount, true), nil},
		{M(stkr.Transition()), M(true, nil)},
		{M(stkr.TotalStake()), M(totalStake, nil)},
		{stkr.AddValidator(validator3, validator3, uint32(360)*24*14, stakeAmount, true), nil},
		{M(stkr.FirstQueued()), M(validator3, nil)},
		{M(stkr.ActivateNextValidator(0)), M(nil)},
		{M(stkr.FirstActive()), M(validator1, nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestStaker_TotalStake(t *testing.T) {
	staker, totalStaked := newStaker(t, 0, 14, false)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		stakes[addr] = stakeAmount
		assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stakeAmount, false))
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
		assert.NoError(t, staker.ActivateNextValidator(0))
		staked, err := staker.TotalStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}

	for addr, stake := range stakes {
		assert.NoError(t, staker.RemoveValidator(addr, uint32(360)*24*14*2))
		totalStaked = totalStaked.Sub(totalStaked, stake)
		staked, err := staker.TotalStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}
}

func TestStaker_AddValidator_MinimumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooLow := big.NewInt(0).Sub(minStake, big.NewInt(1))
	assert.ErrorContains(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, tooLow, true), "stake is out of range")
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true))
}

func TestStaker_AddValidator_MaximumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooHigh := big.NewInt(0).Add(maxStake, big.NewInt(1))
	assert.ErrorContains(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, tooHigh, true), "stake is out of range")
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, maxStake, true))
}

func TestStaker_AddValidator_MaximumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	assert.ErrorContains(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, minStake, true), "period is out of boundaries")
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true))
}

func TestStaker_AddValidator_MinimumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	assert.ErrorContains(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, minStake, true), "period is out of boundaries")
	assert.ErrorContains(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), 100, minStake, true), "period is out of boundaries")
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true))
}

func TestStaker_AddValidator_Duplicate(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
	assert.ErrorContains(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true), "validator already exists")
}

func TestStaker_AddValidator_QueueOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	// add 100 validators to the queue
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
	}

	first, err := staker.FirstQueued()
	assert.NoError(t, err)

	// iterating using the `Next` method should return the same order
	loopAddr := first
	for range 99 {
		next, err := staker.Next(loopAddr)
		assert.NoError(t, err)
		loopVal, err := staker.validatorQueue.linkedList.validators.Get(loopAddr)
		assert.NoError(t, err)
		nextVal, err := staker.validatorQueue.linkedList.validators.Get(next)
		assert.NoError(t, err)
		assert.True(t, loopVal.Stake.Cmp(nextVal.Stake) >= 0)
		loopAddr = next
	}

	// activating validators should continue to set the correct head of the queue
	loopAddr = first
	for range 99 {
		assert.NoError(t, staker.ActivateNextValidator(0))
		first, err = staker.FirstQueued()
		assert.NoError(t, err)
		previous, err := staker.validatorQueue.linkedList.validators.Get(loopAddr)
		assert.NoError(t, err)
		current, err := staker.validatorQueue.linkedList.validators.Get(first)
		assert.NoError(t, err)
		assert.True(t, previous.Stake.Cmp(current.Stake) >= 0)
		loopAddr = first
	}
}

func TestStaker_AddValidator(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, true))

	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)

	assert.NoError(t, staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, true))

	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)

	assert.NoError(t, staker.AddValidator(addr3, addr3, uint32(360)*24*30, stake, true))

	validator, err = staker.Get(addr3)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)

	assert.Error(t, staker.AddValidator(addr4, addr4, uint32(360)*24*15, stake, true), "period is out of boundaries")

	validator, err = staker.Get(addr4)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	addr := datagen.RandAddress()
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, StatusQueued, validator.Status)

	assert.NoError(t, staker.ActivateNextValidator(0))

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
}

func TestStaker_Get_FullFlow_Renewal_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, period, stake, false))
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// activate the validator
	assert.NoError(t, staker.ActivateNextValidator(0))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)
	assert.NoError(t, staker.ActivateNextValidator(0))

	// housekeep the validator
	_, err = staker.Housekeep(period+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)
	withdraw, err := staker.GetWithdraw(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdraw.Amount)
	assert.True(t, withdraw.Available)

	// remove the validator
	assert.NoError(t, staker.RemoveValidator(addr, period+100))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
	withdraw, err = staker.GetWithdraw(addr)
	assert.NoError(t, err)
	assert.True(t, withdraw.Endorsor.IsZero())
}

func TestStaker_WithdrawQueued(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// verify queued empty
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, queued)

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// verify queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, addr, queued)

	// withraw queued
	withdrawAmount, err := staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())

	withdraw, err := staker.GetWithdraw(addr)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, thor.Address{})

	// verify removed queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, queued)
}

func TestStaker_IncreaseQueued(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	stake := RandomStake()

	_, err := staker.IncreaseStake(addr, addr, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// increase stake queued
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	newAmount, err := staker.IncreaseStake(addr, addr, big.NewInt(1000))
	assert.NoError(t, err)
	assert.Equal(t, newAmount, expectedStake)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, validator.Status, StatusQueued)
	assert.Equal(t, validator.Stake, expectedStake)
}

func TestStaker_IncreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	stake := RandomStake()

	_, err := staker.IncreaseStake(addr, addr, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	assert.NoError(t, staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, false))

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, addr)

	// increase stake queued
	_, err = staker.IncreaseStake(addr1, addr1, big.NewInt(1000))
	assert.NoError(t, err)

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, addr1)
}

func TestStaker_IncreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// increase stake of an active validator
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	newStake, err := staker.IncreaseStake(addr, addr, big.NewInt(1000))
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)
}
func TestStaker_DereaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := maxStake

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw is empty
	withdraw, err := staker.GetWithdraw(addr)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, thor.Address{})
	assert.Equal(t, withdraw.Available, false)

	// increase stake of an active validator
	expectedStake := big.NewInt(0).Sub(stake, big.NewInt(1000))
	newStake, err := staker.DecreaseStake(addr, addr, big.NewInt(1000))
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw amount increase
	withdraw, err = staker.GetWithdraw(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), withdraw.Amount)
	assert.Equal(t, false, withdraw.Available)
}

func TestStaker_Get_FullFlow(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, period, stake, false))
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// activate the validator
	assert.NoError(t, staker.ActivateNextValidator(0))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)
	assert.NoError(t, staker.ActivateNextValidator(0))

	// housekeep the validator
	_, err = staker.Housekeep(period+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// remove the validator
	assert.NoError(t, staker.RemoveValidator(addr, period+100))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get_FullFlow_Renewal_On(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// activate the validator
	assert.NoError(t, staker.ActivateNextValidator(0))
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
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// remove the validator
	assert.Error(t, staker.RemoveValidator(addr, 100), "validator cannot be removed")
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// withdraw the stake
	_, err = staker.WithdrawStake(addr, addr)
	assert.Error(t, err, "validator is not inactive")
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
}

func TestStaker_Get_FullFlow_Renewal_On_Then_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
	assert.NoError(t, staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, true))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// activate the validator
	assert.NoError(t, staker.ActivateNextValidator(0))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)
	assert.NoError(t, staker.ActivateNextValidator(0))

	// housekeep the validator
	_, err = staker.Housekeep(period+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// remove the validator
	assert.Error(t, staker.RemoveValidator(addr, 100), "validator cannot be removed")
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// withdraw the stake
	_, err = staker.WithdrawStake(addr, addr)
	assert.Error(t, err, "validator is not inactive")
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())

	assert.NoError(t, staker.UpdateAutoRenew(addr, addr, false, 0))

	// housekeep the validator
	_, err = staker.Housekeep(period*2+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// remove the validator
	assert.NoError(t, staker.RemoveValidator(addr, 100))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get_FullFlow_Renewal_Off_Then_On(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	assert.NoError(t, staker.AddValidator(addr, addr, period, stake, false))
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// activate the validator
	assert.NoError(t, staker.ActivateNextValidator(0))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)
	assert.NoError(t, staker.ActivateNextValidator(0))

	assert.NoError(t, staker.UpdateAutoRenew(addr, addr, true, 0))

	// housekeep the validator
	_, err = staker.Housekeep(period+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// remove the validator
	assert.Error(t, staker.RemoveValidator(addr, 100), "validator cannot be removed")
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.Stake)
	assert.Equal(t, stake, validator.Weight)

	// withdraw the stake
	_, err = staker.WithdrawStake(addr, addr)
	assert.Error(t, err, "validator is not inactive")
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())

	assert.NoError(t, staker.UpdateAutoRenew(addr, addr, false, 0))

	// housekeep the validator
	_, err = staker.Housekeep(period*2+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// remove the validator
	assert.NoError(t, staker.RemoveValidator(addr, 100))
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	// fill 101 validators to leader group
	for range 101 {
		assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, RandomStake(), true))
		assert.NoError(t, staker.ActivateNextValidator(0))
	}

	// try to add one more to the leadergroup
	assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, RandomStake(), true))
	assert.ErrorContains(t, staker.ActivateNextValidator(0), "leader group is full")
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)
	assert.ErrorContains(t, staker.ActivateNextValidator(0), "leader group is full")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	addr := datagen.RandAddress()
	assert.NoError(t, staker.RemoveValidator(addr, 100))
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	assert.NoError(t, staker.AddValidator(addr, addr, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.RemoveValidator(addr, period+1))

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)

	withdraw, err := staker.GetWithdraw(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdraw.Amount)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	stakes := make(map[thor.Address]*big.Int)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
		assert.NoError(t, staker.ActivateNextValidator(0))
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
	staker, _ := newStaker(t, 101, 101, true)

	addr := datagen.RandAddress()
	next, err := staker.Next(addr)
	assert.Nil(t, err)
	assert.Equal(t, thor.Address{}, next)
}

func TestStaker_Next(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	leaderGroup := make([]thor.Address, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
		assert.NoError(t, staker.ActivateNextValidator(0))
		leaderGroup = append(leaderGroup, addr)
	}

	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true))
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

	current := firstQueued
	for range 99 {
		next, err := staker.Next(current)
		assert.NoError(t, err)
		currentVal, err := staker.validatorQueue.linkedList.validators.Get(current)
		assert.NoError(t, err)
		nextVal, err := staker.validatorQueue.linkedList.validators.Get(next)
		assert.NoError(t, err)
		assert.True(t, currentVal.Stake.Cmp(nextVal.Stake) >= 0)
		current = next
	}
}

func TestStaker_GetStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	balance, err := staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Nil(t, balance)

	assert.NoError(t, staker.AddValidator(addr, addr, period, stake, false))
	balance, err = staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, balance)

	assert.NoError(t, staker.ActivateNextValidator(0))
	balance, err = staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, balance)

	assert.NoError(t, staker.RemoveValidator(addr, period+1))
	balance, err = staker.GetStake(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), balance)
}

func TestStaker_WithdrawStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	period := uint32(360) * 24 * 14

	withdrawAmount, err := staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.True(t, withdrawAmount.Sign() == 0)

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(addr, addr, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))
	withdrawAmount, err = staker.WithdrawStake(addr, addr)
	assert.ErrorContains(t, err, "validator is not inactive")
	assert.Nil(t, withdrawAmount)

	_, err = staker.Housekeep(period+1, 0)
	assert.Nil(t, err)
	assert.NoError(t, staker.RemoveValidator(addr, period+1))
	withdrawAmount, err = staker.WithdrawStake(addr, addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
}

func TestStaker_Initialise(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := datagen.RandAddress()

	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param)
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(3)))

	for range 3 {
		assert.NoError(t, staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true))
	}

	transitioned, err := staker.Transition()
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validators after initialisation
	assert.NoError(t, staker.AddValidator(addr, addr, uint32(360)*24*14, minStake, true))

	staker, _ = newStaker(t, 101, 101, true)
	first, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.False(t, first.IsZero())

	expectedLength := big.NewInt(101)
	length, err := staker.leaderGroupSize.Get()
	assert.NoError(t, err)
	assert.True(t, expectedLength.Cmp(length) == 0)
}

func TestStaker_Housekeep_TooEarly(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

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
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr2, addr2, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

	_, err := staker.Housekeep(period+cooldownPeriod, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_Housekeep_Cooldown(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	period := uint32(360) * 24 * 14

	stake := RandomStake()

	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr2, addr2, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))

	_, err := staker.Housekeep(uint32(360)*24*15, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.Stake)
	withdraw, err := staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.True(t, withdraw.Available)

	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, stake, validator.Stake)
}

func TestStaker_Housekeep_CooldownToExited(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr2, addr2, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))

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

func TestStaker_Housekeep_ExitOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr2, addr2, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr3, addr3, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(10))

	assert.NoError(t, staker.UpdateAutoRenew(addr1, addr1, false, uint32(360)*24*12))

	exitBlock := uint32(360) * 24 * 15
	_, err := staker.Housekeep(exitBlock, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(addr3)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock+2, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(addr3)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock+360, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(addr3)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)

	_, err = staker.Housekeep(exitBlock+362, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock+720, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
}

func TestStaker_Housekeep_Cannot_Exit_Without_Cooldown(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))
	assert.NoError(t, staker.AddValidator(addr2, addr2, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))

	exitBlock := uint32(360) * 24 * 14
	_, err := staker.Housekeep(exitBlock, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock+8639, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock+8640, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
}

func TestStaker_Housekeep_RecalculateIncrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := minStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

	_, err := staker.IncreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.Stake, big.NewInt(0).Add(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, stake)

	block = uint32(360) * 24 * 14
	_, err = staker.Housekeep(block, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.Stake, big.NewInt(0).Add(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, big.NewInt(0).Add(stake, big.NewInt(1)))
}

func TestStaker_Housekeep_RecalculateDecrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := maxStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

	_, err := staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.Stake, big.NewInt(0).Sub(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, stake)

	withdraw, err := staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, false)
	assert.Equal(t, withdraw.Amount, big.NewInt(1))

	block = uint32(360) * 24 * 14
	_, err = staker.Housekeep(block, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.Stake, big.NewInt(0).Sub(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, big.NewInt(0).Sub(stake, big.NewInt(1)))

	withdraw, err = staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, true)
	assert.Equal(t, withdraw.Amount, big.NewInt(1))
}

func TestStaker_Housekeep_DecreaseThenWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := maxStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

	_, err := staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.Stake, big.NewInt(0).Sub(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, stake)

	withdraw, err := staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, false)
	assert.Equal(t, withdraw.Amount, big.NewInt(1))

	block = uint32(360) * 24 * 14
	_, err = staker.Housekeep(block, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.Stake, big.NewInt(0).Sub(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, big.NewInt(0).Sub(stake, big.NewInt(1)))

	withdraw, err = staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, true)
	assert.Equal(t, withdraw.Amount, big.NewInt(1))

	withdrawAmount, err := staker.WithdrawStake(addr1, addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, true)
	assert.Equal(t, withdraw.Amount, withdrawAmount)

	withdraw, err = staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, thor.Address{})
	assert.Equal(t, withdraw.Available, false)

	// verify that validator is still present and active
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1)), validator.Stake)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1)), validator.Weight)
	activeValidator, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, addr1, activeValidator)
}

func TestStaker_DecreaseActive_DecreaseMultipleTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := maxStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, true))
	assert.NoError(t, staker.ActivateNextValidator(0))

	_, err := staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1)), validator.Stake)
	withdraw, err := staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, false)
	assert.Equal(t, withdraw.Amount, big.NewInt(1))

	_, err = staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(2)), validator.Stake)
	withdraw, err = staker.GetWithdraw(addr1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw.Endorsor, addr1)
	assert.Equal(t, withdraw.Available, false)
	assert.Equal(t, withdraw.Amount, big.NewInt(2))
}

func TestStaker_Housekeep_Cannot_Exit_If_It_Breaks_Finality(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	assert.NoError(t, staker.AddValidator(addr1, addr1, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(0))

	exitBlock := uint32(360) * 24 * 14
	_, err := staker.Housekeep(exitBlock, 0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock+8640, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	assert.NoError(t, staker.AddValidator(addr2, addr2, period, stake, false))
	assert.NoError(t, staker.ActivateNextValidator(exitBlock+8640))

	_, err = staker.Housekeep(exitBlock+8640+360, 0)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
}
