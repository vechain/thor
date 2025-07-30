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
			if err := staker.AddValidator(key.endorsor, key.node, uint32(360)*24*15, stake); err != nil {
				t.Fatal(err)
			}
		}
		transitioned, err := staker.Transition(0)
		assert.NoError(t, err)
		assert.True(t, transitioned)
	}
	return staker, totalStake
}

func TestStaker_TotalStake(t *testing.T) {
	staker, totalStaked := newStaker(t, 0, 14, false)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		err := staker.AddValidator(addr, addr, uint32(360)*24*15, stakeAmount) // false
		require.NoError(t, err)
		stakes[addr] = stakeAmount
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
		_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
		require.NoError(t, err)
		staked, weight, err := staker.LockedVET()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
		assert.Equal(t, big.NewInt(0).Mul(totalStaked, big.NewInt(2)), weight)
	}

	for id, stake := range stakes {
		// todo wrap this up / use a validation service or staker api instead of direct calling and checking totals
		releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, err := staker.validationService.ExitValidator(id)
		require.NoError(t, err)

		// Exit the aggregation too
		aggExit, err := staker.aggregationService.Exit(id)
		require.NoError(t, err)

		// Update global totals
		err = staker.globalStatsService.RemoveLocked(releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, aggExit)
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

	NewSequence(staker).
		AddValidator(addr, addr, period, stakeAmount).
		AssertQueuedVET(stakeAmount, big.NewInt(0).Mul(stakeAmount, big.NewInt(2))).
		ActivateNext(0).
		SignalExit(addr, addr).
		AssertLockedVET(stakeAmount, big.NewInt(0).Mul(stakeAmount, big.NewInt(2))).
		AssertQueuedVET(big.NewInt(0), big.NewInt(0)).
		Run(t)

	// todo wrap this up / use a validation service or staker api instead of direct calling and checking totals
	releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, err := staker.validationService.ExitValidator(addr)
	require.NoError(t, err)

	// Exit the aggregation too
	aggExit, err := staker.aggregationService.Exit(addr)
	require.NoError(t, err)

	// Update global totals
	err = staker.globalStatsService.RemoveLocked(releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, aggExit)
	require.NoError(t, err)

	NewSequence(staker).AssertLockedVET(big.NewInt(0), big.NewInt(0)).Run(t)
	AssertValidator(t, staker, addr).Status(validation.StatusExit).CooldownVET(stakeAmount)

	NewSequence(staker).WithdrawStake(addr, addr, period+cooldownPeriod, stakeAmount).Run(t)
	AssertValidator(t, staker, addr).Status(validation.StatusExit).WithdrawableVET(big.NewInt(0))

	NewSequence(staker).
		AssertLockedVET(big.NewInt(0), big.NewInt(0)).
		AssertQueuedVET(big.NewInt(0), big.NewInt(0)).
		Run(t)
}

func TestStaker_AddValidator_MinimumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooLow := big.NewInt(0).Sub(MinStake, big.NewInt(1))
	err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, tooLow)
	assert.ErrorContains(t, err, "stake is out of range")
	err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_MaximumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooHigh := big.NewInt(0).Add(MaxStake, big.NewInt(1))
	err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, tooHigh)
	assert.ErrorContains(t, err, "stake is out of range")
	err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MaxStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_MaximumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, MinStake)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_MinimumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, MinStake)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), 100, MinStake)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_Duplicate(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	err := staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	err = staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
	assert.ErrorContains(t, err, "validator already exists")
}

func TestStaker_AddValidator_QueueOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	expectedOrder := [100]thor.Address{}
	// add 100 validations to the queue
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
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
		_, err := staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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

func TestStaker_AddValidator(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()

	NewSequence(staker).
		AddValidator(addr1, addr1, uint32(360)*24*15, stake).
		AddValidator(addr2, addr2, uint32(360)*24*15, stake).
		AddValidator(addr3, addr3, uint32(360)*24*30, stake).
		Run(t)

	AssertValidator(t, staker, addr1).
		IsEmpty(false).
		PendingLocked(stake).
		Status(validation.StatusQueued)

	AssertValidator(t, staker, addr2).
		IsEmpty(false).
		PendingLocked(stake).
		Status(validation.StatusQueued)

	AssertValidator(t, staker, addr3).
		IsEmpty(false).
		PendingLocked(stake).
		Status(validation.StatusQueued)

	// Error case - keep as is since framework doesn't handle errors
	err := staker.AddValidator(addr4, addr4, uint32(360)*24*14, stake)
	assert.Error(t, err, "period is out of boundaries")

	AssertValidator(t, staker, addr4).IsEmpty(true)
}

func TestStaker_QueueUpValidators(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, false)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()

	NewSequence(staker).
		AddValidator(addr1, addr1, uint32(360)*24*15, stake).
		Run(t)

	AssertValidator(t, staker, addr1).
		IsEmpty(false).
		PendingLocked(stake).
		Status(validation.StatusQueued)

	NewSequence(staker).Housekeep(180).Run(t)

	AssertValidator(t, staker, addr1).
		IsEmpty(false).
		LockedVET(stake).
		Status(validation.StatusActive)

	NewSequence(staker).
		AddValidator(addr2, addr2, uint32(360)*24*15, stake).
		AddValidator(addr3, addr3, uint32(360)*24*30, stake).
		Run(t)

	AssertValidator(t, staker, addr2).
		IsEmpty(false).
		PendingLocked(stake).
		Status(validation.StatusQueued)

	AssertValidator(t, staker, addr3).
		IsEmpty(false).
		PendingLocked(stake).
		Status(validation.StatusQueued)

	// Error case - keep as is since framework doesn't handle errors
	err := staker.AddValidator(addr4, addr4, uint32(360)*24*14, stake)
	assert.Error(t, err, "period is out of boundaries")
	AssertValidator(t, staker, addr4).IsEmpty(true)

	AssertValidator(t, staker, addr1).
		IsEmpty(false).
		LockedVET(stake).
		Status(validation.StatusActive)

	NewSequence(staker).Housekeep(180 * 2).Run(t)

	AssertValidator(t, staker, addr2).
		IsEmpty(false).
		LockedVET(stake).
		Status(validation.StatusActive)

	AssertValidator(t, staker, addr1).
		IsEmpty(false).
		LockedVET(stake).
		Status(validation.StatusActive)
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

	NewSequence(staker).
		AddValidator(addr, addr, uint32(360)*24*15, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake)

	NewSequence(staker).ActivateNext(0).Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake)
}

func TestStaker_Get_FullFlow_Renewal_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		AddValidator(addr1, addr1, period, stake).
		AddValidator(addr2, addr2, period, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	NewSequence(staker).
		ActivateNext(0).
		ActivateNext(0).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	NewSequence(staker).
		SignalExit(addr, addr).
		Housekeep(period).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusExit).
		LockedVET(big.NewInt(0)).
		CooldownVET(stake).
		WithdrawableVET(big.NewInt(0))

	NewSequence(staker).
		WithdrawStake(addr, addr, period+cooldownPeriod, stake).
		Run(t)
}

func TestStaker_WithdrawQueued(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AssertFirstQueued(thor.Address{}).
		AddValidator(addr, addr, period, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	NewSequence(staker).
		AssertFirstQueued(addr).
		WithdrawStake(addr, addr, period+cooldownPeriod, stake).
		AssertFirstQueued(thor.Address{}).
		Run(t)
}

func TestStaker_IncreaseQueued(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// Error case - keep as is since framework doesn't handle errors
	err := staker.IncreaseStake(addr, thor.Address{}, stake)
	assert.Error(t, err, "validator not found")

	NewSequence(staker).
		AddValidator(addr, addr, uint32(360)*24*15, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	// Increase stake queued
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	NewSequence(staker).
		IncreaseStake(addr, addr, big.NewInt(1000)).
		Run(t)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	newAmount := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, newAmount, expectedStake)

	AssertValidator(t, staker, addr).
		IsEmpty(false).
		Status(validation.StatusQueued).
		PendingLocked(expectedStake).
		Weight(big.NewInt(0))
}

func TestStaker_IncreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	// Error case - keep as is since framework doesn't handle errors
	err := staker.IncreaseStake(addr, thor.Address{}, stake)
	assert.Error(t, err, "validator not found")

	NewSequence(staker).
		AddValidator(addr, addr, uint32(360)*24*15, stake).
		AddValidator(addr1, addr1, uint32(360)*24*15, stake).
		AddValidator(addr2, addr2, uint32(360)*24*15, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	AssertValidator(t, staker, addr1).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	// Verify order
	NewSequence(staker).AssertFirstQueued(addr).Run(t)

	// Increase stake queued
	increaseBy := big.NewInt(1000)
	expectedIncreaseStake := big.NewInt(0).Add(stake, increaseBy)

	NewSequence(staker).
		IncreaseStake(addr1, addr1, increaseBy).
		AssertFirstQueued(addr).
		AssertNext(addr, addr1).
		AssertNext(addr1, addr2).
		Run(t)

	AssertValidator(t, staker, addr).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	AssertValidator(t, staker, addr1).
		PendingLocked(expectedIncreaseStake).
		Weight(big.NewInt(0))

	AssertValidator(t, staker, addr2).
		PendingLocked(stake).
		Weight(big.NewInt(0))
}

func TestStaker_DecreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	// Error case - keep as is since framework doesn't handle errors
	err := staker.DecreaseStake(addr, thor.Address{}, stake)
	assert.Error(t, err, "validator not found")

	NewSequence(staker).
		AddValidator(addr, addr, uint32(360)*24*15, stake).
		AddValidator(addr1, addr1, uint32(360)*24*15, stake).
		AddValidator(addr2, addr2, uint32(360)*24*15, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	AssertValidator(t, staker, addr1).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	// Verify order
	NewSequence(staker).AssertFirstQueued(addr).Run(t)

	// Decrease stake queued
	decreaseBy := big.NewInt(1000)
	expectedDecreaseStake := big.NewInt(0).Sub(stake, decreaseBy)

	NewSequence(staker).
		DecreaseStake(addr1, addr1, decreaseBy).
		AssertFirstQueued(addr).
		AssertNext(addr, addr1).
		AssertNext(addr1, addr2).
		Run(t)

	AssertValidator(t, staker, addr).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	AssertValidator(t, staker, addr1).
		PendingLocked(expectedDecreaseStake).
		Weight(big.NewInt(0))

	AssertValidator(t, staker, addr2).
		PendingLocked(stake).
		Weight(big.NewInt(0))
}

func TestStaker_IncreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	// Increase stake of an active validator
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	NewSequence(staker).
		IncreaseStake(addr, addr, big.NewInt(1000)).
		Run(t)

	AssertValidator(t, staker, addr).
		LockedVET(stake).
		PendingLocked(big.NewInt(1000)).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	NewSequence(staker).Housekeep(period).Run(t)

	AssertValidator(t, staker, addr).
		Weight(big.NewInt(0).Mul(expectedStake, big.NewInt(2))).
		LockedVET(expectedStake).
		PendingLocked(big.NewInt(0))
}

func TestStaker_ChangeStakeActiveValidatorWithQueued(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	addr := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		AddValidator(addr2, addr2, period, stake).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	AssertValidator(t, staker, addr2).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	NewSequence(staker).
		AssertQueuedVET(stake, big.NewInt(0).Mul(stake, big.NewInt(2))).
		Run(t)

	// Increase stake of an active validator
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	NewSequence(staker).
		IncreaseStake(addr, addr, big.NewInt(1000)).
		Run(t)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, expectedStake, newStake)

	// The queued stake also increases
	NewSequence(staker).
		AssertQueuedVET(big.NewInt(0).Add(stake, big.NewInt(1000)),
			big.NewInt(0).Mul(big.NewInt(0).Add(stake, big.NewInt(1000)), big.NewInt(2))).
		Run(t)

	validator, err = staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, big.NewInt(0).Add(validator.LockedVET, validator.PendingLocked))
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	NewSequence(staker).Housekeep(period).Run(t)

	AssertValidator(t, staker, addr).
		Weight(big.NewInt(0).Mul(expectedStake, big.NewInt(2)))

	// Verify queued stake is still the same as before the increase
	NewSequence(staker).
		AssertQueuedVET(stake, big.NewInt(0).Mul(stake, big.NewInt(2))).
		Run(t)

	// Decrease stake
	decreaseAmount := big.NewInt(2500)
	decreasedAmount := big.NewInt(0).Sub(expectedStake, decreaseAmount)
	NewSequence(staker).
		DecreaseStake(addr, addr, decreaseAmount).
		Run(t)

	AssertValidator(t, staker, addr).
		NextPeriodDecrease(decreaseAmount).
		Weight(big.NewInt(0).Mul(expectedStake, big.NewInt(2)))

	NewSequence(staker).
		AssertQueuedVET(stake, big.NewInt(0).Mul(stake, big.NewInt(2))).
		Housekeep(period).
		Run(t)

	AssertValidator(t, staker, addr).
		WithdrawableVET(big.NewInt(2500)).
		Weight(big.NewInt(0).Mul(decreasedAmount, big.NewInt(2)))

	NewSequence(staker).
		AssertQueuedVET(stake, big.NewInt(0).Mul(stake, big.NewInt(2))).
		Run(t)
}

func TestStaker_DecreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStake
	period := uint32(360) * 24 * 15
	decrease := big.NewInt(1000)

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		WithdrawableVET(big.NewInt(0))

	// Decrease stake of an active validator
	expectedStake := big.NewInt(0).Sub(stake, decrease)
	NewSequence(staker).DecreaseStake(addr, addr, decrease).Run(t)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, stake, newStake)

	AssertValidator(t, staker, addr).
		LockedVET(stake).
		NextPeriodDecrease(decrease).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	NewSequence(staker).Housekeep(period).Run(t)

	AssertValidator(t, staker, addr).
		WithdrawableVET(big.NewInt(1000)).
		Weight(big.NewInt(0).Mul(expectedStake, big.NewInt(2)))
}

func TestStaker_DecreaseActiveThenExit(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStake
	period := uint32(360) * 24 * 15
	decrease := big.NewInt(1000)
	expectedStake := big.NewInt(0).Sub(stake, decrease)

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		WithdrawableVET(big.NewInt(0))

	NewSequence(staker).DecreaseStake(addr, addr, decrease).Run(t)

	AssertValidator(t, staker, addr).
		LockedVET(stake).
		NextPeriodDecrease(decrease).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	NewSequence(staker).Housekeep(period).Run(t)

	AssertValidator(t, staker, addr).
		WithdrawableVET(big.NewInt(1000)).
		LockedVET(expectedStake).
		NextPeriodDecrease(big.NewInt(0))

	NewSequence(staker).
		SignalExit(addr, addr).
		Housekeep(period * 2).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusExit).
		WithdrawableVET(big.NewInt(1000)).
		LockedVET(big.NewInt(0)).
		CooldownVET(expectedStake).
		PendingLocked(big.NewInt(0))
}

func TestStaker_Get_FullFlow(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		AddValidator(addr1, addr1, period, stake).
		AddValidator(addr2, addr2, period, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	NewSequence(staker).
		ActivateNext(0).
		ActivateNext(0).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	NewSequence(staker).
		SignalExit(addr, addr).
		Housekeep(period).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusExit).
		LockedVET(big.NewInt(0)).
		Weight(big.NewInt(0))

	NewSequence(staker).Housekeep(period + cooldownPeriod).Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusExit).
		LockedVET(big.NewInt(0)).
		Weight(big.NewInt(0))

	NewSequence(staker).WithdrawStake(addr, addr, period+cooldownPeriod, stake).Run(t)
}

func TestStaker_Get_FullFlow_Renewal_On(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).AddValidator(addr, addr, period, stake).Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	NewSequence(staker).ActivateNext(0).Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	// Auto-renewal keeps the validator active after housekeeping
	NewSequence(staker).Housekeep(period).Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	// Try to withdraw - should get 0 because auto-renewal is on
	NewSequence(staker).
		WithdrawStake(addr, addr, period+cooldownPeriod, big.NewInt(0)).
		Run(t)

	AssertValidator(t, staker, addr).IsEmpty(false)
}

func TestStaker_Get_FullFlow_Renewal_On_Then_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr, addr, period, stake).
		AddValidator(addr1, addr1, period, stake).
		AddValidator(addr2, addr2, period, stake).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusQueued).
		PendingLocked(stake).
		Weight(big.NewInt(0))

	NewSequence(staker).
		ActivateNext(0).
		ActivateNext(0).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	NewSequence(staker).Housekeep(period).Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2)))

	// Try to withdraw (should get 0 because auto-renewal is on)
	NewSequence(staker).
		WithdrawStake(addr, addr, period+cooldownPeriod, big.NewInt(0)).
		Run(t)

	AssertValidator(t, staker, addr).IsEmpty(false)

	// Signal exit to turn off auto-renewal, then housekeep again
	NewSequence(staker).
		SignalExit(addr, addr).
		Housekeep(period * 2).
		Run(t)

	AssertValidator(t, staker, addr).
		Status(validation.StatusExit).
		CooldownVET(stake).
		Weight(big.NewInt(0)).
		LockedVET(big.NewInt(0)).
		PendingLocked(big.NewInt(0))

	// Now can withdraw the stake
	NewSequence(staker).WithdrawStake(addr, addr, period*2+cooldownPeriod, stake).Run(t)

	AssertValidator(t, staker, addr).IsEmpty(false)
	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake)

	NewSequence(staker).
		IncreaseStake(addr1, addr1, big.NewInt(100)).
		WithdrawStake(addr1, addr1, period+cooldownPeriod, big.NewInt(100)).
		Run(t)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		WithdrawableVET(big.NewInt(0)).
		PendingLocked(big.NewInt(0))
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	// fill 101 validations to leader group
	for range 101 {
		err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, RandomStake())
		assert.NoError(t, err)
		_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
	}

	// try to add one more to the leadergroup
	err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, RandomStake())
	assert.NoError(t, err)

	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.ErrorContains(t, err, "leader group is full")
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)
	_, err := staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.ErrorContains(t, err, "leader group is full")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	err := staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	validator, err := staker.Get(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	_, _, _, err := staker.validationService.ExitValidator(datagen.RandAddress())
	assert.NoError(t, err)
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidator(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr, addr)
	assert.NoError(t, err)

	// todo wrap this up / use a validation service or staker api instead of direct calling and checking totals
	releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, err := staker.validationService.ExitValidator(addr)
	require.NoError(t, err)

	// Exit the aggregation too
	aggExit, err := staker.aggregationService.Exit(addr)
	require.NoError(t, err)

	// Update global totals
	err = staker.globalStatsService.RemoveLocked(releaseLockedTVL, releaseLockedTVLWeight, releaseQueuedTVL, aggExit)
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

	withdrawale, err = staker.WithdrawStake(addr, addr, period+cooldownPeriod)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawale)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	stakes := make(map[thor.Address]*big.Int)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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
		err := staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		leaderGroup = append(leaderGroup, addr)
	}

	queuedGroup := [100]thor.Address{}
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidator(addr, addr, uint32(360)*24*15, stake)
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
		err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStake)
		assert.NoError(t, err)
	}

	transitioned, err := staker.Transition(0)
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validations after initialisation
	err = staker.AddValidator(addr, addr, uint32(360)*24*15, MinStake)
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

	err := staker.AddValidator(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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
	err := staker.AddValidator(addr1, addr1, period, stake)
	assert.NoError(t, err)

	totalLocked, totalWeight, err := staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalLocked.Int64())
	assert.Equal(t, big.NewInt(0).Int64(), totalWeight.Int64())
	assert.Equal(t, stake, totalQueued)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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
	err = staker.AddValidator(addr2, addr2, period, stake)
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalWeight)
	assert.Equal(t, stake, totalQueued)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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
	err = staker.AddValidator(addr3, addr3, period, stake)
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(4)), totalWeight)
	assert.Equal(t, stake, totalQueued)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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

	_, _, err = staker.Housekeep(period + cooldownPeriod)
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

	err := staker.AddValidator(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr3, addr3, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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
	_, _, err = staker.Housekeep(period + epochLength)
	assert.NoError(t, err)
	validator, err = staker.Get(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	// housekeep and exit validator 3
	_, _, err = staker.Housekeep(period + epochLength*2)
	assert.NoError(t, err)

	totalLocked, totalWeight, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, totalLocked.Sign())
	assert.Equal(t, big.NewInt(0).String(), totalWeight.String())

	withdrawable, err := staker.WithdrawStake(addr1, addr1, period+cooldownPeriod)
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

	err := staker.AddValidator(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr3, addr3, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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

	_, _, err = staker.Housekeep(period + epochLength)
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

	err := staker.AddValidator(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr2, addr2, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	err = staker.AddValidator(addr3, addr3, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(period*2, getTestMaxLeaderSize(staker.params))
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
	_, _, err = staker.Housekeep(period*3 + epochLength)
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
	err := staker.AddValidator(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.ActivateNextValidator(0, getTestMaxLeaderSize(staker.params))
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
	assert.Equal(t, big.NewInt(1), validator.PendingLocked)
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
	assert.Equal(t, validator.PendingLocked, big.NewInt(0))
}

func TestStaker_Housekeep_RecalculateDecrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStake
	period := uint32(360) * 24 * 15

	decrease := big.NewInt(1)
	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		ActivateNext(0).
		DecreaseStake(addr1, addr1, big.NewInt(1)).
		Housekeep(360 * 24 * 13).
		Run(t)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		NextPeriodDecrease(decrease)

	NewSequence(staker).Housekeep(360 * 24 * 15).Run(t)

	stake = big.NewInt(0).Sub(stake, decrease)
	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		WithdrawableVET(decrease)
}

func TestStaker_Housekeep_DecreaseThenWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStake
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		ActivateNext(0).
		DecreaseStake(addr1, addr1, big.NewInt(1)).
		Housekeep(360 * 24 * 13).
		Run(t)

	withdraw := big.NewInt(1)
	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		NextPeriodDecrease(withdraw)

	NewSequence(staker).Housekeep(360 * 24 * 15).Run(t)

	stake = big.NewInt(0).Sub(stake, withdraw)
	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		WithdrawableVET(withdraw)

	NewSequence(staker).
		WithdrawStake(addr1, addr1, period+cooldownPeriod, withdraw).
		AssertFirstActive(addr1).
		Run(t)

	AssertValidator(t, staker, addr1).
		WithdrawableVET(big.NewInt(0)).
		Status(validation.StatusActive)
}

func TestStaker_DecreaseActive_DecreaseMultipleTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		ActivateNext(0).
		DecreaseStake(addr1, addr1, big.NewInt(1)).
		Run(t)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		NextPeriodDecrease(big.NewInt(1))

	NewSequence(staker).DecreaseStake(addr1, addr1, big.NewInt(1)).Run(t)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		NextPeriodDecrease(big.NewInt(2))

	NewSequence(staker).Housekeep(period).Run(t)

	expectedStake := big.NewInt(0).Sub(stake, big.NewInt(2))
	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(expectedStake).
		Weight(big.NewInt(0).Mul(expectedStake, big.NewInt(2))).
		WithdrawableVET(big.NewInt(2))
}

func TestStaker_Housekeep_Exit_Decrements_Leader_Group_Size(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		AddValidator(addr2, addr2, period, stake).
		ActivateNext(0).
		ActivateNext(0).
		SignalExit(addr1, addr1).
		SignalExit(addr2, addr2).
		Housekeep(period).
		Run(t)

	AssertValidator(t, staker, addr1).Status(validation.StatusExit)
	AssertValidator(t, staker, addr2).Status(validation.StatusActive)

	NewSequence(staker).Housekeep(period + epochLength).Run(t)

	AssertValidator(t, staker, addr2).Status(validation.StatusExit)

	NewSequence(staker).
		AssertLeaderGroupSize(0).
		AssertFirstActive(thor.Address{}).
		AddValidator(addr3, addr3, period, stake).
		ActivateNext(period).
		SignalExit(addr3, addr3).
		Run(t)

	AssertValidator(t, staker, addr3).Status(validation.StatusActive)

	NewSequence(staker).
		AssertLeaderGroupSize(1).
		AssertFirstActive(addr3).
		Housekeep(period * 2).
		AssertLeaderGroupSize(0).
		Run(t)

	AssertValidator(t, staker, addr3).Status(validation.StatusExit)
}

func TestStaker_Housekeep_Adds_Queued_Validators_Up_To_Limit(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		AddValidator(addr2, addr2, period, stake).
		AddValidator(addr3, addr3, period, stake).
		AssertQueueSize(3).
		AssertLeaderGroupSize(0).
		Housekeep(uint32(360) * 24 * 13).
		Run(t)

	AssertValidator(t, staker, addr1).Status(validation.StatusActive)
	AssertValidator(t, staker, addr2).Status(validation.StatusActive)
	AssertValidator(t, staker, addr3).Status(validation.StatusQueued)

	NewSequence(staker).AssertQueueSize(1).AssertLeaderGroupSize(2)
}

func TestStaker_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		WithdrawStake(addr1, addr1, period, stake).
		Run(t)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusExit).
		LockedVET(big.NewInt(0)).
		Weight(big.NewInt(0)).
		WithdrawableVET(big.NewInt(0)).
		PendingLocked(big.NewInt(0))
}

func TestStaker_IncreaseStake_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(addr1, addr1, period, stake).
		Housekeep(period).
		Run(t)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake)

	expectedWithdraw := big.NewInt(100)
	NewSequence(staker).
		IncreaseStake(addr1, addr1, big.NewInt(100)).
		WithdrawStake(addr1, addr1, period+cooldownPeriod, expectedWithdraw)

	AssertValidator(t, staker, addr1).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(big.NewInt(0).Mul(stake, big.NewInt(2))).
		WithdrawableVET(big.NewInt(0)).
		PendingLocked(big.NewInt(0))
}

func TestStaker_GetRewards(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15
	reward := big.NewInt(1000)

	NewSequence(staker).
		AddValidator(proposerAddr, proposerAddr, period, stake).
		ActivateNext(0).
		Housekeep(period). // start staking period 2
		IncreaseDelegatorsReward(proposerAddr, reward).
		Run(t)

	AssertValidator(t, staker, proposerAddr).
		Rewards(1, big.NewInt(0)).
		Rewards(2, reward)
}

func TestStaker_GetCompletedPeriods(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)

	proposerAddr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 15

	NewSequence(staker).
		AddValidator(proposerAddr, proposerAddr, period, stake).
		ActivateNext(0).
		Run(t)

	AssertValidator(t, staker, proposerAddr).CompletedPeriods(0)
	NewSequence(staker).Housekeep(period).Run(t)
	AssertValidator(t, staker, proposerAddr).CompletedPeriods(1)
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
	increases.Add(increases, thousand)
	decreases.Add(decreases, fiveHundred)

	NewSequence(staker).
		AddValidator(acc, acc, period, initialStake).
		IncreaseStake(acc, acc, thousand).
		DecreaseStake(acc, acc, fiveHundred).
		Run(t)

	AssertValidator(t, staker, acc).Status(validation.StatusQueued)

	// 1st STAKING PERIOD
	NewSequence(staker).Housekeep(period).Run(t)

	expected := new(big.Int).Sub(initialStake, decreases)
	expected = expected.Add(expected, increases)

	AssertValidator(t, staker, acc).
		Status(validation.StatusActive).
		LockedVET(expected)

	// See `1st decrease` -> validator should be able withdraw the decrease amount
	withdraw, err := staker.WithdrawStake(acc, acc, period+1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw, fiveHundred)
	withdrawnTotal = withdrawnTotal.Add(withdrawnTotal, withdraw)

	expectedLocked := new(big.Int).Sub(initialStake, decreases)
	expectedLocked = expectedLocked.Add(expectedLocked, increases)
	AssertValidator(t, staker, acc).LockedVET(expectedLocked)

	// 2nd decrease
	decreases.Add(decreases, thousand)
	increases.Add(increases, fiveHundred)

	NewSequence(staker).
		DecreaseStake(acc, acc, thousand).
		IncreaseStake(acc, acc, fiveHundred).
		Housekeep(period * 2).
		Run(t)

	AssertValidator(t, staker, acc).Status(validation.StatusActive)

	// See `2nd decrease` -> validator should be able withdraw the decrease amount
	withdraw, err = staker.WithdrawStake(acc, acc, period*2+cooldownPeriod)
	assert.NoError(t, err)
	assert.Equal(t, thousand, withdraw)
	withdrawnTotal = withdrawnTotal.Add(withdrawnTotal, withdraw)

	NewSequence(staker).
		SignalExit(acc, acc).
		Housekeep(period * 3).
		Run(t)

	expectedLocked = new(big.Int).Sub(initialStake, decreases)
	expectedLocked = expectedLocked.Add(expectedLocked, increases)

	AssertValidator(t, staker, acc).
		Status(validation.StatusExit).
		CooldownVET(expectedLocked)

	withdraw, err = staker.WithdrawStake(acc, acc, period*3+cooldownPeriod)
	assert.NoError(t, err)
	withdrawnTotal.Add(withdrawnTotal, withdraw)
	depositTotal := new(big.Int).Add(initialStake, increases)
	assert.Equal(t, depositTotal, withdrawnTotal)
}

func Test_GetValidatorTotals(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := big.NewInt(0).Set(MinStake)
	validator := validators[0]
	multiplier := uint8(200)

	var delegationID thor.Bytes32
	NewSequence(staker).AddDelegation(validator.ID, stake, multiplier, &delegationID).Run(t)

	AssertAggregation(t, staker, validator.ID).
		PendingVET(stake).
		PendingWeight(big.NewInt(0).Mul(stake, big.NewInt(int64(2))))

	AssertDelegation(t, staker, delegationID).
		Stake(stake).
		Multiplier(multiplier).
		FirstIteration(2).
		IsLocked(false)

	NewSequence(staker).Housekeep(validator.Period).Run(t)
	AssertDelegation(t, staker, delegationID).IsLocked(true)

	delegation, _, err := staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	totals, err := staker.GetValidatorsTotals(validator.ID)
	assert.NoError(t, err)

	fetchedValidator, err := staker.Get(validator.ID)
	assert.NoError(t, err)

	expectedStake := big.NewInt(0).Set(aggregation.LockedVET)
	expectedStake.Add(expectedStake, validator.LockedVET)

	assert.Equal(t, expectedStake, totals.TotalLockedStake)
	assert.Equal(t, fetchedValidator.Weight, totals.TotalLockedWeight)
	assert.Equal(t, delegation.Stake, totals.DelegationsLockedStake)
	assert.Equal(t, delegation.CalcWeight(), totals.DelegationsLockedWeight)
}

func Test_Validator_Decrease_Exit_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()
	originalStake := big.NewInt(0).Mul(big.NewInt(3), MinStake)
	decrease := big.NewInt(0).Mul(big.NewInt(2), MinStake)

	NewSequence(staker).
		AddValidator(acc, acc, LowStakingPeriod, originalStake).
		ActivateNext(0).
		DecreaseStake(acc, acc, decrease). // 75m - 25m = 50m
		Housekeep(LowStakingPeriod).
		WithdrawStake(acc, acc, LowStakingPeriod+cooldownPeriod, decrease).
		Run(t)

	AssertValidator(t, staker, acc).
		Status(validation.StatusActive).
		LockedVET(big.NewInt(0).Sub(originalStake, decrease))
}

func Test_Validator_Decrease_SeveralTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()
	originalStake := big.NewInt(0).Mul(big.NewInt(3), MinStake)

	NewSequence(staker).
		AddValidator(acc, acc, LowStakingPeriod, originalStake).
		ActivateNext(0).
		DecreaseStake(acc, acc, MinStake). // 75m - 25m = 50m
		DecreaseStake(acc, acc, MinStake). // 50m - 25m
		Run(t)

	assert.ErrorContains(t, staker.DecreaseStake(acc, acc, MinStake), "next period stake is too low for validator")

	AssertValidator(t, staker, acc).
		Status(validation.StatusActive).
		LockedVET(originalStake).
		NextPeriodDecrease(big.NewInt(0).Mul(big.NewInt(2), MinStake))

	NewSequence(staker).Housekeep(LowStakingPeriod * 2).Run(t)

	AssertValidator(t, staker, acc).
		Status(validation.StatusActive).
		LockedVET(MinStake).
		NextPeriodDecrease(big.NewInt(0))
}

func Test_Validator_IncreaseDecrease_Combinations(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	acc := datagen.RandAddress()

	NewSequence(staker).
		AddValidator(acc, acc, LowStakingPeriod, MinStake).
		IncreaseStake(acc, acc, MinStake). // 25m + 25m = 50m
		DecreaseStake(acc, acc, MinStake). // 50m - 25m = 25m
		ActivateNext(0).
		WithdrawStake(acc, acc, 0, MinStake).
		Run(t)

	AssertValidator(t, staker, acc).Status(validation.StatusActive).LockedVET(MinStake)

	assert.NoError(t, staker.IncreaseStake(acc, acc, MinStake))
	assert.ErrorContains(t, staker.DecreaseStake(acc, acc, MinStake), "next period stake is too low for validator")

	NewSequence(staker).
		WithdrawStake(acc, acc, 0, MinStake).
		Housekeep(LowStakingPeriod).
		WithdrawStake(acc, acc, LowStakingPeriod+cooldownPeriod, big.NewInt(0)).
		Run(t)

	AssertValidator(t, staker, acc).LockedVET(MinStake)
}

func TestStaker_AddValidator_CannotAddValidationWithSameMaster(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	address := datagen.RandAddress()
	err := staker.AddValidator(datagen.RandAddress(), address, uint32(360)*24*15, MinStake)
	assert.NoError(t, err)

	err = staker.AddValidator(datagen.RandAddress(), address, uint32(360)*24*15, MinStake)
	assert.Error(t, err, "validator already exists")
}

func TestStaker_AddValidator_CannotAddValidationWithSameMasterAfterExit(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	master := datagen.RandAddress()
	endorsor := datagen.RandAddress()

	// Add, activate and exit a validator
	NewSequence(staker).
		AddValidator(endorsor, master, MediumStakingPeriod, MinStake).
		ActivateNext(0).
		SignalExit(endorsor, master).
		Housekeep(MediumStakingPeriod).
		Run(t)

	err := staker.AddValidator(datagen.RandAddress(), master, MediumStakingPeriod, MinStake)
	assert.Error(t, err, "validator already exists")
}

func getTestMaxLeaderSize(param *params.Params) *big.Int {
	maxLeaderGroupSize, err := param.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		panic(err)
	}
	return maxLeaderGroupSize
}
