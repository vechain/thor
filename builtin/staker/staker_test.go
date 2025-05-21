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

// RandomStake returns a random number between minStake and (maxStake/2)
func RandomStake() *big.Int {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	max := big.NewInt(0).Div(maxStake, big.NewInt(2))

	// Calculate the range (max - minStake)
	rangeStake := new(big.Int).Sub(max, minStake)

	// Generate a random number within the range
	randomOffset := new(big.Int).Rand(rng, rangeStake)

	// Add minStake to ensure the value is within the desired range
	return new(big.Int).Add(minStake, randomOffset)
}

type keySet struct {
	endorsor thor.Address
	master   thor.Address
}

func createKeys(amount int) map[thor.Address]keySet {
	keys := make(map[thor.Address]keySet)
	for range amount {
		nodeMaster := datagen.RandAddress()
		endorsor := datagen.RandAddress()

		keys[nodeMaster] = keySet{
			endorsor: endorsor,
			master:   nodeMaster,
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
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param)
	totalStake := big.NewInt(0)
	if initialise {
		for _, key := range keys {
			stake := RandomStake()
			totalStake = totalStake.Add(totalStake, stake)
			if _, err := staker.AddValidator(key.endorsor, key.master, uint32(360)*24*14, stake, true, 0); err != nil {
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
		{M(stkr.LockedVET()), M(zeroStake, nil)},
		{M(stkr.AddValidator(validator1, validator1, uint32(360)*24*14, stakeAmount, true, 0)), M(thor.MustParseBytes32("0xf97f97edb19ac8a202886795b0a2df88a126cce926e50874448ce885c7ff472e"), nil)},
		{M(stkr.AddValidator(validator2, validator2, uint32(360)*24*14, stakeAmount, true, 0)), M(thor.MustParseBytes32("0xcc72fbf4d301b909215e6ae6b5e6b305843daae701fda7325d04267de4cf731d"), nil)},
		{M(stkr.Transition(0)), M(true, nil)},
		{M(stkr.LockedVET()), M(totalStake, nil)},
		{M(stkr.AddValidator(validator3, validator3, uint32(360)*24*14, stakeAmount, true, 0)), M(thor.MustParseBytes32("0x5ab9f98c3694a90d5c55443e1ca48ff71e3e0d7523ddcfac2fc4033b780e0390"), nil)},
		{M(stkr.FirstQueued()), M(thor.MustParseBytes32("0x5ab9f98c3694a90d5c55443e1ca48ff71e3e0d7523ddcfac2fc4033b780e0390"), nil)},
		{M(stkr.validations.ActivateNext(0, stkr.params)), M(nil)},
		{M(stkr.FirstActive()), M(thor.MustParseBytes32("0xf97f97edb19ac8a202886795b0a2df88a126cce926e50874448ce885c7ff472e"), nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestStaker_TotalStake(t *testing.T) {
	staker, totalStaked := newStaker(t, 0, 14, false)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Bytes32]*big.Int)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stakeAmount, false, 0)
		assert.NoError(t, err)
		stakes[id] = stakeAmount
		totalStaked = totalStaked.Add(totalStaked, stakeAmount)
		assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
		staked, err := staker.LockedVET()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}

	for id, stake := range stakes {
		assert.NoError(t, staker.validations.ExitValidator(id, uint32(360)*24*14*2))
		totalStaked = totalStaked.Sub(totalStaked, stake)
		staked, err := staker.LockedVET()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
	}
}

func TestStaker_TotalStake_Withdrawal(t *testing.T) {
	staker, _ := newStaker(t, 0, 14, false)

	addr := datagen.RandAddress()
	stakeAmount := RandomStake()
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stakeAmount, false, 0)
	assert.NoError(t, err)

	queuedStake, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, queuedStake)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	lockedVET, err := staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, lockedVET)

	queuedStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, 0, queuedStake.Sign())

	assert.NoError(t, staker.validations.ExitValidator(id, uint32(360)*24*14*2))

	lockedVET, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, lockedVET.Sign())

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, stakeAmount, validator.WithdrawableVET)

	withdrawnAmount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, withdrawnAmount)

	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, 0, validator.WithdrawableVET.Sign())

	lockedVET, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, lockedVET.Sign())

	queuedStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, 0, queuedStake.Sign())
}

func TestStaker_AddValidator_MinimumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooLow := big.NewInt(0).Sub(minStake, big.NewInt(1))
	_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, tooLow, true, 0)
	assert.ErrorContains(t, err, "stake is out of range")
	_, err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true, 0)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_MaximumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooHigh := big.NewInt(0).Add(maxStake, big.NewInt(1))
	_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, tooHigh, true, 0)
	assert.ErrorContains(t, err, "stake is out of range")
	_, err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, maxStake, true, 0)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_MaximumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, minStake, true, 0)
	assert.ErrorContains(t, err, "period is out of boundaries")
	_, err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true, 0)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_MinimumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, minStake, true, 0)
	assert.ErrorContains(t, err, "period is out of boundaries")
	_, err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), 100, minStake, true, 0)
	assert.ErrorContains(t, err, "period is out of boundaries")
	_, err = staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true, 0)
	assert.NoError(t, err)
}

func TestStaker_AddValidator_Duplicate(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	_, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
	assert.ErrorContains(t, err, "validator already exists")
}

func TestStaker_AddValidator_QueueOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	expectedOrder := [100]thor.Address{}
	// add 100 validations to the queue
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		_, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
		assert.NoError(t, err)
		expectedOrder[i] = addr
	}

	first, err := staker.FirstQueued()
	assert.NoError(t, err)

	// iterating using the `Next` method should return the same order
	loopID := first
	for i := range 100 {
		loopVal, err := staker.storage.GetValidator(loopID)
		assert.NoError(t, err)
		assert.Equal(t, expectedOrder[i], loopVal.Master)

		next, err := staker.Next(loopID)
		assert.NoError(t, err)
		loopID = next
	}

	// activating validations should continue to set the correct head of the queue
	loopID = first
	for range 99 {
		assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
		first, err = staker.FirstQueued()
		assert.NoError(t, err)
		previous, err := staker.Get(loopID)
		assert.NoError(t, err)
		current, err := staker.Get(first)
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
	id1, err := staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	id2, err := staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	id3, err := staker.AddValidator(addr3, addr3, uint32(360)*24*30, stake, true, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	id4, err := staker.AddValidator(addr4, addr4, uint32(360)*24*15, stake, true, 0)
	assert.Error(t, err, "period is out of boundaries")

	validator, err = staker.Get(id4)
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
	id1, err := staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	_, err = staker.Housekeep(180)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, StatusActive, validator.Status)

	id2, err := staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	id3, err := staker.AddValidator(addr3, addr3, uint32(360)*24*30, stake, true, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	id4, err := staker.AddValidator(addr4, addr4, uint32(360)*24*15, stake, true, 0)
	assert.Error(t, err, "period is out of boundaries")

	validator, err = staker.Get(id4)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, StatusActive, validator.Status)

	_, err = staker.Housekeep(180 * 2)
	assert.NoError(t, err)

	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, StatusActive, validator.Status)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_Get_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	id := datagen.RandomHash()
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.True(t, validator.IsEmpty())
}

func TestStaker_Get(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, StatusQueued, validator.Status)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
}

func TestStaker_Get_FullFlow_Renewal_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, false, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	// remove the validator
	assert.NoError(t, staker.validations.ExitValidator(id, period+100))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, id)
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
	assert.Equal(t, thor.Bytes32{}, queued)

	// add the validator
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// verify queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, id, queued)

	// withraw queued
	withdrawAmount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)

	// verify removed queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, thor.Bytes32{}, queued)
}

func TestStaker_IncreaseQueued(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	stake := RandomStake()

	err := staker.IncreaseStake(addr, thor.Bytes32{}, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// increase stake queued
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	err = staker.IncreaseStake(addr, id, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	newAmount := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, newAmount, expectedStake)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
	assert.Equal(t, validator.Status, StatusQueued)
	assert.Equal(t, validator.PendingLocked, expectedStake)
	assert.Equal(t, big.NewInt(0), validator.Weight)
}

func TestStaker_IncreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	err := staker.IncreaseStake(addr, thor.Bytes32{}, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	id1, err := staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	id2, err := staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, id)

	// increase stake queued
	increaseBy := big.NewInt(1000)
	err = staker.IncreaseStake(addr1, id1, increaseBy)
	assert.NoError(t, err)
	expectedIncreaseStake := big.NewInt(0).Add(stake, increaseBy)

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, id)
	entry, err := staker.Get(queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.PendingLocked)
	assert.Equal(t, big.NewInt(0), entry.Weight)

	queued, err = staker.Next(queued)
	assert.NoError(t, err)
	assert.Equal(t, queued, id1)
	entry, err = staker.Get(queued)
	assert.NoError(t, err)
	assert.Equal(t, expectedIncreaseStake, entry.PendingLocked)
	assert.Equal(t, big.NewInt(0), entry.Weight)

	queued, err = staker.Next(queued)
	assert.NoError(t, err)
	assert.Equal(t, queued, id2)
	entry, err = staker.Get(queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.PendingLocked)
	assert.Equal(t, big.NewInt(0), entry.Weight)
}

func TestStaker_DecreaseQueued_Order(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	err := staker.DecreaseStake(addr, thor.Bytes32{}, stake)
	assert.Error(t, err, "validator not found")

	// add the validator
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	id1, err := staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	id2, err := staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, false, 0)
	assert.NoError(t, err)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, id)

	// increase stake queued
	decreaseBy := big.NewInt(1000)
	err = staker.DecreaseStake(addr1, id1, decreaseBy)
	assert.NoError(t, err)

	expectedDecreaseStake := big.NewInt(0).Sub(stake, decreaseBy)

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, id)
	entry, err := staker.Get(queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.PendingLocked)
	assert.Equal(t, big.NewInt(0), entry.Weight)
	queued, err = staker.Next(queued)
	assert.NoError(t, err)
	assert.Equal(t, queued, id1)
	entry, err = staker.Get(queued)
	assert.NoError(t, err)
	assert.Equal(t, expectedDecreaseStake, entry.PendingLocked)
	assert.Equal(t, big.NewInt(0), entry.Weight)

	queued, err = staker.Next(queued)
	assert.NoError(t, err)
	assert.Equal(t, queued, id2)
	entry, err = staker.Get(queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.PendingLocked)
	assert.Equal(t, big.NewInt(0), entry.Weight)
}

func TestStaker_IncreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// increase stake of an active validator
	expectedStake := big.NewInt(0).Add(big.NewInt(1000), stake)
	err = staker.IncreaseStake(addr, id, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, big.NewInt(0).Add(validator.LockedVET, validator.PendingLocked))
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)
}

func TestStaker_DecreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := maxStake
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw is empty
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(0))

	// decrease stake of an active validator
	expectedStake := big.NewInt(0).Sub(stake, big.NewInt(1000))
	err = staker.DecreaseStake(addr, id, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.LockedVET)
	assert.Equal(t, big.NewInt(1000), validator.CooldownVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)
}

func TestStaker_DecreaseActiveThenExit(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := maxStake
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw is empty
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(0))

	// decrease stake of an active validator
	expectedStake := big.NewInt(0).Sub(stake, big.NewInt(1000))
	err = staker.DecreaseStake(addr, id, big.NewInt(1000))
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	newStake := big.NewInt(0).Add(validator.PendingLocked, validator.LockedVET)
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.LockedVET)
	assert.Equal(t, big.NewInt(1000), validator.CooldownVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0).Mul(expectedStake, big.NewInt(2)), validator.Weight)

	assert.NoError(t, staker.UpdateAutoRenew(addr, id, false, period))

	_, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)

	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, big.NewInt(1000), validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1000)), validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.PendingLocked)
}

func TestStaker_Get_FullFlow(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, false, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	_, err = staker.Housekeep(period + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// remove the validator
	assert.NoError(t, staker.validations.ExitValidator(id, period+100))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
}

func TestStaker_Get_FullFlow_Renewal_On(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// add the validator
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// housekeep the validator
	_, err = staker.Housekeep(uint32(360) * 24 * 14)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// remove the validator
	assert.Error(t, staker.validations.ExitValidator(id, 100), "validator cannot be removed")
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), amount)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
}

func TestStaker_Get_FullFlow_Renewal_On_Then_Off(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, true, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr2, addr2, period, stake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	// housekeep the validator
	_, err = staker.Housekeep(period + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// remove the validator
	assert.Error(t, staker.validations.ExitValidator(id, 100), "validator cannot be removed")
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), amount)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())

	assert.NoError(t, staker.UpdateAutoRenew(addr, id, false, 0))

	// housekeep the validator
	_, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, stake, validator.Weight)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.PendingLocked)

	// remove the validator
	_, err = staker.Housekeep(period*2 + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, stake, validator.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
}

func TestStaker_Get_FullFlow_Renewal_Off_Then_On(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// add the validator
	id, err := staker.AddValidator(addr, addr, period, stake, false, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	_, err = staker.AddValidator(addr2, addr2, period, stake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.PendingLocked)
	assert.Equal(t, big.NewInt(0), validator.Weight)

	// activate the validator
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	assert.NoError(t, staker.UpdateAutoRenew(addr, id, true, 0))

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// remove the validator
	assert.Error(t, staker.validations.ExitValidator(id, 100), "validator cannot be removed")
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), amount)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())

	assert.NoError(t, staker.UpdateAutoRenew(addr, id, false, 0))

	// housekeep the validator
	_, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	// housekeep after cooldown
	_, err = staker.Housekeep(period*2 + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)
	assert.Equal(t, stake, validator.WithdrawableVET)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, id)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.False(t, validator.IsEmpty())
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	// fill 101 validations to leader group
	for range 101 {
		_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, RandomStake(), true, 0)
		assert.NoError(t, err)
		assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	}

	// try to add one more to the leadergroup
	_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, RandomStake(), true, 0)
	assert.NoError(t, err)
	assert.ErrorContains(t, staker.validations.ActivateNext(0, staker.params), "leader group is full")
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)
	assert.ErrorContains(t, staker.validations.ActivateNext(0, staker.params), "leader group is full")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	assert.NoError(t, staker.validations.ExitValidator(datagen.RandomHash(), 100))
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id, err := staker.AddValidator(addr, addr, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	assert.NoError(t, staker.validations.ExitValidator(id, period+1))

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.Weight)
	assert.Equal(t, stake, validator.WithdrawableVET)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	stakes := make(map[thor.Bytes32]*big.Int)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
		assert.NoError(t, err)
		assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
		stakes[id] = stake
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

	id := datagen.RandomHash()
	next, err := staker.Next(id)
	assert.Nil(t, err)
	assert.Equal(t, thor.Bytes32{}, next)
}

func TestStaker_Next(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	leaderGroup := make([]thor.Bytes32, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		id, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
		assert.NoError(t, err)
		assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
		leaderGroup = append(leaderGroup, id)
	}

	queuedGroup := [100]thor.Address{}
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		_, err := staker.AddValidator(addr, addr, uint32(360)*24*14, stake, true, 0)
		assert.NoError(t, err)
		queuedGroup[i] = addr
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
	for i := range 100 {
		currentVal, err := staker.Get(current)
		assert.NoError(t, err)
		assert.Equal(t, queuedGroup[i], currentVal.Master)

		next, err := staker.Next(current)
		assert.NoError(t, err)
		current = next
	}
}

func TestStaker_Initialise(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := datagen.RandAddress()

	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param)
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(3)))

	for range 3 {
		_, err := staker.AddValidator(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*14, minStake, true, 0)
		assert.NoError(t, err)
	}

	transitioned, err := staker.Transition(0)
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validations after initialisation
	_, err = staker.AddValidator(addr, addr, uint32(360)*24*14, minStake, true, 0)
	assert.NoError(t, err)

	staker, _ = newStaker(t, 101, 101, true)
	first, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.False(t, first.IsZero())

	expectedLength := big.NewInt(101)
	length, err := staker.validations.leaderGroup.Len()
	assert.NoError(t, err)
	assert.True(t, expectedLength.Cmp(length) == 0)
}

func TestStaker_Housekeep_TooEarly(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	id, err := staker.AddValidator(addr1, addr1, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	id2, err := staker.AddValidator(addr2, addr2, uint32(360)*24*14, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	_, err = staker.Housekeep(0)
	assert.NoError(t, err)
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
}

func TestStaker_Housekeep_ExitOne(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	// Add first validator
	id, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)

	totalLocked, err := staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalLocked.Int64())
	assert.Equal(t, stake, totalQueued)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, stake, totalLocked)

	// Add second validator
	id2, err := staker.AddValidator(addr2, addr2, period, stake, true, 0)
	assert.NoError(t, err)
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, totalLocked)
	assert.Equal(t, stake, totalQueued)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)

	// Add third validtor
	_, err = staker.AddValidator(addr3, addr3, period, stake, true, 0)
	assert.NoError(t, err)
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
	assert.Equal(t, stake, totalQueued)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(3)), totalLocked)

	// first should be on cooldown
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)

	_, err = staker.Housekeep(period + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.CooldownVET)
	assert.Equal(t, stake, validator.WithdrawableVET)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), totalQueued.Int64())
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), totalLocked)
}

func TestStaker_Housekeep_Cooldown(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	period := uint32(360) * 24 * 14

	stake := RandomStake()

	id1, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	id2, err := staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	_, err = staker.AddValidator(addr3, addr3, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	id, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, id1, id)
	id, err = staker.Next(id)
	assert.NoError(t, err)
	assert.Equal(t, id2, id)

	totalLocked, err := staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(3)), totalLocked)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	totalLocked, err = staker.LockedVET()
	assert.NoError(t, err)
	assert.Equal(t, 0, totalLocked.Sign())

	_, err = staker.Housekeep(period + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	assert.Equal(t, big.NewInt(0), validator.LockedVET)
	assert.Equal(t, big.NewInt(0), validator.CooldownVET)
	assert.Equal(t, stake, validator.WithdrawableVET)
}

func TestStaker_Housekeep_CooldownToExited(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	id2, err := staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	_, err = staker.AddValidator(addr3, addr3, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	_, err = staker.Housekeep(uint32(360) * 24 * 14)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(uint32(360)*24*14 + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
}

func TestStaker_Housekeep_ExitOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()
	addr5 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	id2, err := staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	id3, err := staker.AddValidator(addr3, addr3, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(period*2, staker.params))

	assert.NoError(t, staker.UpdateAutoRenew(addr1, id1, false, period*2+1))

	_, err = staker.Housekeep(period * 3)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(period*3 + cooldownPeriod)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	// housekeep again, 2/3 rule not applied
	_, err = staker.Housekeep(period * 4)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)

	_, err = staker.AddValidator(addr4, addr4, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(period*5, staker.params))

	_, err = staker.Housekeep(period * 6)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)

	_, err = staker.AddValidator(addr5, addr5, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(period*7, staker.params))

	_, err = staker.Housekeep(period * 8)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
}

func TestStaker_Housekeep_RecalculateIncrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := minStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	id1, err := staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	err = staker.IncreaseStake(addr1, id1, big.NewInt(1))
	assert.NoError(t, err)

	// housekeep half way through the period, validator's locked vet should not change
	_, err = staker.Housekeep(period / 2)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(1), validator.PendingLocked)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	stake = big.NewInt(0).Add(stake, big.NewInt(1))
	assert.Equal(t, validator.LockedVET, stake)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(0))
	assert.Equal(t, validator.PendingLocked, big.NewInt(0))
}

func TestStaker_Housekeep_RecalculateDecrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := maxStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	id1, err := staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	err = staker.DecreaseStake(addr1, id1, big.NewInt(1))
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1)), validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, big.NewInt(1), validator.CooldownVET)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1)), validator.LockedVET)

	block = uint32(360) * 24 * 14
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	stake = big.NewInt(0).Sub(stake, big.NewInt(1))
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.LockedVET, stake)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(1))
}

func TestStaker_Housekeep_DecreaseThenWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := maxStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	id1, err := staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	err = staker.DecreaseStake(addr1, id1, big.NewInt(1))
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	assert.Equal(t, validator.LockedVET, big.NewInt(0).Sub(stake, big.NewInt(1)))
	assert.Equal(t, validator.Weight, big.NewInt(0).Mul(stake, big.NewInt(2)))
	assert.Equal(t, validator.CooldownVET, big.NewInt(1))

	block = uint32(360) * 24 * 14
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	stake = big.NewInt(0).Sub(stake, big.NewInt(1))
	assert.Equal(t, validator.LockedVET, stake)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(1))

	withdrawAmount, err := staker.WithdrawStake(addr1, id1)
	assert.NoError(t, err)
	assert.Equal(t, validator.WithdrawableVET, withdrawAmount)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), validator.WithdrawableVET)

	// verify that validator is still present and active
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validator.Weight)
	activeValidator, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, id1, activeValidator)
}

func TestStaker_DecreaseActive_DecreaseMultipleTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := maxStake
	period := uint32(360) * 24 * 14

	// auto renew is turned on
	id1, err := staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	err = staker.DecreaseStake(addr1, id1, big.NewInt(1))
	assert.NoError(t, err)

	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(1)), validator.LockedVET)
	assert.Equal(t, validator.CooldownVET, big.NewInt(1))

	err = staker.DecreaseStake(addr1, id1, big.NewInt(1))
	assert.NoError(t, err)

	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Sub(stake, big.NewInt(2)), validator.LockedVET)
	assert.Equal(t, validator.CooldownVET, big.NewInt(2))

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, validator.WithdrawableVET, big.NewInt(2))
	assert.Equal(t, validator.CooldownVET, big.NewInt(0))
}

func TestStaker_Housekeep_Cannot_Exit_If_It_Breaks_Finality(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	exitBlock := uint32(360) * 24 * 14
	_, err = staker.Housekeep(exitBlock)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)

	_, err = staker.Housekeep(exitBlock + 8640)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)

	_, err = staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(exitBlock+8640, staker.params))
	_, err = staker.AddValidator(addr3, addr3, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(exitBlock+8640, staker.params))

	_, err = staker.Housekeep(exitBlock + 8640 + 360)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
}

func TestStaker_Housekeep_Exit_Decrements_Leader_Group_Size(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	id2, err := staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	exitBlock := uint32(360) * 24 * 14
	_, err = staker.Housekeep(exitBlock)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	validator, err = staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator.Status)
	leaderGroupSize, err := staker.validations.leaderGroup.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Uint64(), leaderGroupSize.Uint64())

	leaderGroupHead, err := staker.validations.leaderGroup.Peek()
	assert.NoError(t, err)
	assert.True(t, leaderGroupHead.IsEmpty())
	// add 1 more to satisfy the 2/3 rule
	addrBytes := []byte{228, 202, 197, 111, 38, 14, 207, 213, 17, 196, 29, 144, 140, 132, 77, 192, 58, 239, 29, 134}
	addr3 := thor.BytesToAddress(addrBytes)
	id3, err := staker.AddValidator(addr3, addr3, period, stake, false, 0)
	assert.NoError(t, err)
	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))
	validator, err = staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	leaderGroupSize, err = staker.validations.leaderGroup.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), leaderGroupSize)
	leaderGroupHead, err = staker.validations.leaderGroup.Peek()
	assert.NoError(t, err)
	assert.Equal(t, addr3, leaderGroupHead.Master)
	leaderGroupSize, err = staker.validations.cooldownQueue.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), leaderGroupSize)

	_, err = staker.Housekeep(exitBlock * 2)
	assert.NoError(t, err)
	validator, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validator.Status)
	validator2, err := staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator2.Status)
	validator3, err := staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusCooldown, validator3.Status)
	leaderGroupSize, err = staker.validations.leaderGroup.Len()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), leaderGroupSize.Uint64())
}

func TestStaker_Housekeep_Adds_Queued_Validators_Up_To_Limit(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)
	id2, err := staker.AddValidator(addr2, addr2, period, stake, false, 0)
	assert.NoError(t, err)
	id3, err := staker.AddValidator(addr3, addr3, period, stake, false, 0)
	assert.NoError(t, err)

	queuedValidators, err := staker.validations.validatorQueue.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(3), queuedValidators)

	leaderGroupSize, err := staker.validations.leaderGroup.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), leaderGroupSize.String())

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator.Status)
	validator1, err := staker.Get(id2)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validator1.Status)
	validator2, err := staker.Get(id3)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator2.Status)
	leaderGroupSize, err = staker.validations.leaderGroup.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), leaderGroupSize)
	queuedValidators, err = staker.validations.validatorQueue.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), queuedValidators)
}

func TestStaker_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, false, 0)
	assert.NoError(t, err)

	withdraw, err := staker.WithdrawStake(addr1, id1)
	assert.NoError(t, err)
	assert.Equal(t, stake, withdraw)

	validation, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, validation.Status)
	assert.Equal(t, big.NewInt(0), validation.LockedVET)
	assert.Equal(t, big.NewInt(0), validation.Weight)
	assert.Equal(t, big.NewInt(0), validation.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), validation.PendingLocked)
}

func TestStaker_IncreaseStake_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id1, err := staker.AddValidator(addr1, addr1, period, stake, true, 0)
	assert.NoError(t, err)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)

	validation, err := staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validation.Status)
	assert.Equal(t, stake, validation.LockedVET)

	assert.NoError(t, staker.IncreaseStake(addr1, id1, big.NewInt(100)))
	withdrawAmount, err := staker.WithdrawStake(addr1, id1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), withdrawAmount)

	validation, err = staker.Get(id1)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, validation.Status)
	assert.Equal(t, stake, validation.LockedVET)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), validation.Weight)
	assert.Equal(t, big.NewInt(0), validation.WithdrawableVET)
	assert.Equal(t, big.NewInt(0), validation.PendingLocked)
}

func TestStaker_GetRewards(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id, err := staker.AddValidator(proposerAddr, proposerAddr, period, stake, true, 0)
	assert.NoError(t, err)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	amount, err := staker.GetRewards(id, 1)
	assert.NoError(t, err)
	assert.Equal(t, new(big.Int), amount)

	reward := big.NewInt(1000)
	staker.IncreaseReward(proposerAddr, *reward)

	amount, err = staker.GetRewards(id, 1)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), amount)
}

func TestStaker_GetCompletedPeriods(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 14

	id, err := staker.AddValidator(proposerAddr, proposerAddr, period, stake, true, 0)
	assert.NoError(t, err)

	assert.NoError(t, staker.validations.ActivateNext(0, staker.params))

	periods, err := staker.GetCompletedPeriods(id)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), periods)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)

	periods, err = staker.GetCompletedPeriods(id)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), periods)
}
