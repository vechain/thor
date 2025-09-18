// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// #nosec G404
package staker

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
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

// RandomStake returns a random number between MinStake and (MaxStake/2)
func RandomStake() uint64 {
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))

	max := MaxStakeVET / 2
	// Calculate the range (max - MinStake)
	rangeStake := max - MinStakeVET

	// Generate a random number within the range
	randomOffset := rng.Uint64() % rangeStake

	// Add MinStake to ensure the value is within the desired range
	return MinStakeVET + randomOffset
}

type keySet struct {
	endorser thor.Address
	node     thor.Address
}

func createKeys(amount int) map[thor.Address]keySet {
	keys := make(map[thor.Address]keySet)
	for range amount {
		node := datagen.RandAddress()
		endorser := datagen.RandAddress()

		keys[node] = keySet{
			endorser: endorser,
			node:     node,
		}
	}
	return keys
}

type testStaker struct {
	addr thor.Address
	state *state.State
	*Staker
}

func (ts *testStaker) AddValidation(
	validator thor.Address,
	endorser thor.Address,
	period uint32,
	stake uint64,
) error {
	balance, err := ts.state.GetBalance(ts.addr)
	if err != nil {
		return err
	}
	newBalance := big.NewInt(0).Add(balance, big.NewInt(0).SetUint64(stake))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return err
	}
	if err := ts.Staker.AddValidation(validator, endorser, period, stake); err != nil {
		if ts.state.SetBalance(ts.addr, balance) != nil {
			return err
		}
	}
	return err
}

func (ts *testStaker) IncreaseStake(validator thor.Address, endorser thor.Address, amount uint64) error {
	balance, err := ts.state.GetBalance(ts.addr)
	if err != nil {
		return err
	}
	newBalance := big.NewInt(0).Add(balance, big.NewInt(0).SetUint64(amount))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return err
	}
	err = ts.Staker.IncreaseStake(validator, endorser, amount)
	if err != nil {
		if ts.state.SetBalance(ts.addr, balance) != nil {
			return err
		}
	}
	return err
}

func (ts *testStaker) WithdrawStake(validator thor.Address, endorser thor.Address, currentBlock uint32) (uint64, error) {
	amount, err := ts.Staker.WithdrawStake(validator, endorser, currentBlock)
	if err != nil {
		return 0, err
	}
	balance, err := ts.state.GetBalance(ts.addr)
	if err != nil {
		return 0, err
	}
	newBalance := big.NewInt(0).Sub(balance, big.NewInt(0).SetUint64(amount))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return 0, err
	}
	return amount, nil
}

func (ts *testStaker) AddDelegation(
	validator thor.Address,
	stake uint64,
	multiplier uint8,
	currentBlock uint32,
) (*big.Int, error) {
	balance, err := ts.state.GetBalance(ts.addr)
	if err != nil {
		return nil, err
	}
	newBalance := big.NewInt(0).Add(balance, big.NewInt(0).SetUint64(stake))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return nil, err
	}
	delegation, err := ts.Staker.AddDelegation(validator, stake, multiplier, currentBlock)
	if err != nil {
		if ts.state.SetBalance(ts.addr, balance) != nil {
			return nil, err
		}
	}
	return delegation, err
}

func (ts *testStaker) WithdrawDelegation(
	delegationID *big.Int,
	currentBlock uint32,
) (uint64, error) {
	amount, err := ts.Staker.WithdrawDelegation(delegationID, currentBlock)
	if err != nil {
		return amount, err
	}
	balance, err := ts.state.GetBalance(ts.addr)
	if err != nil {
		return 0, err
	}
	newBalance := big.NewInt(0).Sub(balance, big.NewInt(0).SetUint64(amount))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return 0, err
	}
	return amount, nil
}

func newStaker(t *testing.T, amount int, maxValidators int64, initialise bool) (*testStaker, uint64) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	keys := createKeys(amount)
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	stakerAddr := thor.BytesToAddress([]byte("stkr"))

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators)))
	stakerImpl := New(stakerAddr, st, param, nil)
	staker :=&testStaker{
		addr:  stakerAddr,
		state: st,
		Staker: stakerImpl,
	}

	totalStake := uint64(0)
	if initialise {
		for _, key := range keys {
			stake := RandomStake()
			totalStake += stake
			if err := staker.AddValidation(key.node, key.endorser, uint32(360)*24*15, stake); err != nil {
				t.Fatal(err)
			}
		}
		transitioned, err := staker.transition(0)
		assert.NoError(t, err)
		assert.True(t, transitioned)
	}

	return &testStaker{
		addr:  stakerAddr,
		state: st,
		Staker: stakerImpl,
	}, totalStake
}

func TestStaker_TotalStake(t *testing.T) {
	staker, totalStaked := newStaker(t, 0, 14, false)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]uint64)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stakeAmount) // false
		assert.NoError(t, err)
		stakes[addr] = stakeAmount
		totalStaked += stakeAmount
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		require.NoError(t, err)
		staked, weight, err := staker.LockedStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
		assert.Equal(t, totalStaked, weight)
	}

	for id, stake := range stakes {
		exit, err := staker.validationService.ExitValidator(id)
		require.NoError(t, err)

		// exit the aggregation too
		aggExit, err := staker.aggregationService.Exit(id)
		require.NoError(t, err)

		// Update global totals
		err = staker.globalStatsService.ApplyExit(exit, aggExit)
		require.NoError(t, err)

		totalStaked -= stake
		staked, weight, err := staker.LockedStake()
		assert.Nil(t, err)
		assert.Equal(t, totalStaked, staked)
		assert.Equal(t, totalStaked, weight)
	}
}

func TestStaker_TotalStake_Withdrawal(t *testing.T) {
	staker, _ := newStaker(t, 0, 14, false)

	addr := datagen.RandAddress()
	stakeAmount := RandomStake()
	period := uint32(360) * 24 * 15
	err := staker.AddValidation(addr, addr, period, stakeAmount)
	assert.NoError(t, err)

	queuedStake, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, queuedStake)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr, addr, 10)
	assert.NoError(t, err)

	lockedVET, lockedWeight, err := staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, lockedVET)
	assert.Equal(t, stakeAmount, lockedWeight)

	queuedStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), queuedStake)

	exit, err := staker.validationService.ExitValidator(addr)
	require.NoError(t, err)

	// exit the aggregation too
	aggExit, err := staker.aggregationService.Exit(addr)
	require.NoError(t, err)

	// Update global totals
	err = staker.globalStatsService.ApplyExit(exit, aggExit)
	require.NoError(t, err)

	lockedVET, lockedWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), lockedVET)
	assert.Equal(t, uint64(0), lockedWeight)

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stakeAmount, validator.CooldownVET)

	withdrawableAmount, err := staker.GetWithdrawable(addr, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, withdrawableAmount)

	withdrawnAmount, err := staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, stakeAmount, withdrawnAmount)

	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.WithdrawableVET)

	lockedVET, lockedWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), lockedVET)
	assert.Equal(t, uint64(0), lockedWeight)

	queuedStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), queuedStake)
}

func TestStaker_AddValidation_MinimumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooLow := MinStakeVET - 1
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, tooLow)
	assert.ErrorContains(t, err, "stake is below minimum")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MaximumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooHigh := MaxStakeVET + 1
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, tooHigh)
	assert.ErrorContains(t, err, "stake is above maximum")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MaxStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MaximumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, MinStakeVET)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MinimumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, MinStakeVET)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), 100, MinStakeVET)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_Duplicate(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := uint64(25e6)
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
		_, err := staker.validationService.GetValidation(loopID)
		assert.NoError(t, err)
		assert.Equal(t, expectedOrder[i], loopID)

		next, err := staker.Next(loopID)
		assert.NoError(t, err)
		loopID = next
	}

	// activating validations should continue to set the correct head of the queue
	loopID = first
	for range 99 {
		_, err := staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		first, err = staker.FirstQueued()
		assert.NoError(t, err)
		previous, err := staker.GetValidation(loopID)
		assert.NoError(t, err)
		current, err := staker.GetValidation(first)
		assert.NoError(t, err)
		assert.True(t, previous.LockedVET >= current.LockedVET)
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

	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr3, addr3, uint32(360)*24*30, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr4, addr4, uint32(360)*24*14, stake)
	assert.Error(t, err, "period is out of boundaries")

	validator, err = staker.GetValidation(addr4)
	assert.NoError(t, err)
	assert.Nil(t, validator)
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

	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	_, err = staker.Housekeep(180)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr3, addr3, uint32(360)*24*30, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr3)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr4, addr4, uint32(360)*24*14, stake)
	assert.Error(t, err, "period is out of boundaries")

	validator, err = staker.GetValidation(addr4)
	assert.NoError(t, err)
	assert.Nil(t, validator)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)

	_, err = staker.Housekeep(180 * 2)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validation.StatusActive, validator.Status)
}

func TestStaker_Get_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	id := datagen.RandAddress()
	validator, err := staker.GetValidation(id)
	assert.NoError(t, err)
	assert.Nil(t, validator)
}

func TestStaker_Get(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr)
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

	active, queued, err := staker.GetValidationsNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), active)
	assert.Equal(t, uint64(3), queued)

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	active, queued, err = staker.GetValidationsNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), active)
	assert.Equal(t, uint64(2), queued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	active, queued, err = staker.GetValidationsNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), active)
	assert.Equal(t, uint64(1), queued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	active, queued, err = staker.GetValidationsNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), active)
	assert.Equal(t, uint64(0), queued)

	err = staker.SignalExit(addr, addr, 10)
	assert.NoError(t, err)

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, uint64(0), validator.WithdrawableVET)

	active, queued, err = staker.GetValidationsNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), active)
	assert.Equal(t, uint64(0), queued)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
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
	assert.True(t, queued.IsZero())

	// add the validator
	period := uint32(360) * 24 * 15
	err = staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// verify queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, addr, queued)

	// withraw queued
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)

	// verify removed queued
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.True(t, queued.IsZero())
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// increase stake queued
	expectedStake := 1000 + stake
	err = staker.IncreaseStake(addr, addr, 1000)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	newAmount := validator.QueuedVET + validator.LockedVET
	assert.Equal(t, newAmount, expectedStake)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, validator.Status, validation.StatusQueued)
	assert.Equal(t, validator.QueuedVET, expectedStake)
	assert.Equal(t, uint64(0), validator.Weight)
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	err = staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, addr)

	// increase stake queued
	increaseBy := uint64(1000)
	err = staker.IncreaseStake(addr1, addr1, increaseBy)
	assert.NoError(t, err)
	expectedIncreaseStake := stake + increaseBy

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, addr)
	entry, err := staker.GetValidation(queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, uint64(0), entry.Weight)

	queuedAddr, err := staker.Next(queued)
	assert.NoError(t, err)
	assert.Equal(t, queuedAddr, addr1)
	entry, err = staker.GetValidation(queuedAddr)
	assert.NoError(t, err)
	assert.Equal(t, expectedIncreaseStake, entry.QueuedVET)
	assert.Equal(t, uint64(0), entry.Weight)

	queuedAddr, err = staker.Next(queuedAddr)
	assert.NoError(t, err)
	assert.Equal(t, queuedAddr, addr2)
	entry, err = staker.GetValidation(queuedAddr)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, uint64(0), entry.Weight)
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	err = staker.AddValidation(addr1, addr1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	err = staker.AddValidation(addr2, addr2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// verify order
	queued, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, addr)

	// increase stake queued
	decreaseBy := uint64(1000)
	err = staker.DecreaseStake(addr1, addr1, decreaseBy)
	assert.NoError(t, err)

	expectedDecreaseStake := stake - decreaseBy

	// verify order after increasing stake
	queued, err = staker.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, queued, addr)
	entry, err := staker.GetValidation(queued)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, uint64(0), entry.Weight)
	next, err := staker.Next(queued)
	assert.NoError(t, err)
	assert.Equal(t, next, addr1)
	entry, err = staker.GetValidation(next)
	assert.NoError(t, err)
	assert.Equal(t, expectedDecreaseStake, entry.QueuedVET)
	assert.Equal(t, uint64(0), entry.Weight)

	next, err = staker.Next(next)
	assert.NoError(t, err)
	assert.Equal(t, next, addr2)
	entry, err = staker.GetValidation(next)
	assert.NoError(t, err)
	assert.Equal(t, stake, entry.QueuedVET)
	assert.Equal(t, uint64(0), entry.Weight)
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
	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	// increase stake of an active validator
	expectedStake := 1000 + stake
	err = staker.IncreaseStake(addr, addr, 1000)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	newStake := validator.QueuedVET + validator.LockedVET
	assert.Equal(t, expectedStake, newStake)

	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.LockedVET+validator.QueuedVET)
	assert.Equal(t, validator.LockedVET, validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.Weight)
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
	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	validator2, err := staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator2.Status)
	assert.Equal(t, stake, validator2.QueuedVET)
	assert.Equal(t, uint64(0), validator2.Weight)
	queuedVET, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)

	// increase stake of an active validator
	expectedStake := 1000 + stake
	err = staker.IncreaseStake(addr, addr, 1000)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	newStake := validator.QueuedVET + validator.LockedVET
	assert.Equal(t, expectedStake, newStake)

	// the queued stake also increases
	queuedVET, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+1000, queuedVET)

	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.LockedVET+validator.QueuedVET)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, validator.Weight)

	// verify queued stake is still the same as before the increase
	queuedVET, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)

	// decrease stake
	decreaseAmount := uint64(2500)
	decreasedAmount := expectedStake - decreaseAmount
	err = staker.DecreaseStake(addr, addr, decreaseAmount)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, decreaseAmount, validator.PendingUnlockVET)
	assert.Equal(t, expectedStake, validator.Weight)

	queuedVET, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)

	// verify queued weight is decreased
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2500), validator.WithdrawableVET)
	assert.Equal(t, decreasedAmount, validator.Weight)

	// verify queued stake is still the same as before the decrease
	queuedVET, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVET)
}

func TestStaker_DecreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStakeVET
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw is empty
	assert.Equal(t, validator.WithdrawableVET, uint64(0))

	// decrease stake of an active validator
	decrease := uint64(1000)
	expectedStake := stake - decrease
	err = staker.DecreaseStake(addr, addr, decrease)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	newStake := validator.QueuedVET + validator.LockedVET
	assert.Equal(t, stake, newStake)
	assert.Equal(t, decrease, validator.PendingUnlockVET)

	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, decrease, validator.PendingUnlockVET)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1000), validator.WithdrawableVET)
	assert.Equal(t, expectedStake, validator.Weight)
}

func TestStaker_DecreaseActiveThenExit(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStakeVET
	period := uint32(360) * 24 * 15

	// add the validator
	err := staker.AddValidation(addr, addr, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw is empty
	assert.Equal(t, validator.WithdrawableVET, uint64(0))

	// decrease stake of an active validator
	decrease := uint64(1000)
	expectedStake := stake - decrease
	err = staker.DecreaseStake(addr, addr, decrease)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, decrease, validator.PendingUnlockVET)

	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, decrease, validator.PendingUnlockVET)
	assert.Equal(t, stake, validator.Weight)

	// verify withdraw amount decrease
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1000), validator.WithdrawableVET)
	assert.Equal(t, expectedStake, validator.LockedVET)
	assert.Equal(t, uint64(0), validator.PendingUnlockVET)

	assert.NoError(t, staker.SignalExit(addr, addr, 129600))

	_, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(1000), validator.WithdrawableVET)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, expectedStake, validator.CooldownVET)
	assert.Equal(t, uint64(0), validator.QueuedVET)
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.SignalExit(addr, addr, 10)
	assert.NoError(t, err)

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	_, err = staker.Housekeep(period + thor.CooldownPeriod())
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)

	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), amount)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	// activate the validator
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// housekeep the validator
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	// withdraw the stake
	amount, err := staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), amount)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.False(t, validator == nil)

	assert.NoError(t, staker.SignalExit(addr, addr, 10))

	// housekeep the validator
	_, err = staker.Housekeep(period * 1)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, uint64(0), validator.Weight)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, uint64(0), validator.QueuedVET)

	// withdraw the stake
	withdrawAmount, err := staker.WithdrawStake(addr, addr, period*2+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawAmount)
	validator, err = staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
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

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	addr := datagen.RandAddress()
	_, err := staker.validationService.ExitValidator(addr)
	assert.ErrorContains(t, err, "failed to get existing validator")
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
	err = staker.SignalExit(addr, addr, 10)
	assert.NoError(t, err)

	exit, err := staker.validationService.ExitValidator(addr)
	require.NoError(t, err)

	// exit the aggregation too
	aggExit, err := staker.aggregationService.Exit(addr)
	require.NoError(t, err)

	// Update global totals
	err = staker.globalStatsService.ApplyExit(exit, aggExit)
	require.NoError(t, err)

	validator, err := staker.GetValidation(addr)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, uint64(0), validator.Weight)
	assert.Equal(t, stake, validator.CooldownVET)

	withdrawale, err := staker.WithdrawStake(addr, addr, period)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawale)

	withdrawale, err = staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, stake, withdrawale)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	added := make(map[thor.Address]bool)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, uint32(360)*24*15, stake)
		assert.NoError(t, err)
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		added[addr] = true
	}

	leaderGroup, err := staker.LeaderGroup()
	assert.NoError(t, err)

	leaders := make(map[thor.Address]bool)
	for _, leader := range leaderGroup {
		leaders[leader.Address] = true
	}

	for addr := range added {
		assert.Contains(t, leaders, addr)
	}
}

func TestStaker_Next_Empty(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	id := datagen.RandAddress()
	next, err := staker.Next(id)
	assert.NoError(t, err)
	assert.True(t, next.IsZero())
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
		_, err := staker.GetValidation(current)
		assert.NoError(t, err)
		assert.Equal(t, queuedGroup[i], current)

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
	stakerImpl := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)
	staker := &testStaker{
		Staker: stakerImpl,
		addr:   thor.BytesToAddress([]byte("stkr")),
		state:  st,
	}
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(3)))

	for range 3 {
		err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
		assert.NoError(t, err)
	}

	transitioned, err := staker.transition(0)
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validations after initialisation
	err = staker.AddValidation(addr, addr, uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)

	staker, _ = newStaker(t, 101, 101, true)
	first, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.False(t, first.IsZero())

	expectedLength := uint64(101)
	length, err := staker.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, expectedLength, length)
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

	_, err = staker.Housekeep(0)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	validator, err = staker.GetValidation(addr2)
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

	totalLocked, totalWeight, err := staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalLocked)
	assert.Equal(t, uint64(0), totalWeight)
	assert.Equal(t, stake, totalQueued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalQueued)
	assert.Equal(t, stake, totalLocked)
	assert.Equal(t, stake, totalWeight)

	// Add second validator
	err = staker.AddValidation(addr2, addr2, period, stake)
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, totalLocked)
	assert.Equal(t, stake, totalWeight)
	assert.Equal(t, stake, totalQueued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalQueued)
	assert.Equal(t, stake*2, totalLocked)
	assert.Equal(t, stake*2, totalWeight)

	// Add third validator
	err = staker.AddValidation(addr3, addr3, period, stake)
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake*2, totalLocked)
	assert.Equal(t, stake*2, totalWeight)
	assert.Equal(t, stake, totalQueued)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalQueued)
	assert.Equal(t, stake*3, totalLocked)
	assert.Equal(t, stake*3, totalWeight)

	// disable auto renew
	err = staker.SignalExit(addr1, addr1, 10)
	assert.NoError(t, err)

	// first should be on cooldown
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalQueued)
	assert.Equal(t, stake*2, totalLocked)
	assert.Equal(t, stake*2, totalWeight)

	_, err = staker.Housekeep(period + thor.CooldownPeriod())
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, uint64(0), validator.WithdrawableVET)
	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	totalQueued, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalQueued)
	assert.Equal(t, stake*2, totalLocked)
	assert.Equal(t, stake*2, totalWeight)
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
	err = staker.SignalExit(addr1, addr1, 10)
	assert.NoError(t, err)
	err = staker.SignalExit(addr2, addr2, 10)
	assert.NoError(t, err)
	err = staker.SignalExit(addr3, addr3, 10)
	assert.NoError(t, err)

	id, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, addr1, id)
	next, err := staker.Next(id)
	assert.NoError(t, err)
	assert.Equal(t, addr2, next)

	totalLocked, totalWeight, err := staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake*3, totalLocked)
	assert.Equal(t, stake*3, totalWeight)

	// housekeep and exit validator 1
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, uint64(0), validator.WithdrawableVET)
	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	totalLocked, _, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.True(t, totalLocked > 0)

	// housekeep and exit validator 2
	_, err = staker.Housekeep(period + thor.EpochLength())
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, uint64(0), validator.LockedVET)
	assert.Equal(t, stake, validator.CooldownVET)
	assert.Equal(t, uint64(0), validator.WithdrawableVET)

	// housekeep and exit validator 3
	_, err = staker.Housekeep(period + thor.EpochLength()*2)
	assert.NoError(t, err)

	totalLocked, totalWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totalLocked)
	assert.Equal(t, uint64(0), totalWeight)

	withdrawable, err := staker.WithdrawStake(addr1, addr1, period+thor.CooldownPeriod())
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
	err = staker.SignalExit(addr1, addr1, 10)
	assert.NoError(t, err)
	err = staker.SignalExit(addr2, addr2, 10)
	assert.NoError(t, err)
	err = staker.SignalExit(addr3, addr3, 10)
	assert.NoError(t, err)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)

	_, err = staker.Housekeep(period + thor.EpochLength())
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	validator, err = staker.GetValidation(addr2)
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
	err = staker.SignalExit(addr2, addr2, 10)
	assert.NoError(t, err)
	err = staker.SignalExit(addr3, addr3, 259200)
	assert.NoError(t, err)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator2, err := staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator2.Status)
	validator3, err := staker.GetValidation(addr3)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator3.Status)
	assert.NoError(t, err)
	validator1, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator1.Status)

	// renew validator 1 for next period
	_, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	assert.NoError(t, staker.SignalExit(addr1, addr1, 259201))

	// housekeep -> validator 3 placed intention to leave first
	_, err = staker.Housekeep(period * 3)
	assert.NoError(t, err)
	validator3, err = staker.GetValidation(addr3)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator3.Status)
	assert.NoError(t, err)
	validator1, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator1.Status)

	// housekeep -> validator 1 waited 1 epoch after validator 3
	_, err = staker.Housekeep(period*3 + thor.EpochLength())
	assert.NoError(t, err)
	validator1, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator1.Status)
}

func TestStaker_Housekeep_RecalculateIncrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MinStakeVET
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.IncreaseStake(addr1, addr1, 1)
	assert.NoError(t, err)

	// housekeep half way through the period, validator's locked vet should not change
	_, err = staker.Housekeep(period / 2)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, uint64(1), validator.QueuedVET)
	assert.Equal(t, stake, validator.Weight)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	stake += 1
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, uint64(0))
	assert.Equal(t, validator.QueuedVET, uint64(0))
}

func TestStaker_Housekeep_RecalculateDecrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStakeVET
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	decrease := uint64(1)
	err = staker.DecreaseStake(addr1, addr1, decrease)
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	assert.Equal(t, decrease, validator.PendingUnlockVET)

	block = uint32(360) * 24 * 15
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	expectedStake := stake - decrease
	assert.Equal(t, expectedStake, validator.LockedVET)
	assert.Equal(t, expectedStake, validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, uint64(1))
}

func TestStaker_Housekeep_DecreaseThenWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStakeVET
	period := uint32(360) * 24 * 15

	// auto renew is turned on
	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.DecreaseStake(addr1, addr1, 1)
	assert.NoError(t, err)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	assert.Equal(t, validator.PendingUnlockVET, uint64(1))

	block = uint32(360) * 24 * 15
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	stake -= 1
	assert.Equal(t, validator.LockedVET, stake)
	assert.Equal(t, stake, validator.Weight)
	assert.Equal(t, validator.WithdrawableVET, uint64(1))

	withdrawAmount, err := staker.WithdrawStake(addr1, addr1, block+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, validator.WithdrawableVET, withdrawAmount)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), validator.WithdrawableVET)

	// verify that validator is still present and active
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, stake, validator.Weight)
	activeValidator, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, addr1, activeValidator)
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

	err = staker.DecreaseStake(addr1, addr1, 1)
	assert.NoError(t, err)

	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, uint64(1), validator.PendingUnlockVET)

	err = staker.DecreaseStake(addr1, addr1, 1)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake, validator.LockedVET)
	assert.Equal(t, validator.PendingUnlockVET, uint64(2))

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, stake-2, validator.LockedVET)
	assert.Equal(t, uint64(2), validator.WithdrawableVET)
	assert.Equal(t, uint64(0), validator.CooldownVET)
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
	err = staker.SignalExit(addr1, addr1, 10)
	assert.NoError(t, err)

	exitBlock := uint32(360) * 24 * 15
	_, err = staker.Housekeep(exitBlock)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)

	_, err = staker.Housekeep(exitBlock + 8640)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
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

	_, err = staker.Housekeep(exitBlock + 8640 + 360)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(addr1)
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
		SignalExit(addr1, addr1, 10).
		SignalExit(addr2, addr2, 10).
		AssertGlobalWithdrawable(0).
		Housekeep(period).
		AssertGlobalWithdrawable(0).
		AssertGlobalCooldown(stake).
		AssertLeaderGroupSize(1).
		AssertFirstActive(addr2)

	assertValidation(t, staker, addr1).Status(validation.StatusExit)
	assertValidation(t, staker, addr2).Status(validation.StatusActive)

	block := period + thor.EpochLength()
	newTestSequence(t, staker).
		AssertGlobalCooldown(stake).
		AssertGlobalWithdrawable(0).
		Housekeep(block).
		AssertGlobalCooldown(stake * 2).
		AssertGlobalWithdrawable(0).
		AssertLeaderGroupSize(0).
		AssertFirstActive(thor.Address{})

	assertValidation(t, staker, addr2).Status(validation.StatusExit)

	newTestSequence(t, staker).
		AddValidation(addr3, addr3, period, stake).
		ActivateNext(block).
		SignalExit(addr3, addr3, block+period-1).
		AssertFirstActive(addr3).
		AssertLeaderGroupSize(1)

	assertValidation(t, staker, addr3).Status(validation.StatusActive)

	block = block + period
	newTestSequence(t, staker).Housekeep(block).AssertGlobalWithdrawable(0).AssertGlobalCooldown(stake * 3)

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
	assert.Equal(t, uint64(3), queuedValidators)

	leaderGroupSize, err := staker.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), leaderGroupSize)

	block := uint32(360) * 24 * 13
	_, err = staker.Housekeep(block)
	assert.NoError(t, err)
	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	validator1, err := staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator1.Status)
	validator2, err := staker.GetValidation(addr3)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator2.Status)
	leaderGroupSize, err = staker.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), leaderGroupSize)
	queuedValidators, err = staker.validationService.QueuedGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), queuedValidators)
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

	val, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val.Status)
	assert.Equal(t, uint64(0), val.LockedVET)
	assert.Equal(t, uint64(0), val.Weight)
	assert.Equal(t, uint64(0), val.WithdrawableVET)
	assert.Equal(t, uint64(0), val.QueuedVET)
}

func TestStaker_IncreaseStake_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := uint32(360) * 24 * 15

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)

	val, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, stake, val.LockedVET)

	assert.NoError(t, staker.IncreaseStake(addr1, addr1, 100))
	withdrawAmount, err := staker.WithdrawStake(addr1, addr1, period+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), withdrawAmount)

	val, err = staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, stake, val.LockedVET)
	assert.Equal(t, stake, val.Weight)
	assert.Equal(t, uint64(0), val.WithdrawableVET)
	assert.Equal(t, uint64(0), val.QueuedVET)
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
	staker.IncreaseDelegatorsReward(proposerAddr, reward, 10)

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

	val, err := staker.GetValidation(proposerAddr)
	assert.NoError(t, err)
	assert.NotNil(t, val, "validation not found")
	periods, err := val.CompletedIterations(period - 1)

	assert.NoError(t, err)
	assert.Equal(t, uint32(0), periods)

	_, err = staker.Housekeep(period)
	assert.NoError(t, err)

	val, err = staker.GetValidation(proposerAddr)
	assert.NoError(t, err)
	assert.NotNil(t, val, "validation not found")
	periods, err = val.CompletedIterations(period)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), periods)
}

func TestStaker_MultipleUpdates_CorrectWithdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()
	initialStake := RandomStake()
	increases := uint64(0)
	decreases := uint64(0)
	withdrawnTotal := uint64(0)
	thousand := uint64(1000)
	fiveHundred := uint64(500)

	period := uint32(360) * 24 * 15

	// QUEUED
	err := staker.AddValidation(acc, acc, period, initialStake)
	assert.NoError(t, err)

	increases += thousand
	assert.NoError(t, staker.IncreaseStake(acc, acc, thousand))
	// 1st decrease
	decreases += fiveHundred
	assert.NoError(t, staker.DecreaseStake(acc, acc, fiveHundred))

	validator, err := staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	// 1st STAKING PERIOD
	_, err = staker.Housekeep(period)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)
	expected := initialStake - decreases
	expected += increases
	assert.Equal(t, expected, validator.LockedVET)

	// See `1st decrease` -> validator should be able withdraw the decrease amount
	withdraw, err := staker.WithdrawStake(acc, acc, period+1)
	assert.NoError(t, err)
	assert.Equal(t, withdraw, fiveHundred)
	withdrawnTotal += withdraw

	expectedLocked := initialStake - decreases
	expectedLocked += increases
	validator, err = staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, expectedLocked, validator.LockedVET)

	// 2nd decrease
	decreases += thousand
	assert.NoError(t, staker.DecreaseStake(acc, acc, thousand))
	increases += fiveHundred
	assert.NoError(t, staker.IncreaseStake(acc, acc, fiveHundred))

	// 2nd STAKING PERIOD
	_, err = staker.Housekeep(period * 2)
	assert.NoError(t, err)
	validator, err = staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, validator.Status)

	// See `2nd decrease` -> validator should be able withdraw the decrease amount
	withdraw, err = staker.WithdrawStake(acc, acc, period*2+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, thousand, withdraw)
	withdrawnTotal += withdraw

	assert.NoError(t, staker.SignalExit(acc, acc, period*2))

	// EXITED
	_, err = staker.Housekeep(period * 3)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	expectedLocked = initialStake - decreases
	expectedLocked += increases
	validator, err = staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, expectedLocked, validator.CooldownVET)

	withdraw, err = staker.WithdrawStake(acc, acc, period*3+thor.CooldownPeriod())
	assert.NoError(t, err)
	withdrawnTotal += withdraw
	depositTotal := initialStake + increases
	assert.Equal(t, depositTotal, withdrawnTotal)
}

func Test_GetValidatorTotals_ValidatorExiting(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	delegationID := new(big.Int)
	dStake := stakes.NewWeightedStakeWithMultiplier(MinStakeVET, 255)
	newTestSequence(t, staker).AddDelegation(validator.ID, dStake.VET, 255, delegationID, 10)

	_, err := staker.aggregationService.GetAggregation(validators[0].ID)
	assert.NoError(t, err)

	vStake := stakes.NewWeightedStakeWithMultiplier(validators[0].LockedVET, validation.Multiplier)

	newTestSequence(t, staker).AssertTotals(validator.ID, &validation.Totals{
		TotalQueuedStake:  dStake.VET,
		TotalLockedWeight: vStake.Weight,
		TotalLockedStake:  vStake.VET,
		TotalExitingStake: 0,
		NextPeriodWeight:  vStake.Weight + dStake.Weight + vStake.Weight,
	})

	vStake.Weight += validators[0].LockedVET
	newTestSequence(t, staker).
		AssertGlobalWithdrawable(0).
		Housekeep(validator.Period).
		AssertGlobalWithdrawable(0).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  vStake.VET + dStake.VET,
			TotalLockedWeight: vStake.Weight + dStake.Weight,
			NextPeriodWeight:  vStake.Weight + dStake.Weight,
		}).
		SignalExit(validator.ID, validator.Endorser, 10).
		AssertGlobalWithdrawable(0).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  vStake.VET + dStake.VET,
			TotalLockedWeight: vStake.Weight + dStake.Weight,
			TotalExitingStake: vStake.VET + dStake.VET,
			NextPeriodWeight:  0,
		})
}

func Test_GetValidatorTotals_DelegatorExiting_ThenValidator(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	_, err := staker.aggregationService.GetAggregation(validator.ID)
	require.NoError(t, err)
	vStake := stakes.NewWeightedStakeWithMultiplier(validators[0].LockedVET, validation.Multiplier)
	dStake := stakes.NewWeightedStakeWithMultiplier(MinStakeVET, 255)

	delegationID := new(big.Int)
	newTestSequence(t, staker).AddDelegation(validator.ID, dStake.VET, 255, delegationID, 10)

	newTestSequence(t, staker).AssertTotals(validator.ID, &validation.Totals{
		TotalQueuedStake:  dStake.VET,
		TotalLockedWeight: vStake.Weight,
		TotalLockedStake:  vStake.VET,
		NextPeriodWeight:  vStake.Weight + dStake.Weight + vStake.VET,
	})

	staker.globalStatsService.GetWithdrawableStake()

	vStake.Weight += validators[0].LockedVET
	newTestSequence(t, staker).
		AssertGlobalWithdrawable(0).
		Housekeep(validator.Period).
		AssertGlobalWithdrawable(0).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  vStake.VET + dStake.VET,
			TotalLockedWeight: vStake.Weight + dStake.Weight,
			TotalQueuedStake:  0,
			NextPeriodWeight:  vStake.Weight + dStake.Weight,
		}).
		SignalDelegationExit(delegationID, validator.Period+1).
		AssertGlobalWithdrawable(0).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  vStake.VET + dStake.VET,
			TotalLockedWeight: vStake.Weight + dStake.Weight,
			TotalQueuedStake:  0,
			TotalExitingStake: dStake.VET,
			NextPeriodWeight:  vStake.Weight - vStake.VET,
		}).
		SignalExit(validator.ID, validator.Endorser, validator.Period+1).
		AssertGlobalWithdrawable(0).
		AssertGlobalCooldown(0).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  vStake.VET + dStake.VET,
			TotalLockedWeight: vStake.Weight + dStake.Weight,
			TotalExitingStake: vStake.VET + dStake.VET,
			NextPeriodWeight:  vStake.Weight + dStake.Weight - vStake.Weight - dStake.Weight,
		}).
		Housekeep(validator.Period*2).
		AssertGlobalWithdrawable(dStake.VET).
		AssertGlobalCooldown(validator.LockedVET).
		AssertTotals(validator.ID, &validation.Totals{
			TotalLockedStake:  0,
			TotalLockedWeight: 0,
			TotalExitingStake: 0,
			NextPeriodWeight:  0,
		})
}

func Test_Validator_Decrease_Exit_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()

	originalStake := uint64(3) * MinStakeVET
	err := staker.AddValidation(acc, acc, thor.LowStakingPeriod(), originalStake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// Decrease stake
	decrease := uint64(2) * MinStakeVET
	err = staker.DecreaseStake(acc, acc, decrease)
	assert.NoError(t, err)

	// Turn off auto-renew  - can't decrease if auto-renew is false
	err = staker.SignalExit(acc, acc, thor.LowStakingPeriod()-1)
	assert.NoError(t, err)

	// Housekeep, should exit the validator
	_, err = staker.Housekeep(thor.LowStakingPeriod())
	assert.NoError(t, err)

	validator, err := staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, validator.Status)
	assert.Equal(t, originalStake, validator.CooldownVET)
}

func Test_Validator_Decrease_SeveralTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	acc := datagen.RandAddress()

	originalStake := uint64(3) * MinStakeVET
	err := staker.AddValidation(acc, acc, thor.LowStakingPeriod(), originalStake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// Decrease stake - ok 75m - 25m = 50m
	err = staker.DecreaseStake(acc, acc, MinStakeVET)
	assert.NoError(t, err)

	// Decrease stake - ok 50m - 25m = 25m
	err = staker.DecreaseStake(acc, acc, MinStakeVET)
	assert.NoError(t, err)

	// Decrease stake - should fail, min stake is 25m
	err = staker.DecreaseStake(acc, acc, MinStakeVET)
	assert.ErrorContains(t, err, "next period stake is lower than minimum stake")
}

func Test_Validator_IncreaseDecrease_Combinations(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	acc := datagen.RandAddress()

	// Add & activate validator
	err := staker.AddValidation(acc, acc, thor.LowStakingPeriod(), MinStakeVET)
	assert.NoError(t, err)

	// Increase and decrease - both should be okay since we're only dealing with QueuedVET
	assert.NoError(t, staker.IncreaseStake(acc, acc, MinStakeVET)) // 25m + 25m = 50m
	assert.NoError(t, staker.DecreaseStake(acc, acc, MinStakeVET)) // 25m - 50m = 25m

	// Activate the validator.
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// Withdraw the previous decrease amount
	withdrawal, err := staker.WithdrawStake(acc, acc, 0)
	assert.NoError(t, err)
	assert.Equal(t, MinStakeVET, withdrawal, "withdraw should be 0 since we are withdrawing from pending locked")

	// Assert previous increase/decrease had no effect since they requested the same amount
	val, err := staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, MinStakeVET, val.LockedVET)

	// Increase stake (ok): 25m + 25m = 50m
	assert.NoError(t, staker.IncreaseStake(acc, acc, MinStakeVET))
	// Decrease stake (NOT ok): 25m - 25m = 0. The Previous increase is not applied since it is still currently withdrawable.
	assert.ErrorContains(t, staker.DecreaseStake(acc, acc, MinStakeVET), "next period stake is lower than minimum stake")
	// Instantly withdraw - This is bad, it pulls from the QueuedVET, which means total stake later will be 0.
	// The decrease previously marked as okay since the current TVL + pending TVL was greater than the min stake.
	withdraw1, err := staker.WithdrawStake(acc, acc, 0)
	assert.NoError(t, err)
	assert.Equal(t, MinStakeVET, withdraw1, "withdraw should be 0 since we are withdrawing from pending locked")

	// Housekeep, should move pending locked to locked, and pending withdraw to withdrawable
	_, err = staker.Housekeep(thor.LowStakingPeriod())
	assert.NoError(t, err)

	// Withdraw again
	withdraw2, err := staker.WithdrawStake(acc, acc, thor.LowStakingPeriod()+thor.CooldownPeriod())
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdraw2)

	validator, err := staker.GetValidation(acc)
	assert.NoError(t, err)
	assert.True(t, validator.LockedVET >= MinStakeVET, "locked vet should be greater than or equal to min stake")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMaster(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	address := datagen.RandAddress()
	err := staker.AddValidation(address, datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)

	err = staker.AddValidation(address, datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
	assert.Error(t, err, "validator already exists")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMasterAfterExit(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	master := datagen.RandAddress()
	endorser := datagen.RandAddress()
	err := staker.AddValidation(master, endorser, uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)

	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	err = staker.SignalExit(master, endorser, 10)
	assert.NoError(t, err)

	_, err = staker.validationService.ExitValidator(master)
	assert.NoError(t, err)

	err = staker.AddValidation(master, datagen.RandAddress(), uint32(360)*24*15, MinStakeVET)
	assert.Error(t, err, "validator already exists")
}

func TestStaker_HasDelegations(t *testing.T) {
	staker, _ := newStaker(t, 1, 1, true)

	validator, err := staker.FirstActive()
	assert.NoError(t, err)
	dStake := delegationStake()
	stakingPeriod := thor.MediumStakingPeriod()

	delegationID := big.NewInt(0)
	newTestSequence(t, staker).
		// no delegations, should be false
		AssertHasDelegations(validator, false).
		// delegation added, housekeeping not performed, should be false
		AddDelegation(validator, dStake, 200, delegationID, 10).
		AssertHasDelegations(validator, false).
		AssertGlobalWithdrawable(0).
		// housekeeping performed, should be true
		Housekeep(stakingPeriod).
		AssertGlobalWithdrawable(0).
		AssertHasDelegations(validator, true).
		// signal exit, housekeeping not performed, should still be true
		SignalDelegationExit(delegationID, stakingPeriod*1).
		AssertGlobalWithdrawable(0).
		AssertHasDelegations(validator, true).
		// housekeeping performed, should be false
		Housekeep(stakingPeriod*2).
		AssertGlobalWithdrawable(dStake).
		AssertHasDelegations(validator, false)
}

func TestStaker_SetBeneficiary(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)

	master := datagen.RandAddress()
	endorser := datagen.RandAddress()
	beneficiary := datagen.RandAddress()

	testSetup := newTestSequence(t, staker)

	// add validation without a beneficiary
	testSetup.AddValidation(master, endorser, thor.MediumStakingPeriod(), MinStakeVET).ActivateNext(0)
	assertValidation(t, staker, master).Beneficiary(thor.Address{})

	// negative cases
	assert.ErrorContains(t, staker.SetBeneficiary(master, master, beneficiary), "endorser required")
	assert.ErrorContains(t, staker.SetBeneficiary(endorser, endorser, beneficiary), "validation does not exist")

	// set beneficiary, should be successful
	testSetup.SetBeneficiary(master, endorser, beneficiary)
	assertValidation(t, staker, master).Beneficiary(beneficiary)

	// remove the beneficiary
	testSetup.SetBeneficiary(master, endorser, thor.Address{})
	assertValidation(t, staker, master).Beneficiary(thor.Address{})
}

func getTestMaxLeaderSize(param *params.Params) uint64 {
	maxLeaderGroupSize, err := param.Get(thor.KeyMaxBlockProposers)
	if err != nil {
		panic(err)
	}
	return maxLeaderGroupSize.Uint64()
}

func TestStaker_TestWeights(t *testing.T) {
	staker, _ := newStaker(t, 1, 1, true)

	validator, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, val.Weight)

	// one active validator without delegations
	lStake, lWeight, err := staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err := staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, totals.TotalLockedStake)
	assert.Equal(t, val.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, val.Weight, totals.NextPeriodWeight)

	// one active validator without delegations, one queued delegator without delegations
	stake := MinStakeVET
	keys := createKeys(1)
	validator2 := thor.Address{}
	for _, key := range keys {
		validator2 = key.node
		if err := staker.AddValidation(key.node, key.endorser, uint32(360)*24*15, stake); err != nil {
			t.Fatal(err)
		}
	}

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, totals.TotalLockedStake)
	assert.Equal(t, val.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, val.Weight, totals.NextPeriodWeight)

	totals2, err := staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake, totals2.TotalQueuedStake)
	assert.Equal(t, stake, totals2.NextPeriodWeight)

	// active validator with queued delegation, queued validator
	delegationID := new(big.Int)
	dStake := stakes.NewWeightedStakeWithMultiplier(1, 255)
	newTestSequence(t, staker).AddDelegation(validator, dStake.VET, 255, delegationID, 10)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+dStake.VET, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, totals.TotalLockedStake)
	assert.Equal(t, val.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, dStake.VET, totals.TotalQueuedStake)
	assert.Equal(t, val.Weight+dStake.Weight+val.LockedVET, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake, totals2.TotalQueuedStake)
	assert.Equal(t, stake, totals2.NextPeriodWeight)

	// second delegator shouldn't multiply
	delegationID2 := big.NewInt(2)
	newTestSequence(t, staker).AddDelegation(validator, dStake.VET, 255, delegationID2, 10)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+dStake.VET*2, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, totals.TotalLockedStake)
	assert.Equal(t, val.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, dStake.VET*2, totals.TotalQueuedStake)
	assert.Equal(t, val.Weight+val.LockedVET+dStake.Weight*2, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake, totals2.TotalQueuedStake)
	assert.Equal(t, stake, totals2.NextPeriodWeight)

	// delegator on queued should multiply
	delegationID3 := big.NewInt(3)
	newTestSequence(t, staker).AddDelegation(validator2, dStake.VET, 255, delegationID3, 10)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+dStake.VET*3, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, totals.TotalLockedStake)
	assert.Equal(t, val.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, dStake.VET*2, totals.TotalQueuedStake)
	assert.Equal(t, val.Weight+val.LockedVET+dStake.Weight*2, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake+dStake.VET, totals2.TotalQueuedStake)
	assert.Equal(t, stake*2+dStake.Weight, totals2.NextPeriodWeight)

	// second delegator on queued should not multiply
	delegationID4 := big.NewInt(4)
	newTestSequence(t, staker).AddDelegation(validator2, dStake.VET, 255, delegationID4, 10)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+dStake.VET*4, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, totals.TotalLockedStake)
	assert.Equal(t, val.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, dStake.VET*2, totals.TotalQueuedStake)
	assert.Equal(t, val.Weight+val.LockedVET+dStake.Weight*2, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake+dStake.VET*2, totals2.TotalQueuedStake)
	assert.Equal(t, stake*2+dStake.Weight*2, totals2.NextPeriodWeight)

	// Housekeep, first validator should have both delegations active
	stakingPeriod := thor.MediumStakingPeriod()
	newTestSequence(t, staker).Housekeep(stakingPeriod)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET+dStake.VET*2, lStake)
	assert.Equal(t, val.LockedVET*2+dStake.Weight*2, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+dStake.VET*2, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET+dStake.VET*2, totals.TotalLockedStake)
	assert.Equal(t, val.LockedVET*2+dStake.Weight*2, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, val.LockedVET*2+dStake.Weight*2, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake+dStake.VET*2, totals2.TotalQueuedStake)
	assert.Equal(t, stake*2+dStake.Weight*2, totals2.NextPeriodWeight)

	// exit queued
	newTestSequence(t, staker).
		AssertGlobalWithdrawable(0).
		WithdrawDelegation(delegationID3, dStake.VET, 10).
		AssertGlobalWithdrawable(0)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET+dStake.VET*2, lStake)
	assert.Equal(t, val.LockedVET*2+dStake.Weight*2, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake+dStake.VET, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET+dStake.VET*2, totals.TotalLockedStake)
	assert.Equal(t, val.LockedVET*2+dStake.Weight*2, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, val.LockedVET*2+dStake.Weight*2, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake+dStake.VET, totals2.TotalQueuedStake)
	assert.Equal(t, stake*2+dStake.Weight, totals2.NextPeriodWeight)

	// exit second queued, multiplier should be one
	stakeIncrease := uint64(1000)
	newTestSequence(t, staker).WithdrawDelegation(delegationID4, dStake.VET, 10)
	newTestSequence(t, staker).IncreaseStake(validator, val.Endorser, stakeIncrease)
	newTestSequence(t, staker).AssertGlobalWithdrawable(0).
		Housekeep(stakingPeriod * 3).
		AssertGlobalWithdrawable(0)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	increasedLocked := val.LockedVET + stakeIncrease
	increasedLockedWeight := increasedLocked * 2
	assert.Equal(t, increasedLocked+dStake.VET*2, lStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight*2, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, increasedLocked+dStake.VET*2, totals.TotalLockedStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight*2, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight*2, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake, totals2.TotalQueuedStake)
	assert.Equal(t, stake, totals2.NextPeriodWeight)

	// exit first active, multiplier should not change
	newTestSequence(t, staker).SignalDelegationExit(delegationID, stakingPeriod*3)
	newTestSequence(t, staker).AssertGlobalWithdrawable(0).
		Housekeep(stakingPeriod * 4).
		AssertGlobalWithdrawable(dStake.VET)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, increasedLocked+dStake.VET, lStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, increasedLocked+dStake.VET, totals.TotalLockedStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake, totals2.TotalQueuedStake)
	assert.Equal(t, stake, totals2.NextPeriodWeight)

	// exit second active, multiplier should change to 1
	newTestSequence(t, staker).SignalDelegationExit(delegationID2, stakingPeriod*4)
	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, increasedLocked+dStake.VET, totals.TotalLockedStake)
	assert.Equal(t, increasedLockedWeight+dStake.Weight, totals.TotalLockedWeight)
	assert.Equal(t, dStake.VET, totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, increasedLockedWeight-increasedLocked, totals.NextPeriodWeight)

	newTestSequence(t, staker).AssertGlobalWithdrawable(dStake.VET).
		Housekeep(stakingPeriod * 5).
		AssertGlobalWithdrawable(dStake.VET * 2)
	assert.NoError(t, err)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, increasedLocked, lStake)
	assert.Equal(t, increasedLocked, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, increasedLocked, totals.TotalLockedStake)
	assert.Equal(t, increasedLocked, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, increasedLocked, totals.NextPeriodWeight)

	totals2, err = staker.GetValidationTotals(validator2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), totals2.TotalLockedStake)
	assert.Equal(t, uint64(0), totals2.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals2.TotalExitingStake)
	assert.Equal(t, stake, totals2.TotalQueuedStake)
	assert.Equal(t, stake, totals2.NextPeriodWeight)
}

func TestStaker_TestWeights_IncreaseStake(t *testing.T) {
	staker, _ := newStaker(t, 1, 1, true)

	validator, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, val.Weight)

	// one active validator without delegations
	lStake, lWeight, err := staker.LockedStake()
	baseStake := val.LockedVET
	assert.NoError(t, err)
	assert.Equal(t, baseStake, lStake)
	assert.Equal(t, baseStake, lWeight)

	qStake, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err := staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, baseStake, totals.NextPeriodWeight)

	// one active validator without delegations, increase stake, multiplier should be 0, increase stake should be queued
	stakeIncrease := uint64(1500)
	newTestSequence(t, staker).IncreaseStake(validator, val.Endorser, stakeIncrease)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeIncrease, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, stakeIncrease, totals.TotalQueuedStake)
	assert.Equal(t, baseStake+stakeIncrease, totals.NextPeriodWeight)

	// adding queued delegation, queued stake should multiply
	delegationID1 := big.NewInt(1)
	delStake := MinStakeVET
	newTestSequence(t, staker).AddDelegation(validator, delStake, 200, delegationID1, 10)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeIncrease+delStake, qStake)
	expectedWeight := stakeIncrease*2 + delStake*2 + baseStake

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, stakeIncrease+delStake, totals.TotalQueuedStake)
	assert.Equal(t, baseStake+expectedWeight, totals.NextPeriodWeight)

	// decreasing stake shouldn't affect multipliers
	stakeDecrease := uint64(500)
	stakeIncDecDiff := stakeIncrease - stakeDecrease
	newTestSequence(t, staker).DecreaseStake(validator, val.Endorser, stakeDecrease)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stakeIncrease+delStake, qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, stakeDecrease, totals.TotalExitingStake)
	assert.Equal(t, stakeIncrease+delStake, totals.TotalQueuedStake)
	assert.Equal(t, baseStake+expectedWeight-stakeDecrease*2, totals.NextPeriodWeight)

	stakingPeriod := thor.MediumStakingPeriod()
	newTestSequence(t, staker).Housekeep(stakingPeriod * 2)
	assert.NoError(t, err)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	expectedStake := baseStake + stakeIncDecDiff
	expectedStake += delStake
	assert.Equal(t, expectedStake, lStake)
	expectedWeight = baseStake*2 + stakeIncDecDiff*2
	// adding delegation stake with multiplier
	expectedWeight += delStake * 2
	assert.Equal(t, expectedWeight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, totals.TotalLockedStake)
	assert.Equal(t, expectedWeight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, expectedWeight, totals.NextPeriodWeight)
}

func TestStaker_TestWeights_DecreaseStake(t *testing.T) {
	staker, _ := newStaker(t, 1, 1, true)

	validator, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(validator)
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, val.Weight)

	// one active validator without delegations
	lStake, lWeight, err := staker.LockedStake()
	baseStake := val.LockedVET
	assert.NoError(t, err)
	assert.Equal(t, baseStake, lStake)
	assert.Equal(t, baseStake, lWeight)

	qStake, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err := staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, baseStake, totals.NextPeriodWeight)

	// one active validator without delegations, increase stake, multiplier should be 0, decrease stake should be queued
	stakeDecrease := uint64(1500)
	newTestSequence(t, staker).DecreaseStake(validator, val.Endorser, stakeDecrease)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, stakeDecrease, totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, baseStake-stakeDecrease, totals.NextPeriodWeight)

	// adding queued delegation, queued stake should multiply
	delegationID1 := big.NewInt(1)
	delStake := MinStakeVET
	newTestSequence(t, staker).AddDelegation(validator, delStake, 200, delegationID1, 10)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, delStake, qStake)
	expectedWeight := delStake*2 + lStake

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, stakeDecrease, totals.TotalExitingStake)
	assert.Equal(t, delStake, totals.TotalQueuedStake)
	assert.Equal(t, baseStake+expectedWeight-stakeDecrease*2, totals.NextPeriodWeight)

	// decreasing stake shouldn't affect multipliers
	additionalDecrease := uint64(500)
	stakeDecrease += additionalDecrease
	newTestSequence(t, staker).DecreaseStake(validator, val.Endorser, additionalDecrease)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, val.LockedVET, lStake)
	assert.Equal(t, val.Weight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, delStake, qStake)
	expectedWeight = delStake*2 + lStake

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, baseStake, totals.TotalLockedStake)
	assert.Equal(t, baseStake, totals.TotalLockedWeight)
	assert.Equal(t, stakeDecrease, totals.TotalExitingStake)
	assert.Equal(t, delStake, totals.TotalQueuedStake)
	assert.Equal(t, baseStake+expectedWeight-stakeDecrease*2, totals.NextPeriodWeight)

	stakingPeriod := thor.MediumStakingPeriod()
	newTestSequence(t, staker).AssertGlobalWithdrawable(0).
		Housekeep(stakingPeriod * 2).
		AssertGlobalWithdrawable(stakeDecrease)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	expectedStake := baseStake - stakeDecrease
	expectedStake += delStake
	assert.Equal(t, expectedStake, lStake)
	expectedWeight = baseStake*2 - stakeDecrease*2
	expectedWeight += delStake * 2
	assert.Equal(t, expectedWeight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, totals.TotalLockedStake)
	assert.Equal(t, expectedWeight, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, expectedWeight, totals.NextPeriodWeight)

	newTestSequence(t, staker).SignalDelegationExit(delegationID1, 129600)

	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, lStake)
	assert.Equal(t, expectedWeight, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, totals.TotalLockedStake)
	assert.Equal(t, expectedWeight, totals.TotalLockedWeight)
	assert.Equal(t, delStake, totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, expectedWeight-delStake*2-baseStake+stakeDecrease, totals.NextPeriodWeight)

	newTestSequence(t, staker).AssertGlobalWithdrawable(stakeDecrease).
		Housekeep(stakingPeriod * 3).
		AssertGlobalWithdrawable(stakeDecrease + delStake)
	lStake, lWeight, err = staker.LockedStake()
	assert.NoError(t, err)

	expectedStake -= delStake
	assert.Equal(t, expectedStake, lStake)
	assert.Equal(t, expectedStake, lWeight)

	qStake, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	totals, err = staker.GetValidationTotals(validator)
	assert.NoError(t, err)
	assert.Equal(t, expectedStake, totals.TotalLockedStake)
	assert.Equal(t, expectedStake, totals.TotalLockedWeight)
	assert.Equal(t, uint64(0), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.TotalQueuedStake)
	assert.Equal(t, expectedStake, totals.NextPeriodWeight)
}

func TestStaker_OfflineValidator(t *testing.T) {
	staker, _ := newStaker(t, 5, 5, true)

	testSetup := newTestSequence(t, staker)

	validator1, err := testSetup.staker.FirstActive()
	assert.NoError(t, err)

	validator2, err := testSetup.staker.Next(validator1)
	assert.NoError(t, err)

	val1, err := testSetup.staker.GetValidation(validator1)
	assert.NoError(t, err)
	assert.Nil(t, val1.OfflineBlock)
	assert.Nil(t, val1.ExitBlock)

	val2, err := testSetup.staker.GetValidation(validator2)
	assert.NoError(t, err)
	assert.Nil(t, val2.OfflineBlock)
	assert.Nil(t, val2.ExitBlock)

	// setting validator offline will record offline block
	testSetup.SetOnline(validator1, 4, false)

	expectedOfflineBlock := uint32(4)
	val1, err = testSetup.staker.GetValidation(validator1)
	assert.NoError(t, err)
	assert.Equal(t, &expectedOfflineBlock, val1.OfflineBlock)
	assert.Nil(t, val1.ExitBlock)

	// setting validator online will clear offline block
	testSetup.SetOnline(validator1, 8, true)

	val1, err = testSetup.staker.GetValidation(validator1)
	assert.NoError(t, err)
	assert.Nil(t, val1.OfflineBlock)
	assert.Nil(t, val1.ExitBlock)

	// setting validator offline will not trigger eviction until threshold is met
	testSetup.SetOnline(validator1, 8, false)
	// Epoch length is 180, 336 is the number of epochs in 7 days which is threshold, 8 is the block number when val wen't offline
	testSetup.Housekeep(thor.EpochLength() * 336)

	expectedOfflineBlock = uint32(8)
	val1, err = testSetup.staker.GetValidation(validator1)
	assert.NoError(t, err)
	assert.Equal(t, &expectedOfflineBlock, val1.OfflineBlock)
	assert.Nil(t, val1.ExitBlock)

	// exit status is set to first free epoch after current one
	testSetup.Housekeep(thor.EpochLength() * 48 * 3 * 3)
	expectedExitBlock := (thor.EpochLength() * 48 * 3 * 3) + 180

	expectedOfflineBlock = uint32(8)
	val1, err = testSetup.staker.GetValidation(validator1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val1.Status)
	assert.Equal(t, &expectedOfflineBlock, val1.OfflineBlock)
	assert.Equal(t, expectedExitBlock, *val1.ExitBlock)

	// validator should exit here
	testSetup.AssertGlobalWithdrawable(0).
		Housekeep(expectedExitBlock).
		AssertGlobalCooldown(val1.LockedVET).
		AssertGlobalWithdrawable(0)

	val1, err = testSetup.staker.GetValidation(validator1)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val1.Status)
	assert.Equal(t, &expectedOfflineBlock, val1.OfflineBlock)
	assert.Equal(t, expectedExitBlock, *val1.ExitBlock)
}

func TestStaker_Housekeep_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	paramsAddr := thor.BytesToAddress([]byte("params"))

	param := params.New(paramsAddr, st)

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(2)))
	staker := New(stakerAddr, st, param, nil)

	housekeep, err := staker.Housekeep(thor.EpochLength() - 1)
	assert.NoError(t, err)
	assert.False(t, housekeep)

	activeHeadSlot := thor.BytesToBytes32([]byte(("validations-active-head")))
	st.SetRawStorage(stakerAddr, activeHeadSlot, rlp.RawValue{0xFF})

	_, err = staker.Housekeep(thor.EpochLength() * 48 * 3)
	assert.Error(t, err)

	keys := createKeys(2)

	st.SetRawStorage(stakerAddr, activeHeadSlot, rlp.RawValue{0x0})
	slotLockedVET := thor.BytesToBytes32([]byte(("total-weighted-stake")))
	valAddr := thor.Address{}
	for _, key := range keys {
		stake := RandomStake()
		valAddr = key.node
		if err := staker.AddValidation(key.node, key.endorser, uint32(360)*24*15, stake); err != nil {
			t.Fatal(err)
		}
	}
	lockedVet, err := st.GetRawStorage(stakerAddr, slotLockedVET)
	assert.NoError(t, err)
	st.SetRawStorage(stakerAddr, slotLockedVET, rlp.RawValue{0xFF})
	_, err = staker.Housekeep(thor.EpochLength())
	assert.Error(t, err)

	_, err = staker.Housekeep(thor.EpochLength() * 2)
	assert.Error(t, err)

	slotQueuedGroupSize := thor.BytesToBytes32([]byte(("validations-queued-group-size")))
	st.SetRawStorage(stakerAddr, slotLockedVET, lockedVet)
	st.SetRawStorage(stakerAddr, slotQueuedGroupSize, rlp.RawValue{0xFF})
	_, err = staker.Housekeep(thor.EpochLength() * 4)
	assert.Error(t, err)

	st.SetRawStorage(stakerAddr, slotLockedVET, rlp.RawValue{0xc2, 0x80, 0x80})
	st.SetRawStorage(stakerAddr, slotQueuedGroupSize, rlp.RawValue{0x0})

	slotActiveGroupSize := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	st.SetRawStorage(stakerAddr, slotActiveGroupSize, rlp.RawValue{0xFF})
	count, err := staker.computeActivationCount(true)
	assert.Error(t, err)
	assert.Equal(t, uint64(0), count)

	st.SetRawStorage(stakerAddr, slotActiveGroupSize, rlp.RawValue{0x0})
	st.SetRawStorage(paramsAddr, thor.KeyMaxBlockProposers, rlp.RawValue{0xFF})
	count, err = staker.computeActivationCount(true)
	assert.Error(t, err)
	assert.Equal(t, uint64(0), count)

	slotAggregations := thor.BytesToBytes32([]byte("aggregated-delegations"))
	validatorAddr := thor.BytesToAddress([]byte("renewal1"))
	slot := thor.Blake2b(validatorAddr.Bytes(), slotAggregations.Bytes())
	st.SetRawStorage(stakerAddr, slot, []byte{0xFF, 0xFF, 0xFF, 0xFF})
	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(0)))
	err = staker.applyEpochTransition(&EpochTransition{
		Block:           0,
		Renewals:        []thor.Address{validatorAddr},
		ExitValidator:   thor.Address{},
		Evictions:       nil,
		ActivationCount: 0,
	})
	assert.ErrorContains(t, err, "failed to get validator aggregation")
	re2 := thor.BytesToAddress([]byte("renewal2"))
	err = staker.applyEpochTransition(&EpochTransition{
		Block:           0,
		Renewals:        []thor.Address{re2},
		ExitValidator:   thor.Address{},
		Evictions:       nil,
		ActivationCount: 0,
	})
	assert.ErrorContains(t, err, "failed to get existing validator")

	slotValidations := thor.BytesToBytes32([]byte(("validations")))
	slot = thor.Blake2b(valAddr.Bytes(), slotValidations.Bytes())
	raw, err := st.GetRawStorage(stakerAddr, slot)
	assert.NoError(t, err)
	st.SetRawStorage(stakerAddr, slot, rlp.RawValue{0xFF})

	err = staker.applyEpochTransition(&EpochTransition{
		Block:           0,
		Renewals:        []thor.Address{},
		ExitValidator:   valAddr,
		Evictions:       nil,
		ActivationCount: 0,
	})

	assert.ErrorContains(t, err, "failed to get validator")

	st.SetRawStorage(stakerAddr, slot, raw)
	slot = thor.Blake2b(valAddr.Bytes(), slotAggregations.Bytes())
	st.SetRawStorage(stakerAddr, slot, []byte{0xFF, 0xFF, 0xFF, 0xFF})
	st.SetRawStorage(stakerAddr, slotActiveGroupSize, rlp.RawValue{0x2})
	err = staker.applyEpochTransition(&EpochTransition{
		Block:           0,
		Renewals:        []thor.Address{},
		ExitValidator:   valAddr,
		Evictions:       nil,
		ActivationCount: 0,
	})

	assert.ErrorContains(t, err, "failed to get validator")
}

func TestValidation_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	param := params.New(thor.BytesToAddress([]byte("params")), st)

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(2)))
	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := New(stakerAddr, st, param, nil)

	node1 := datagen.RandAddress()
	stake := RandomStake()
	err := staker.AddValidation(node1, node1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	validationsSlot := thor.BytesToBytes32([]byte(("validations")))
	slot := thor.Blake2b(node1.Bytes(), validationsSlot.Bytes())
	st.SetRawStorage(stakerAddr, slot, rlp.RawValue{0xFF})
	_, err = staker.GetWithdrawable(node1, thor.EpochLength())
	assert.Error(t, err)

	_, err = staker.GetValidationTotals(node1)
	assert.Error(t, err)

	_, err = staker.WithdrawStake(node1, node1, thor.EpochLength())
	assert.Error(t, err)

	err = staker.SignalExit(node1, node1, 10)
	assert.Error(t, err)

	err = staker.SignalDelegationExit(big.NewInt(0), 10)
	assert.Error(t, err)

	_, err = staker.GetValidation(node1)
	assert.Error(t, err)
}

func TestValidation_DecreaseOverflow(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	addr := datagen.RandAddress()
	endorser := datagen.RandAddress()

	newTestSequence(t, staker).AddValidation(addr, endorser, thor.MediumStakingPeriod(), MinStakeVET)

	overflowDecrease := math.MaxUint64 - MinStakeVET - 1
	assert.ErrorContains(t, staker.DecreaseStake(addr, endorser, overflowDecrease), "decrease amount is too large")

	assertValidation(t, staker, addr).QueuedVET(MinStakeVET)
}

func TestValidation_IncreaseOverflow(t *testing.T) {
	staker, _ := newStaker(t, 0, 1, false)
	addr := datagen.RandAddress()
	endorser := datagen.RandAddress()

	newTestSequence(t, staker).AddValidation(addr, endorser, thor.MediumStakingPeriod(), MinStakeVET)

	overflowIncrease := math.MaxUint64 - MinStakeVET + 1
	assert.ErrorContains(t, staker.IncreaseStake(addr, endorser, overflowIncrease), "increase amount is too large")

	assertValidation(t, staker, addr).QueuedVET(MinStakeVET)
}

func TestValidation_WithdrawBeforeAfterCooldown(t *testing.T) {
	staker, _ := newStaker(t, 2, 2, true)

	first, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(first)
	assert.NoError(t, err)

	stake := val.LockedVET

	test := newTestSequence(t, staker)

	test.AssertGlobalWithdrawable(0).
		SignalExit(first, val.Endorser, 1).
		Housekeep(thor.MediumStakingPeriod())

	assertValidation(t, staker, first).
		Status(validation.StatusExit).
		WithdrawableVET(0).
		CooldownVET(stake)

	test.AssertGlobalCooldown(stake).
		AssertGlobalWithdrawable(0).
		WithdrawStake(first, val.Endorser, thor.MediumStakingPeriod()+thor.CooldownPeriod(), stake).
		AssertGlobalWithdrawable(0).
		AssertGlobalCooldown(0)
}
