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
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
	return a
}

// RandomStake returns a random number between MinStake and (MaxStake/2)
func RandomStake() *big.Int {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	max := big.NewInt(0).Div(MaxStake, big.NewInt(2))

	// Calculate the range (max - MinStake)
	rangeStake := new(big.Int).Sub(max, MinStake)

	// Generate a random number within the range
	randomOffset := new(big.Int).Rand(rng, rangeStake)

	// Add MinStake to ensure the value is within the desired range
	return new(big.Int).Add(MinStake, randomOffset)
}

type keySet struct {
	endorsor thor.Address
	node     thor.Address
}

func createKeys(amount int) map[thor.Address]keySet {
	keys := make(map[thor.Address]keySet)
	for range amount {
		node := datagen.RandAddress()
		endorsor := datagen.RandAddress()

		keys[node] = keySet{
			endorsor: endorsor,
			node:     node,
		}
	}
	return keys
}

func newStaker(t *testing.T, amount int, maxValidators int64, initialise bool) (*Staker, *big.Int) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	keys := createKeys(amount)
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators))

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators)))
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)
	totalStake := big.NewInt(0)
	if initialise {
		for _, key := range keys {
			stake := RandomStake()
			totalStake = totalStake.Add(totalStake, stake)
			if err := staker.AddValidation(key.node, key.endorsor, uint32(360)*24*15, stake); err != nil {
				t.Fatal(err)
			}
		}
		transitioned, err := staker.Transition(0)
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
		{M(stkr.LockedVET()), M(zeroStake, zeroStake, nil)},
		{M(stkr.AddValidation(validator1, validator1, uint32(360)*24*15, stakeAmount)), M(nil)},
		{M(stkr.AddValidation(validator2, validator2, uint32(360)*24*15, stakeAmount)), M(nil)},
		{M(stkr.Transition(0)), M(true, nil)},
		{M(stkr.LockedVET()), M(totalStake, big.NewInt(0).Mul(totalStake, big.NewInt(2)), nil)},
		{M(stkr.AddValidation(validator3, validator3, uint32(360)*24*15, stakeAmount)), M(nil)},
		{M(stkr.FirstQueued()), M(&validator3, nil)},
		{M(func() (*thor.Address, error) {
			activated, err := stkr.activateNextValidation(0, getTestMaxLeaderSize(stkr.params))
			return activated, err
		}()), M(&validator3, nil)},
		{M(stkr.FirstActive()), M(&validator1, nil)},
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
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stakeAmount) // false
		assert.NoError(t, err)
		stakes[addr] = stakeAmount
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		require.NoError(t, err)
		staked, weight, err := staker.LockedVET()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
		assert.Equal(t, big.NewInt(0).Mul(totalStaked, big.NewInt(2)), weight)
	}

	for id, stake := range stakes {
		// todo wrap this up / use a validation service or staker api instead of direct calling and checking totals
		exit, err := staker.validationService.ExitValidator(id)
		require.NoError(t, err)

		// Exit the aggregation too
		aggExit, err := staker.aggregationService.Exit(id)
		require.NoError(t, err)

		// Update global totals
		err = staker.globalStatsService.ApplyExit(exit.Add(aggExit))
		require.NoError(t, err)

		totalStaked = totalStaked.Sub(totalStaked, stake)
		staked, weight, err := staker.LockedVET()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
		assert.Equal(t, big.NewInt(0).Mul(totalStaked, big.NewInt(2)).String(), weight.String())
	}
}

func TestStaker_TotalStake_Withdrawal(t *testing.T) {
	staker, _ := newStaker(t, 0, 14, false)

	addr := datagen.RandAddress()
	stakeAmount := RandomStake()
	period := uint32(360) * 24 * 15
	err := staker.AddValidation(addr, addr, period, stakeAmount)
	assert.NoError(t, err)

	queuedStake, queuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, queuedStake)
	assert.Equal(t, big.NewInt(0).Mul(stakeAmount, big.NewInt(2)), queuedWeight)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr, addr)
	assert.NoError(t, err)

	lockedVET, lockedWeight, err := staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, lockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stakeAmount, big.NewInt(2)), lockedWeight)

	queuedStake, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, 0, queuedStake.Sign())
	assert.Equal(t, 0, queuedWeight.Sign())

	// todo wrap this up / use a validation service or staker api instead of direct calling and checking totals
	exit, err := staker.validationService.ExitValidator(addr)
	require.NoError(t, err)

	// Exit the aggregation too
	aggExit, err := staker.aggregationService.Exit(addr)
	require.NoError(t, err)

	// Update global totals
	err = staker.globalStatsService.ApplyExit(exit.Add(aggExit))
	require.NoError(t, err)

	lockedVET, lockedWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, lockedVET.Sign())
	assert.Equal(t, 0, lockedWeight.Sign())

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stakeAmount, validator.CooldownVET)

	withdrawnAmount, err := staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, withdrawnAmount)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, 0, validator.WithdrawableVET.Sign())

	lockedVET, lockedWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, lockedVET.Sign())
	assert.Equal(t, 0, lockedWeight.Sign())

	queuedStake, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, 0, queuedStake.Sign())
	assert.Equal(t, 0, queuedWeight.Sign())
}

func TestStaker_AddValidation_MinimumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooLow := big.NewInt(0).Sub(MinStake, big.NewInt(1))
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, tooLow)
	assert.ErrorContains(t, err, "stake is out of range")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MaximumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooHigh := big.NewInt(0).Add(MaxStake, big.NewInt(1))
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, tooHigh)
	assert.ErrorContains(t, err, "stake is out of range")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MaxStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MaximumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, MinStake)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MinimumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, MinStake)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), 100, MinStake)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_Duplicate(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.ErrorContains(t, err, "validator already exists")
}

func TestStaker_AddValidation_QueueOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	expectedOrder := [100]thor.Address{}
	// add 100 validations to the queue
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		expectedOrder[i] = addr
	}

	first, err := staker.FirstQueued()
	assert.NoError(t, err)

	// iterating using the `Next` method should return the same order
	loopID := first
	for i := range 100 {
		_, err := staker.validationService.GetValidation(*loopID)
		assert.NoError(t, err)
		assert.Equal(t, expectedOrder[i], *loopID)

		next, err := staker.Next(*loopID)
		assert.NoError(t, err)
		loopID = &next
	}

	// activating validations should continue to set the correct head of the queue
	loopID = first
	for range 99 {
		_, err := staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		first, err = staker.FirstQueued()
		assert.NoError(t, err)
		previous, err := staker.Get(*loopID)
		assert.NoError(t, err)
		current, err := staker.Get(*first)
		assert.NoError(t, err)
		assert.True(t, previous.LockedVET.Cmp(current.LockedVET) >= 0)
		loopID = first
	}
}

func TestStaker_AddValidation(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()
	err := staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr3, addr3, uint32(360)*24*30, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr4, addr4, uint32(360)*24*14, stake)
	assert.Error(t, err, "period is out of boundaries")

	validator, err = staker.Get(addr4)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_QueueUpValidators(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, false)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()
	err := staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	_, _, err = staker.Housekeep(180)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr3, addr3, uint32(360)*24*30, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr3)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr4, addr4, uint32(360)*24*14, stake)
	assert.Error(t, err, "period is out of boundaries")

	validator, err = staker.Get(addr4)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)

	_, _, err = staker.Housekeep(180 * 2)
	assert.NoError(t, err)

	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)
}

func TestStaker_Get_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	id := datagen.RandAddress()
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
}

func TestStaker_Get_FullFlow_Renewal_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)

	active, queued, err := staker.GetValidatorsNum()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), active.String())
	assert.Equal(t, big.NewInt(3), queued)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	active, queued, err = staker.GetValidatorsNum()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), active)
	assert.Equal(t, big.NewInt(2), queued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	active, queued, err = staker.GetValidatorsNum()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), active)
	assert.Equal(t, big.NewInt(1), queued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	active, queued, err = staker.GetValidatorsNum()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(3), active)
	assert.Equal(t, big.NewInt(0).String(), queued.String())

	err = staker.SignalExit(addr, addr)
	assert.NoError(t, err)

	// housekeep the validator
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	active, queued, err = staker.GetValidatorsNum()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), active)
	assert.Equal(t, big.NewInt(0).String(), queued.String())

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
}

func TestStaker_WithdrawQueued(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// verify queued empty
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, *queued)

	// add the validator
	period := uint32(360) * 24 * 15
	err = staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// verify queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, addr, *queued)

	// withraw queued
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)

	// verify removed queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, *queued)
}

func TestStaker_IncreaseQueued(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	stake := RandomStake()

	err := staker.IncreaseStake(addr, thor.Address{}, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	err = staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// increase stake queued
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	err = staker.IncreaseStake(addr, addr, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	newAmount := big.NewInt(0).Add(validator.QueuedVET, validator.LockedVET)
	assert.Equal(t, newAmount, expectedStake)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, validator.Status, validation.StatusQueued)
	assert.Equal(t, validator.QueuedVET, expectedStake)
	assert.Equal(t, big.NewInt(0), validator.Weight)
}

func TestStaker_IncreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	err := staker.IncreaseStake(addr, thor.Address{}, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	err = staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	err = staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, *queued, addr)

	// increase stake queued
	increaseBy := big.NewInt(1000)
	err = staker.IncreaseStake(addr1, addr1, increaseBy)
	assert.NoError(t, err)
	expectedIncreaseStake := big.NewInt(0).Add(stake, increaseBy)

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, *queued, addr)
	entry, err := staker.Get(*queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, big.NewInt(0), entry.Weight)

	queuedAddr, err := staker.Next(*queued)
	assert.NoError(t, err)
	assert.Equal(t, queuedAddr, addr1)
	entry, err = staker.Get(queuedAddr)
	assert.NoError(t, err)
	assert.Equal(t, expectedIncreaseStake, entry.QueuedVET)
	assert.Equal(t, big.NewInt(0), entry.Weight)

	queuedAddr, err = staker.Next(queuedAddr)
	assert.NoError(t, err)
	assert.Equal(t, queuedAddr, addr2)
	entry, err = staker.Get(queuedAddr)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, big.NewInt(0), entry.Weight)
}

func TestStaker_DecreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	err := staker.DecreaseStake(addr, thor.Address{}, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	err = staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	err = staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, *queued, addr)

	// increase stake queued
	decreaseBy := big.NewInt(1000)
	err = staker.DecreaseStake(addr1, addr1, decreaseBy)
	assert.NoError(t, err)

	expectedDecreaseStake := big.NewInt(0).Sub(stake, decreaseBy)

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, *queued, addr)
	entry, err := staker.Get(*queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, big.NewInt(0), entry.Weight)
	next, err := staker.Next(*queued)
	assert.NoError(t, err)
	assert.Equal(t, next, addr1)
	entry, err = staker.Get(next)
	assert.NoError(t, err)
	assert.Equal(t, expectedDecreaseStake, entry.QueuedVET)
	assert.Equal(t, big.NewInt(0), entry.Weight)

	next, err = staker.Next(next)
	assert.NoError(t, err)
	assert.Equal(t, next, addr2)
	entry, err = staker.Get(next)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, big.NewInt(0), entry.Weight)
}

func TestStaker_IncreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// increase stake of an active validator
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	err = staker.IncreaseStake(addr, addr, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.QueuedVET, validator.LockedVET)
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, big.NewInt(0).Add(validator.LockedVET, validator.QueuedVET))
	assert.Equal(t, big.NewInt(0).Mul(validator.LockedVET, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)
}

func TestStaker_ChangeStakeActiveValidatorWithQueued(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	addr := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	// add a second validator
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	validator2, err := staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator2.Status)
	assert.Equal(t, stake, validator2.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator2.Weight)
	queuedVET, queuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	// increase stake of an active validator
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	err = staker.IncreaseStake(addr, addr, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.QueuedVET, validator.LockedVET)
	assert.Equal(t, expectedStake, newStake)

	// the queued stake also increases
	queuedVET, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Add(stake, big.NewInt(1000)), queuedVET)
	assert.Equal(t, big.NewInt(0).Mul(big.NewInt(0).Add(stake, big.NewInt(1000)), big.NewInt(2)), queuedWeight)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, big.NewInt(0).Add(validator.LockedVET, validator.QueuedVET))
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)

	// verify queued stake is still the same as before the increase
	queuedVET, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	// decrease stake
	decreaseAmount := big.NewInt(2500)
	decreasedAmount := big.NewInt(0).Sub(expectedStake, decreaseAmount)
	err = staker.DecreaseStake(addr, addr, decreaseAmount)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, decreaseAmount, validator.PendingUnlockVET)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)

	queuedVET, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	// verify queued weight is decreased
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2500), validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0).Mul(decreasedAmount, big.NewInt(2)), validator.Weight)

	// verify queued stake is still the same as before the decrease
	queuedVET, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)
}

func TestStaker_DecreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStake
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw is empty
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(0))

	// decrease stake of an active validator
	decrease := big.NewInt(1000)
	expectedStake := big.NewInt(0).Sub(stake, decrease)
	err = staker.DecreaseStake(addr, addr, decrease)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.QueuedVET, validator.LockedVET)
	assert.Equal(t, stake, newStake)
	assert.Equal(t, decrease, validator.PendingUnlockVET)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, decrease, validator.PendingUnlockVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)
}

func TestStaker_DecreaseActiveThenExit(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStake
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw is empty
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(0))

	// decrease stake of an active validator
	decrease := big.NewInt(1000)
	expectedStake := big.NewInt(0).Sub(stake, decrease)
	err = staker.DecreaseStake(addr, addr, decrease)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, decrease, validator.PendingUnlockVET)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, decrease, validator.PendingUnlockVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), validator.WithdrawableVET)
	assert.Equal(t, expectedStake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.PendingUnlockVET)

	assert.NoError(t, staker.SignalExit(addr, addr))

	_, _, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(1000), validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, expectedStake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.QueuedVET)
}

func TestStaker_Get_FullFlow(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.SignalExit(addr, addr)
	assert.NoError(t, err)

	// housekeep the validator
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	_, _, err = staker.Housekeep(period + CooldownPeriod.Get())
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
}

func TestStaker_Get_FullFlow_Renewal_On(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// add the validator
	period := uint32(360) * 24 * 15
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// housekeep the validator
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), amount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
}

func TestStaker_Get_FullFlow_Renewal_On_Then_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// housekeep the validator
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), amount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())

	assert.NoError(t, staker.SignalExit(addr, addr))

	// housekeep the validator
	_, _, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.QueuedVET)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period*2+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	// fill 101 validations to leader group
	for range 101 {
		err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, RandomStake())
		assert.NoError(t, err)
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
	}

	// try to add one more to the leadergroup
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, RandomStake())
	assert.NoError(t, err)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.ErrorContains(t, err, "leader group is full")
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)
	_, err := staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.ErrorContains(t, err, "leader group is full")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	_, err := staker.validationService.ExitValidator(datagen.RandAddress())
	assert.NoError(t, err)
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr, addr)
	assert.NoError(t, err)

	// todo wrap this up / use a validation service or staker api instead of direct calling and checking totals
	exit, err := staker.validationService.ExitValidator(addr)
	require.NoError(t, err)

	// Exit the aggregation too
	aggExit, err := staker.aggregationService.Exit(addr)
	require.NoError(t, err)

	// Update global totals
	err = staker.globalStatsService.ApplyExit(exit.Add(aggExit))
	require.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)
	assert.Equal(t, stake, validator.CooldownVET)

	withdrawale, err := staker.WithdrawStake(addr, addr, period)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), withdrawale)

	withdrawale, err = staker.WithdrawStake(addr, addr, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawale)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	stakes := make(map[thor.Address]*big.Int)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		stakes[addr] = stake
	}

	leaderGroup, err := staker.LeaderGroup()
	assert.NoError(t, err)

	for id, stake := range stakes {
		assert.Contains(t, leaderGroup, id)
		assert.Equal(t, stake, leaderGroup[id].LockedVET)
	}
}

func TestStaker_Next_Empty(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	id := datagen.RandAddress()
	next, err := staker.Next(id)
	assert.Nil(t, err)
	assert.Equal(t, thor.Address{}, next)
}

func TestStaker_Next(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	leaderGroup := make([]thor.Address, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		leaderGroup = append(leaderGroup, addr)
	}

	queuedGroup := [100]thor.Address{}
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		queuedGroup[i] = addr
	}

	firstLeader, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, leaderGroup[0], *firstLeader)

	for i := range 99 {
		next, err := staker.Next(leaderGroup[i])
		assert.NoError(t, err)
		assert.Equal(t, leaderGroup[i+1], next)
	}

	firstQueued, err := staker.FirstQueued()
	assert.NoError(t, err)

	current := firstQueued
	for i := range 100 {
		_, err := staker.Get(*current)
		assert.NoError(t, err)
		assert.Equal(t, queuedGroup[i], *current)

		next, err := staker.Next(*current)
		assert.NoError(t, err)
		current = &next
	}
}

func TestStaker_Initialise(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := datagen.RandAddress()

	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(3)))

	for range 3 {
		err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
		assert.NoError(t, err)
	}

	transitioned, err := staker.Transition(0)
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validations after initialisation
	err = staker.AddValidation(addr, addr, uint32(360)*24*15, MinStake)
	assert.NoError(t, err)

	staker, _ = newStaker(t, 101, 101, true)
	first, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.False(t, first.IsZero())

	expectedLength := big.NewInt(101)
	length, err := staker.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.True(t, expectedLength.Cmp(length) == 0)
}

func TestStaker_Housekeep_TooEarly(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	err := staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	_, _, err = staker.Housekeep(0)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
}

func TestStaker_Housekeep_ExitOne(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// Add first validator
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)

	totalLocked, totalWeight, err := staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalLocked.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), totalWeight.Int64())
	assert.Equal(t, stake, totalQueued)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), queuedWeight.Int64())
	assert.Equal(t, stake, totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)).Int64(), totalWeight.Int64())

	// Add second validator
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalWeight)
	assert.Equal(t, stake, totalQueued)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), queuedWeight.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(4)), totalWeight)

	// Add third validtor
	err = staker.AddValidation(addr3, addr3, period, stake)
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(4)), totalWeight)
	assert.Equal(t, stake, totalQueued)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), queuedWeight.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(3)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(6)), totalWeight)

	// disable auto renew
	err = staker.SignalExit(addr1, addr1)
	assert.NoError(t, err)

	// first should be on cooldown
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), queuedWeight.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(4)), totalWeight)

	_, _, err = staker.Housekeep(period + CooldownPeriod.Get())
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), queuedWeight.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(4)).String(), totalWeight.String())
}

func TestStaker_Housekeep_Cooldown(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	period := uint32(360) * 24 * 15

	stake := RandomStake()

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr3, addr3, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew on all validators
	err = staker.SignalExit(addr1, addr1)
	assert.NoError(t, err)
	err = staker.SignalExit(addr2, addr2)
	assert.NoError(t, err)
	err = staker.SignalExit(addr3, addr3)
	assert.NoError(t, err)

	id, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, addr1, *id)
	next, err := staker.Next(*id)
	assert.NoError(t, err)
	assert.Equal(t, addr2, next)

	totalLocked, totalWeight, err := staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(3)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(6)), totalWeight)

	// housekeep and exit validator 1
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	totalLocked, _, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 1, totalLocked.Sign())

	// housekeep and exit validator 2
	_, _, err = staker.Housekeep(period + EpochLength.Get())
	assert.NoError(t, err)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	// housekeep and exit validator 3
	_, _, err = staker.Housekeep(period + EpochLength.Get()*2)
	assert.NoError(t, err)

	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, totalLocked.Sign())
	assert.Equal(t, big.NewInt(0).String(), totalWeight.String())

	withdrawable, err := staker.WithdrawStake(addr1, addr1, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawable)
}

func TestStaker_Housekeep_CooldownToExited(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr3, addr3, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr1, addr1)
	assert.NoError(t, err)
	err = staker.SignalExit(addr2, addr2)
	assert.NoError(t, err)
	err = staker.SignalExit(addr3, addr3)
	assert.NoError(t, err)

	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)

	_, _, err = staker.Housekeep(period + EpochLength.Get())
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
}

func TestStaker_Housekeep_ExitOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr3, addr3, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(period*2, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr2, addr2)
	assert.NoError(t, err)
	err = staker.SignalExit(addr3, addr3)
	assert.NoError(t, err)

	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator2, err := staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator2.Status)
	validator3, err := staker.Get(addr3)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator3.Status)
	assert.NoError(t, err)
	validator1, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator1.Status)

	// renew validator 1 for next period
	_, _, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	assert.NoError(t, staker.SignalExit(addr1, addr1))

	// housekeep -> validator 3 placed intention to leave first
	_, _, err = staker.Housekeep(period * 3)
	assert.NoError(t, err)
	validator3, err = staker.Get(addr3)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator3.Status)
	assert.NoError(t, err)
	validator1, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator1.Status)

	// housekeep -> validator 1 waited 1 epoch after validator 3
	_, _, err = staker.Housekeep(period*3 + EpochLength.Get())
	assert.NoError(t, err)
	validator1, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator1.Status)
}

func TestStaker_Housekeep_RecalculateIncrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MinStake
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.IncreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	// housekeep half way through the period, validator's locked vet should not change
	_, _, err = staker.Housekeep(period / 2)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(1), validator.QueuedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	stake = big.NewInt(0).Add(stake, big.NewInt(1))
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(0))
	assert.Equal(t, validator.QueuedVET, big.NewInt(0))
}

func TestStaker_Housekeep_RecalculateDecrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStake
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	decrease := big.NewInt(1)
	err = staker.DecreaseStake(addr1, addr1, decrease)
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, _, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, decrease, validator.PendingUnlockVET)

	block = uint32(360) * 24 * 15
	_, _, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	expectedStake := big.NewInt(0).Sub(stake, decrease)
	assert.Equal(t, expectedStake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(1))
}

func TestStaker_Housekeep_DecreaseThenWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStake
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, _, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.PendingUnlockVET, big.NewInt(1))

	block = uint32(360) * 24 * 15
	_, _, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	stake = big.NewInt(0).Sub(stake, big.NewInt(1))
	assert.Equal(t, validator.LockedVET, stake)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(1))

	withdrawAmount, err := staker.WithdrawStake(addr1, addr1, block+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, validator.WithdrawableVET, withdrawAmount)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	// verify that validator is still present and active
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	activeValidator, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, addr1, *activeValidator)
}

func TestStaker_DecreaseActive_DecreaseMultipleTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validator.PendingUnlockVET, big.NewInt(1))

	err = staker.DecreaseStake(addr1, addr1, big.NewInt(1))
	assert.NoError(t, err)

	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validator.PendingUnlockVET, big.NewInt(2))

	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, new(big.Int).Sub(stake, big.NewInt(2)), validator.LockedVET)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(2))
	assert.Equal(t, validator.CooldownVET, big.NewInt(0))
}

func TestStaker_Housekeep_Cannot_Exit_If_It_Breaks_Finality(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr1, addr1)
	assert.NoError(t, err)

	exitBlock := uint32(360) * 24 * 15
	_, _, err = staker.Housekeep(exitBlock)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)

	_, _, err = staker.Housekeep(exitBlock + 8640)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)

	err = staker.AddValidation(addr2, addr2, period, stake) // false
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(exitBlock+8640, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidation(addr3, addr3, period, stake) // false
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(exitBlock+8640, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	_, _, err = staker.Housekeep(exitBlock + 8640 + 360)
	assert.NoError(t, err)
	validator, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
}

func TestStaker_Housekeep_Exit_Decrements_Leader_Group_Size(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	newTestSequence(t, staker).
		AddValidation(addr1, addr1, period, stake).
		ActivateNext(0).
		AddValidation(addr2, addr2, period, stake).
		ActivateNext(0).
		SignalExit(addr1, addr1).
		SignalExit(addr2, addr2).
		Housekeep(period).
		AssertLeaderGroupSize(1).
		AssertFirstActive(addr2)

	assertValidation(t, staker, addr1).Status(validation.StatusExit)
	assertValidation(t, staker, addr2).Status(validation.StatusActive)

	block := period + EpochLength.Get()
	newTestSequence(t, staker).
		Housekeep(block).
		AssertLeaderGroupSize(0).
		AssertFirstActive(thor.Address{})

	assertValidation(t, staker, addr2).Status(validation.StatusExit)

	newTestSequence(t, staker).
		AddValidation(addr3, addr3, period, stake).
		ActivateNext(block).
		SignalExit(addr3, addr3).
		AssertFirstActive(addr3).
		AssertLeaderGroupSize(1)

	assertValidation(t, staker, addr3).Status(validation.StatusActive)

	block = block + period
	newTestSequence(t, staker).Housekeep(block)

	assertValidation(t, staker, addr3).Status(validation.StatusExit)
}

func TestStaker_Housekeep_Adds_Queued_Validators_Up_To_Limit(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr3, addr3, period, stake)
	assert.NoError(t, err)

	queuedValidators, err := staker.validationService.QueuedGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(3), queuedValidators)

	leaderGroupSize, err := staker.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), leaderGroupSize.String())

	block := uint32(360) * 24 * 13
	_, _, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	validator1, err := staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator1.Status)
	validator2, err := staker.Get(addr3)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator2.Status)
	leaderGroupSize, err = staker.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), leaderGroupSize)
	queuedValidators, err = staker.validationService.QueuedGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), queuedValidators)
}

func TestStaker_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake) // false
	assert.NoError(t, err)

	withdraw, err := staker.WithdrawStake(addr1, addr1, period)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdraw)

	val, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val.Status)
	assert.Equal(t, big.NewInt(0), val.LockedVET)
	assert.Equal(t, big.NewInt(0), val.Weight)
	assert.Equal(t, big.NewInt(0), val.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), val.QueuedVET)
}

func TestStaker_IncreaseStake_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)

	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)

	val, err := staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, stake, val.LockedVET)

	assert.NoError(t, staker.IncreaseStake(addr1, addr1, big.NewInt(100)))
	withdrawAmount, err := staker.WithdrawStake(addr1, addr1, period+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), withdrawAmount)

	val, err = staker.Get(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, stake, val.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), val.Weight)
	assert.Equal(t, big.NewInt(0), val.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), val.QueuedVET)
}

func TestStaker_GetRewards(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(proposerAddr, proposerAddr, period, stake)
	assert.NoError(t, err)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	amount, err := staker.GetDelegatorRewards(proposerAddr, 1)
	assert.NoError(t, err)
	assert.Equal(t, new(big.Int), amount)

	reward := big.NewInt(1000)
	staker.IncreaseDelegatorsReward(proposerAddr, reward)

	amount, err = staker.GetDelegatorRewards(proposerAddr, 1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), amount)
}

func TestStaker_GetCompletedPeriods(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(proposerAddr, proposerAddr, period, stake)
	assert.NoError(t, err)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	periods, err := staker.GetCompletedPeriods(proposerAddr)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), periods)

	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)

	periods, err = staker.GetCompletedPeriods(proposerAddr)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), periods)
}

func TestStaker_MultipleUpdates_CorrectWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()
	initialStake := RandomStake()
	increases := big.NewInt(0)
	decreases := big.NewInt(0)
	withdrawnTotal := big.NewInt(0)
	thousand := big.NewInt(1000)
	fiveHundred := big.NewInt(500)

	period := uint32(360) * 24 * 15

	// QUEUED
	err := staker.AddValidation(acc, acc, period, initialStake)
	assert.NoError(t, err)

	// assert.NoError(t, staker.SignalExit(acc, id))
	increases.Add(increases, thousand)
	assert.NoError(t, staker.IncreaseStake(acc, acc, thousand))
	// 1st decrease
	decreases.Add(decreases, fiveHundred)
	assert.NoError(t, staker.DecreaseStake(acc, acc, fiveHundred))

	validator, err := staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	// 1st STAKING PERIOD
	_, _, err = staker.Housekeep(period)
	assert.NoError(t, err)

	validator, err = staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	expected := new(big.Int).Sub(initialStake, decreases)
	expected = expected.Add(expected, increases)
	assert.Equal(t, expected, validator.LockedVET)

	// See `1st decrease` -> validator should be able withdraw the decrease amount
	withdraw, err := staker.WithdrawStake(acc, acc, period+1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw, fiveHundred)
	withdrawnTotal = withdrawnTotal.Add(withdrawnTotal, withdraw)

	expectedLocked := new(big.Int).Sub(initialStake, decreases)
	expectedLocked = expectedLocked.Add(expectedLocked, increases)
	validator, err = staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, expectedLocked, validator.LockedVET)

	// 2nd decrease
	decreases.Add(decreases, thousand)
	assert.NoError(t, staker.DecreaseStake(acc, acc, thousand))
	increases.Add(increases, fiveHundred)
	assert.NoError(t, staker.IncreaseStake(acc, acc, fiveHundred))

	// 2nd STAKING PERIOD
	_, _, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	validator, err = staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)

	// See `2nd decrease` -> validator should be able withdraw the decrease amount
	withdraw, err = staker.WithdrawStake(acc, acc, period*2+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, thousand, withdraw)
	withdrawnTotal = withdrawnTotal.Add(withdrawnTotal, withdraw)

	assert.NoError(t, staker.SignalExit(acc, acc))

	// EXITED
	_, _, err = staker.Housekeep(period * 3)
	assert.NoError(t, err)

	validator, err = staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	expectedLocked = new(big.Int).Sub(initialStake, decreases)
	expectedLocked = expectedLocked.Add(expectedLocked, increases)
	validator, err = staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, expectedLocked, validator.CooldownVET)

	withdraw, err = staker.WithdrawStake(acc, acc, period*3+CooldownPeriod.Get())
	assert.NoError(t, err)
	withdrawnTotal.Add(withdrawnTotal, withdraw)
	depositTotal := new(big.Int).Add(initialStake, increases)
	assert.Equal(t, depositTotal, withdrawnTotal)
}

func Test_GetValidatorTotals_ValidatorExiting(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	vStake := validation.WeightedStake(validators[0].LockedVET)
	dStake := stakes.NewWeightedStake(MinStake, 255)

	delegationID := new(big.Int)
	newTestSequence(t, staker).AddDelegation(validator.ID, dStake.VET(), 255, delegationID)

	newTestSequence(t, staker).AssertTotals(validator.ID, &validation.Totals{
		TotalQueuedWeight: dStake.Weight(),
		TotalQueuedStake:  dStake.VET(),
		TotalLockedWeight: vStake.Weight(),
		TotalLockedStake:  vStake.VET(),
	})

	newTestSequence(t, staker).
		Housekeep(validator.Period).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalLockedWeight: big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
		}).
		SignalExit(validator.ID, validator.Endorsor).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:   big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalLockedWeight:  big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
			TotalExitingStake:  big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalExitingWeight: big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
		})
}

func Test_GetValidatorTotals_DelegatorExiting_ThenValidator(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	vStake := validation.WeightedStake(validators[0].LockedVET)
	dStake := stakes.NewWeightedStake(MinStake, 255)

	delegationID := new(big.Int)
	newTestSequence(t, staker).AddDelegation(validator.ID, dStake.VET(), 255, delegationID)

	newTestSequence(t, staker).AssertTotals(validator.ID, &validation.Totals{
		TotalQueuedWeight: dStake.Weight(),
		TotalQueuedStake:  dStake.VET(),
		TotalLockedWeight: vStake.Weight(),
		TotalLockedStake:  vStake.VET(),
	})

	newTestSequence(t, staker).
		Housekeep(validator.Period).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalLockedWeight: big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
			TotalQueuedStake:  big.NewInt(0),
			TotalQueuedWeight: big.NewInt(0),
		}).
		SignalDelegationExit(delegationID).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:   big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalLockedWeight:  big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
			TotalQueuedStake:   big.NewInt(0),
			TotalQueuedWeight:  big.NewInt(0),
			TotalExitingStake:  dStake.VET(),
			TotalExitingWeight: dStake.Weight(),
		}).
		SignalExit(validator.ID, validator.Endorsor).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:   big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalLockedWeight:  big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
			TotalExitingStake:  big.NewInt(0).Add(vStake.VET(), dStake.VET()),
			TotalExitingWeight: big.NewInt(0).Add(vStake.Weight(), dStake.Weight()),
		}).
		Housekeep(validator.Period*2).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:   big.NewInt(0),
			TotalLockedWeight:  big.NewInt(0),
			TotalExitingStake:  big.NewInt(0),
			TotalExitingWeight: big.NewInt(0),
		})
}

func Test_Validator_Decrease_Exit_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()

	originalStake := big.NewInt(0).Mul(big.NewInt(3), MinStake)
	err := staker.AddValidation(acc, acc, LowStakingPeriod.Get(), originalStake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// Decrease stake
	decrease := big.NewInt(0).Mul(big.NewInt(2), MinStake)
	err = staker.DecreaseStake(acc, acc, decrease)
	assert.NoError(t, err)

	// Turn off auto-renew  - can't decrease if auto-renew is false
	err = staker.SignalExit(acc, acc)
	assert.NoError(t, err)

	// Housekeep, should exit the validator
	_, _, err = staker.Housekeep(LowStakingPeriod.Get())
	assert.NoError(t, err)

	validator, err := staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, originalStake, validator.CooldownVET)
}

func Test_Validator_Decrease_SeveralTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()

	originalStake := big.NewInt(0).Mul(big.NewInt(3), MinStake)
	err := staker.AddValidation(acc, acc, LowStakingPeriod.Get(), originalStake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// Decrease stake - ok 75m - 25m = 50m
	err = staker.DecreaseStake(acc, acc, MinStake)
	assert.NoError(t, err)

	// Decrease stake - ok 50m - 25m = 25m
	err = staker.DecreaseStake(acc, acc, MinStake)
	assert.NoError(t, err)

	// Decrease stake - should fail, min stake is 25m
	err = staker.DecreaseStake(acc, acc, MinStake)
	assert.ErrorContains(t, err, "next period stake is too low for validator")
}

func Test_Validator_IncreaseDecrease_Combinations(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	acc := datagen.RandAddress()

	// Add & activate validator
	err := staker.AddValidation(acc, acc, LowStakingPeriod.Get(), MinStake)
	assert.NoError(t, err)

	// Increase and decrease - both should be okay since we're only dealing with QueuedVET
	assert.NoError(t, staker.IncreaseStake(acc, acc, MinStake)) // 25m + 25m = 50m
	assert.NoError(t, staker.DecreaseStake(acc, acc, MinStake)) // 25m - 50m = 25m

	// Activate the validator.
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// Withdraw the previous decrease amount
	withdrawal, err := staker.WithdrawStake(acc, acc, 0)
	assert.NoError(t, err)
	assert.Equal(t, MinStake, withdrawal, "withdraw should be 0 since we are withdrawing from pending locked")

	// Assert previous increase/decrease had no effect since they requested the same amount
	val, err := staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, MinStake, val.LockedVET)

	// Increase stake (ok): 25m + 25m = 50m
	assert.NoError(t, staker.IncreaseStake(acc, acc, MinStake))
	// Decrease stake (NOT ok): 25m - 25m = 0. The Previous increase is not applied since it is still currently withdrawable.
	assert.ErrorContains(t, staker.DecreaseStake(acc, acc, MinStake), "next period stake is too low for validator")
	// Instantly withdraw - This is bad, it pulls from the QueuedVET, which means total stake later will be 0.
	// The decrease previously marked as okay since the current TVL + pending TVL was greater than the min stake.
	withdraw1, err := staker.WithdrawStake(acc, acc, 0)
	assert.NoError(t, err)
	assert.Equal(t, MinStake, withdraw1, "withdraw should be 0 since we are withdrawing from pending locked")

	// Housekeep, should move pending locked to locked, and pending withdraw to withdrawable
	_, _, err = staker.Housekeep(LowStakingPeriod.Get())
	assert.NoError(t, err)

	// Withdraw again
	withdraw2, err := staker.WithdrawStake(acc, acc, LowStakingPeriod.Get()+CooldownPeriod.Get())
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), withdraw2)

	validator, err := staker.Get(acc)
	assert.NoError(t, err)
	assert.Equal(t, 0, validator.LockedVET.Cmp(MinStake), "locked vet should be greater than or equal to min stake")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMaster(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	address := datagen.RandAddress()
	err := staker.AddValidation(address, datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)

	err = staker.AddValidation(address, datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.Error(t, err, "validator already exists")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMasterAfterExit(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	master := datagen.RandAddress()
	endorsor := datagen.RandAddress()
	err := staker.AddValidation(master, endorsor, uint32(360)*24*15, MinStake)
	assert.NoError(t, err)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.SignalExit(master, endorsor)
	assert.NoError(t, err)

	_, err = staker.validationService.ExitValidator(master)
	assert.NoError(t, err)

	err = staker.AddValidation(master, datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.Error(t, err, "validator already exists")
}

func TestStaker_HasDelegations(t *testing.T) {
	staker, _ := newStaker(t, 1, 1, true)

	validator, err := staker.FirstActive()
	assert.NoError(t, err)
	dStake := delegationStake()
	stakingPeriod := MediumStakingPeriod.Get()

	delegationID := big.NewInt(0)
	newTestSequence(t, staker).
		// no delegations, should be false
		AssertHasDelegations(*validator, false).
		// delegation added, housekeeping not performed, should be false
		AddDelegation(*validator, dStake, 200, delegationID).
		AssertHasDelegations(*validator, false).
		// housekeeping performed, should be true
		Housekeep(stakingPeriod).
		AssertHasDelegations(*validator, true).
		// signal exit, housekeeping not performed, should still be true
		SignalDelegationExit(delegationID).
		AssertHasDelegations(*validator, true).
		// housekeeping performed, should be false
		Housekeep(stakingPeriod*2).
		AssertHasDelegations(*validator, false)
}

func TestStaker_SetBeneficiary(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	master := datagen.RandAddress()
	endorsor := datagen.RandAddress()
	beneficiary := datagen.RandAddress()

	testSetup := newTestSequence(t, staker)

	// add validation without a beneficiary
	testSetup.AddValidation(master, endorsor, MediumStakingPeriod.Get(), MinStake).ActivateNext(0)
	assertValidation(t, staker, master).Beneficiary(nil)

	// negative cases
	assert.ErrorContains(t, staker.SetBeneficiary(master, master, beneficiary), "invalid endorser")
	assert.ErrorContains(t, staker.SetBeneficiary(endorsor, endorsor, beneficiary), "failed to get validator")

	// set beneficiary, should be successful
	testSetup.SetBeneficiary(master, endorsor, beneficiary)
	assertValidation(t, staker, master).Beneficiary(&beneficiary)

	// remove the beneficiary
	testSetup.SetBeneficiary(master, endorsor, thor.Address{})
	assertValidation(t, staker, master).Beneficiary(nil)
}

func getTestMaxLeaderSize(param *params.Params) *big.Int {
	maxLeaderGroupSize, err := param.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		panic(err)
	}
	return maxLeaderGroupSize
}
