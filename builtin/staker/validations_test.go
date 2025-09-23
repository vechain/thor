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
	addr  thor.Address
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
	newBalance := big.NewInt(0).Add(balance, ToWei(stake))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return err
	}
	err = ts.Staker.AddValidation(validator, endorser, period, stake)
	if err != nil {
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
	newBalance := big.NewInt(0).Add(balance, ToWei(amount))
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
	newBalance := big.NewInt(0).Sub(balance, ToWei(amount))
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
	newBalance := big.NewInt(0).Add(balance, ToWei(stake))
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
	newBalance := big.NewInt(0).Sub(balance, ToWei(amount))
	if ts.state.SetBalance(ts.addr, newBalance) != nil {
		return 0, err
	}
	return amount, nil
}

// newStakerV2 is a temporary function to help migration to use TestSequence.
func newStakerV2(t *testing.T, amount int, maxValidators int64, initialise bool) (*TestSequence, uint64) {
	staker, totalStake := newStaker(t, amount, maxValidators, initialise)

	return newTestSequence(t, staker), totalStake
}

func newStaker(t *testing.T, amount int, maxValidators int64, initialise bool) (*testStaker, uint64) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	keys := createKeys(amount)
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	stakerAddr := thor.BytesToAddress([]byte("stkr"))

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(maxValidators)))
	stakerImpl := New(stakerAddr, st, param, nil)
	staker := &testStaker{
		addr:   stakerAddr,
		state:  st,
		Staker: stakerImpl,
	}

	totalStake := uint64(0)
	if initialise {
		for _, key := range keys {
			stake := RandomStake()
			totalStake += stake
			if err := staker.AddValidation(key.node, key.endorser, thor.MediumStakingPeriod(), stake); err != nil {
				t.Fatal(err)
			}
		}
		transitioned, err := staker.transition(0)
		assert.NoError(t, err)
		assert.True(t, transitioned)
	}

	return &testStaker{
		addr:   stakerAddr,
		state:  st,
		Staker: stakerImpl,
	}, totalStake
}

func TestStaker_TotalStake(t *testing.T) {
	staker, totalStaked := newStakerV2(t, 0, 14, false)

	stakers := datagen.RandAddresses(10)
	stakes := make(map[thor.Address]uint64)

	for _, addr := range stakers {
		stakeAmount := RandomStake()
		staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stakeAmount) // false
		stakes[addr] = stakeAmount
		totalStaked += stakeAmount
		staker.ActivateNext(0)
		staker.AssertLockedVET(totalStaked, totalStaked)
	}

	for id, stake := range stakes {
		staker.ExitValidator(id)
		totalStaked -= stake
		staker.AssertLockedVET(totalStaked, totalStaked)
	}
}

func TestStaker_TotalStake_Withdrawal(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 14, false)

	addr := datagen.RandAddress()
	stakeAmount := RandomStake()
	period := thor.MediumStakingPeriod()

	// add + activate
	staker.
		AddValidation(addr, addr, period, stakeAmount).
		AssertQueuedVET(stakeAmount).
		ActivateNext(0)

	// disable auto renew
	staker.SignalExit(addr, addr, 10).
		AssertLockedVET(stakeAmount, stakeAmount).
		AssertQueuedVET(0)

	// exit
	staker.
		ExitValidator(addr).
		AssertLockedVET(0, 0)

	assertValidation(t, staker, addr).
		Status(validation.StatusExit).
		CooldownVET(stakeAmount)

	staker.
		AssertWithdrawable(addr, period+thor.CooldownPeriod(), stakeAmount).
		WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stakeAmount)

	assertValidation(t, staker, addr).
		Status(validation.StatusExit).
		WithdrawableVET(0)

	staker.
		AssertLockedVET(0, 0).
		AssertQueuedVET(0)
}

func TestStaker_AddValidation_MinimumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooLow := MinStakeVET - 1
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), tooLow)
	assert.ErrorContains(t, err, "stake is below minimum")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MaximumStake(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	tooHigh := MaxStakeVET + 1
	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), tooHigh)
	assert.ErrorContains(t, err, "stake is above maximum")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MaxStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MaximumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, MinStakeVET)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_MinimumStakingPeriod(t *testing.T) {
	staker, _ := newStaker(t, 101, 101, true)

	err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, MinStakeVET)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), 100, MinStakeVET)
	assert.ErrorContains(t, err, "period is out of boundaries")
	err = staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	assert.NoError(t, err)
}

func TestStaker_AddValidation_Duplicate(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := uint64(25e6)
	err := staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	assert.NoError(t, err)
	err = staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	assert.ErrorContains(t, err, "validator already exists")
}

func TestStaker_AddValidation_QueueOrder(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	expectedOrder := [100]thor.Address{}
	// add 100 validations to the queue
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
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
	err := staker.AddValidation(addr1, addr1, thor.MediumStakingPeriod(), stake)
	assert.NoError(t, err)

	validator, err := staker.GetValidation(addr1)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr2, addr2, thor.MediumStakingPeriod(), stake)
	assert.NoError(t, err)

	validator, err = staker.GetValidation(addr2)
	assert.NoError(t, err)
	assert.False(t, validator == nil)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	err = staker.AddValidation(addr3, addr3, thor.HighStakingPeriod(), stake)
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
	staker, _ := newStakerV2(t, 101, 101, false)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()
	staker.AddValidation(addr1, addr1, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr1).Status(validation.StatusQueued).QueuedVET(stake)

	staker.Housekeep(180)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake)

	staker.AddValidation(addr2, addr2, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr2).Status(validation.StatusQueued).QueuedVET(stake)

	staker.AddValidation(addr3, addr3, thor.HighStakingPeriod(), stake)
	staker.AssertValidation(addr3).Status(validation.StatusQueued).QueuedVET(stake)

	staker.AddValidation(addr4, addr4, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr4).Status(validation.StatusQueued).QueuedVET(stake)

	staker.Housekeep(180 * 2)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake)
	staker.AssertValidation(addr2).Status(validation.StatusActive).LockedVET(stake)
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
	err := staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
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
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()
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
	staker, _ := newStakerV2(t, 68, 101, true)
	addr := datagen.RandAddress()
	stake := RandomStake()

	staker.IncreaseStakeErrors(addr, thor.Address{}, stake, "validation does not exist")
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr).
		Status(validation.StatusQueued).
		QueuedVET(stake).
		Weight(0)

	// increase stake queued
	increase := uint64(1000)
	staker.IncreaseStake(addr, addr, increase)

	staker.AssertValidation(addr).
		Status(validation.StatusQueued).
		QueuedVET(increase + stake).
		Weight(0)
}

func TestStaker_IncreaseQueued_Order(t *testing.T) {
	staker, _ := newStakerV2(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	// add addr
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// add addr1
	staker.AddValidation(addr1, addr1, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr1).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// add addr2
	staker.AddValidation(addr2, addr2, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr2).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// verify order
	staker.AssertFirstQueued(addr).
		AssertNext(addr, addr1).
		AssertNext(addr1, addr2).
		AssertNext(addr2, thor.Address{})
}

func TestStaker_DecreaseQueued_Order(t *testing.T) {
	staker, _ := newStakerV2(t, 68, 101, true)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()

	staker.DecreaseStakeErrors(addr, thor.Address{}, stake, "validation does not exist")

	// add the validator
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	staker.AddValidation(addr1, addr1, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr1).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	staker.AddValidation(addr2, addr2, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr2).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// verify order
	staker.AssertFirstQueued(addr).AssertNext(addr, addr1).AssertNext(addr1, addr2).AssertNext(addr2, thor.Address{})

	// increase stake queued
	decreaseBy := uint64(1000)
	staker.DecreaseStake(addr1, addr1, decreaseBy)
	staker.AssertFirstQueued(addr).AssertNext(addr, addr1).AssertNext(addr1, addr2).AssertNext(addr2, thor.Address{})
	staker.AssertValidation(addr1).QueuedVET(stake - decreaseBy).Weight(0)
}

func TestStaker_IncreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

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
	staker, _ := newStakerV2(t, 0, 1, false)
	addr := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.AddValidation(addr, addr, period, stake)
	// add a second validator
	staker.AddValidation(addr2, addr2, period, stake)

	staker.ActivateNext(0)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake)
	staker.AssertValidation(addr2).Status(validation.StatusQueued).QueuedVET(stake)

	increase := uint64(1000)
	staker.IncreaseStake(addr, addr, increase)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake).QueuedVET(increase).Weight(stake)
	staker.AssertQueuedVET(increase + stake) // addr2 + addr 1 increase

	// verify withdraw amount decrease
	staker.Housekeep(period)
	staker.AssertValidation(addr).Weight(stake + increase).LockedVET(stake + increase).QueuedVET(0)
	staker.AssertQueuedVET(stake)

	// decrease stake
	decreaseAmount := uint64(2500)
	staker.DecreaseStake(addr, addr, decreaseAmount)
	staker.AssertValidation(addr).PendingUnlockVET(decreaseAmount).LockedVET(stake + increase)
	staker.AssertQueuedVET(stake)

	// verify queued weight is decreased
	staker.Housekeep(period)
	staker.AssertValidation(addr).
		Weight(stake + increase - decreaseAmount).
		LockedVET(stake + increase - decreaseAmount).
		PendingUnlockVET(0)
	staker.AssertQueuedVET(stake)
}

func TestStaker_DecreaseActive(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr := datagen.RandAddress()
	stake := MaxStakeVET
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()
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
	period := thor.MediumStakingPeriod()

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
	staker, _ := newStakerV2(t, 0, 101, false)

	// fill 101 validations to leader group
	for range 101 {
		staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), RandomStake())
		staker.ActivateNext(0)
	}

	// try to add one more to the leadergroup
	staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), RandomStake())
	staker.ActivateNextErrors(0, "leader group is full")
}

func TestStaker_ActivateNextValidator_EmptyQueue(t *testing.T) {
	staker, _ := newStakerV2(t, 100, 101, true)
	staker.ActivateNextErrors(0, "no validator in the queue")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker, _ := newStakerV2(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.ActivateNext(0)

	staker.AssertValidation(addr).Status(validation.StatusActive)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker, _ := newStakerV2(t, 101, 101, true)

	addr := datagen.RandAddress()
	staker.ExitValidatorErrors(addr, "failed to get existing validator")
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker, _ := newStakerV2(t, 68, 101, true)

	addr := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(addr, addr, period, stake)
	staker.ActivateNext(0)

	// disable auto renew
	staker.SignalExit(addr, addr, 10)
	staker.ExitValidator(addr)
	staker.AssertValidation(addr).Status(validation.StatusExit).LockedVET(0).CooldownVET(stake).Weight(0)

	// withdraw before cooldown, zero amount
	staker.WithdrawStake(addr, addr, period, 0)
	// withdraw after cooldown
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stake)
}

func TestStaker_LeaderGroup(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	added := make(map[thor.Address]bool)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
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
		err := staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
		assert.NoError(t, err)
		_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
		assert.NoError(t, err)
		leaderGroup = append(leaderGroup, addr)
	}

	queuedGroup := [100]thor.Address{}
	for i := range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		err := staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
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
		err := staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
		assert.NoError(t, err)
	}

	transitioned, err := staker.transition(0)
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validations after initialisation
	err = staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), MinStakeVET)
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
	staker, _ := newStakerV2(t, 68, 101, true)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()

	stake := RandomStake()

	staker.
		AddValidation(addr1, addr1, thor.MediumStakingPeriod(), stake).
		ActivateNext(0).
		AddValidation(addr2, addr2, thor.MediumStakingPeriod(), stake).
		ActivateNext(0)

	staker.Housekeep(0)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake)
	staker.AssertValidation(addr2).Status(validation.StatusActive).LockedVET(stake)
}

func TestStaker_Housekeep_ExitOne(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// Add first validator
	staker.AddValidation(addr1, addr1, period, stake)
	staker.AssertLockedVET(0, 0).AssertQueuedVET(stake)
	staker.ActivateNext(0)
	staker.AssertLockedVET(stake, stake).AssertQueuedVET(0)

	// Add second validator
	staker.AddValidation(addr2, addr2, period, stake)
	staker.AssertLockedVET(stake, stake).AssertQueuedVET(stake)
	staker.ActivateNext(0)
	staker.AssertLockedVET(stake*2, stake*2).AssertQueuedVET(0)

	// Add third validator
	staker.AddValidation(addr3, addr3, period, stake)
	staker.AssertLockedVET(stake*2, stake*2).AssertQueuedVET(stake)
	staker.ActivateNext(0)
	staker.AssertLockedVET(stake*3, stake*3).AssertQueuedVET(0)

	// disable auto renew
	staker.SignalExit(addr1, addr1, 10)

	// first should be on cooldown
	staker.Housekeep(period)
	staker.AssertValidation(addr1).Status(validation.StatusExit).LockedVET(0).CooldownVET(stake).WithdrawableVET(0)
	staker.AssertValidation(addr2).Status(validation.StatusActive).LockedVET(stake)
	staker.AssertLockedVET(stake*2, stake*2).AssertQueuedVET(0)

	staker.Housekeep(period + thor.CooldownPeriod())
	staker.AssertValidation(addr1).Status(validation.StatusExit).LockedVET(0).CooldownVET(stake).WithdrawableVET(0)
	staker.AssertValidation(addr2).Status(validation.StatusActive).LockedVET(stake)
	staker.AssertLockedVET(stake*2, stake*2).AssertQueuedVET(0)
}

func TestStaker_Housekeep_Cooldown(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	period := thor.MediumStakingPeriod()

	stake := RandomStake()

	staker.AddValidation(addr1, addr1, period, stake).
		ActivateNext(0).
		AddValidation(addr2, addr2, period, stake).
		ActivateNext(0).
		AddValidation(addr3, addr3, period, stake).
		ActivateNext(0)

	// disable auto renew on all validators
	staker.SignalExit(addr1, addr1, 10)
	staker.SignalExit(addr2, addr2, 10)
	staker.SignalExit(addr3, addr3, 10)

	id, _ := staker.FirstActive()
	next, _ := staker.Next(id)
	assert.Equal(t, addr2, next)

	staker.AssertLockedVET(stake*3, stake*3)

	// housekeep and exit validator 1
	staker.Housekeep(period)
	staker.AssertValidation(addr1).Status(validation.StatusExit).LockedVET(0).CooldownVET(stake).WithdrawableVET(0)
	staker.AssertValidation(addr2).Status(validation.StatusActive).LockedVET(stake)

	// housekeep and exit validator 2
	staker.Housekeep(period + thor.EpochLength())
	staker.AssertValidation(addr2).Status(validation.StatusExit).LockedVET(0).CooldownVET(stake).WithdrawableVET(0)

	// housekeep and exit validator 3
	staker.Housekeep(period + thor.EpochLength()*2)
	staker.AssertLockedVET(0, 0)

	staker.WithdrawStake(addr1, addr1, period+thor.CooldownPeriod(), stake)
}

func TestStaker_Housekeep_CooldownToExited(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

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
	staker, _ := newStakerV2(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(addr1, addr1, period, stake).
		ActivateNext(0).
		AddValidation(addr2, addr2, period, stake).
		ActivateNext(0).
		AddValidation(addr3, addr3, period, stake).
		ActivateNext(period * 2)

	// disable auto renew
	staker.SignalExit(addr2, addr2, 10).SignalExit(addr3, addr3, 259200)

	staker.Housekeep(period)
	staker.AssertValidation(addr2).Status(validation.StatusExit)
	staker.AssertValidation(addr3).Status(validation.StatusActive)
	staker.AssertValidation(addr1).Status(validation.StatusActive)

	// renew validator 1 for next period
	staker.Housekeep(period*2).SignalExit(addr1, addr1, 259201)

	// housekeep -> validator 3 placed intention to leave first
	staker.Housekeep(period * 3)
	staker.AssertValidation(addr3).Status(validation.StatusExit)
	staker.AssertValidation(addr1).Status(validation.StatusActive)

	// housekeep -> validator 1 waited 1 epoch after validator 3
	staker.Housekeep(period*3 + thor.EpochLength())
	staker.AssertValidation(addr1).Status(validation.StatusExit)
}

func TestStaker_Housekeep_RecalculateIncrease(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MinStakeVET
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

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

	block = thor.MediumStakingPeriod()
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
	staker, _ := newStakerV2(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := MaxStakeVET
	period := thor.MediumStakingPeriod()

	// auto renew is turned on
	staker.AddValidation(addr1, addr1, period, stake).
		ActivateNext(0).
		DecreaseStake(addr1, addr1, 1)

	block := uint32(360) * 24 * 13
	staker.Housekeep(block)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake).Weight(stake).PendingUnlockVET(1)

	block = thor.MediumStakingPeriod()
	staker.Housekeep(block)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake - 1).Weight(stake - 1).WithdrawableVET(1)

	staker.WithdrawStake(addr1, addr1, block+thor.CooldownPeriod(), 1)
	staker.AssertValidation(addr1).WithdrawableVET(0)

	// verify that validator is still present and active
	staker.AssertValidation(addr1).LockedVET(stake - 1).Weight(stake - 1)
	staker.AssertFirstActive(addr1)
}

func TestStaker_DecreaseActive_DecreaseMultipleTimes(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

	err := staker.AddValidation(addr1, addr1, period, stake)
	assert.NoError(t, err)
	_, err = staker.activateNextValidation(0, getTestMaxLeaderSize(staker.params))
	assert.NoError(t, err)

	// disable auto renew
	err = staker.SignalExit(addr1, addr1, 10)
	assert.NoError(t, err)

	exitBlock := thor.MediumStakingPeriod()
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
	staker, _ := newStakerV2(t, 0, 3, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.
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
	staker.
		AssertGlobalCooldown(stake).
		AssertGlobalWithdrawable(0).
		Housekeep(block).
		AssertGlobalCooldown(stake * 2).
		AssertGlobalWithdrawable(0).
		AssertLeaderGroupSize(0).
		AssertFirstActive(thor.Address{})

	assertValidation(t, staker, addr2).Status(validation.StatusExit)

	staker.
		AddValidation(addr3, addr3, period, stake).
		ActivateNext(block).
		SignalExit(addr3, addr3, block+period-1).
		AssertFirstActive(addr3).
		AssertLeaderGroupSize(1)

	assertValidation(t, staker, addr3).Status(validation.StatusActive)

	block = block + period
	staker.Housekeep(block).AssertGlobalWithdrawable(0).AssertGlobalCooldown(stake * 3)

	assertValidation(t, staker, addr3).Status(validation.StatusExit)
}

func TestStaker_Housekeep_Adds_Queued_Validators_Up_To_Limit(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 2, false)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(addr1, addr1, period, stake).
		AddValidation(addr2, addr2, period, stake).
		AddValidation(addr3, addr3, period, stake).
		AssertQueueSize(3).
		AssertLeaderGroupSize(0)

	block := uint32(360) * 24 * 13
	staker.Housekeep(block)
	staker.AssertValidation(addr1).Status(validation.StatusActive)
	staker.AssertValidation(addr2).Status(validation.StatusActive)
	staker.AssertValidation(addr3).Status(validation.StatusQueued)
	staker.AssertLeaderGroupSize(2)
	staker.AssertQueueSize(1)
}

func TestStaker_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

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
	period := thor.MediumStakingPeriod()

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
	staker, _ := newStakerV2(t, 0, 1, false)

	acc := datagen.RandAddress()
	initialStake := RandomStake()
	increases := uint64(0)
	decreases := uint64(0)
	withdrawnTotal := uint64(0)
	thousand := uint64(1000)
	fiveHundred := uint64(500)

	period := thor.MediumStakingPeriod()

	// QUEUED
	staker.AddValidation(acc, acc, period, initialStake)

	increases += thousand
	staker.IncreaseStake(acc, acc, thousand)
	// 1st decrease
	decreases += fiveHundred
	staker.DecreaseStake(acc, acc, fiveHundred)

	validator := staker.GetValidator(acc)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	// 1st STAKING PERIOD
	staker.Housekeep(period)

	validator = staker.GetValidator(acc)
	assert.Equal(t, validation.StatusActive, validator.Status)
	expected := initialStake - decreases
	expected += increases
	assert.Equal(t, expected, validator.LockedVET)

	// See `1st decrease` -> validator should be able withdraw the decrease amount
	staker.WithdrawStake(acc, acc, period+1, fiveHundred)
	withdrawnTotal += fiveHundred

	expectedLocked := initialStake - decreases
	expectedLocked += increases
	validator = staker.GetValidator(acc)
	assert.Equal(t, expectedLocked, validator.LockedVET)

	// 2nd decrease
	decreases += thousand
	staker.DecreaseStake(acc, acc, thousand)
	increases += fiveHundred
	staker.IncreaseStake(acc, acc, fiveHundred)

	// 2nd STAKING PERIOD
	staker.Housekeep(period * 2)
	validator = staker.GetValidator(acc)
	assert.Equal(t, validation.StatusActive, validator.Status)

	// See `2nd decrease` -> validator should be able withdraw the decrease amount
	staker.WithdrawStake(acc, acc, period*2+thor.CooldownPeriod(), thousand)
	withdrawnTotal += thousand

	staker.SignalExit(acc, acc, period*2)

	// EXITED
	staker.Housekeep(period * 3)

	validator = staker.GetValidator(acc)
	assert.Equal(t, validation.StatusExit, validator.Status)
	expectedLocked = initialStake - decreases
	expectedLocked += increases
	validator = staker.GetValidator(acc)
	assert.Equal(t, expectedLocked, validator.CooldownVET)

	staker.WithdrawStake(acc, acc, period*3+thor.CooldownPeriod(), expectedLocked)
	withdrawnTotal += expectedLocked
	depositTotal := initialStake + increases
	assert.Equal(t, depositTotal, withdrawnTotal)
}

func Test_GetValidatorTotals_ValidatorExiting(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	dStake := stakes.NewWeightedStakeWithMultiplier(MinStakeVET, 255)
	newTestSequence(t, staker).AddDelegation(validator.ID, dStake.VET, 255, 10)

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

	delegationID := newTestSequence(t, staker).AddDelegation(validator.ID, dStake.VET, 255, 10)

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
	staker, _ := newStakerV2(t, 0, 1, false)
	acc := datagen.RandAddress()

	// Add & activate validator
	staker.AddValidation(acc, acc, thor.LowStakingPeriod(), MinStakeVET)

	// Increase and decrease - both should be okay since we're only dealing with QueuedVET
	staker.IncreaseStake(acc, acc, MinStakeVET) // 25m + 25m = 50m
	staker.DecreaseStake(acc, acc, MinStakeVET) // 25m - 50m = 25m

	// Activate the validator.
	staker.ActivateNext(0)

	// Withdraw the previous decrease amount
	staker.WithdrawStake(acc, acc, 0, MinStakeVET)

	// Assert previous increase/decrease had no effect since they requested the same amount
	staker.AssertValidation(acc).Status(validation.StatusActive).LockedVET(MinStakeVET).Weight(MinStakeVET)

	// Increase stake (ok): 25m + 25m = 50m
	staker.IncreaseStake(acc, acc, MinStakeVET)
	// Decrease stake (NOT ok): 25m - 25m = 0. The Previous increase is not applied since it is still currently withdrawable.
	staker.DecreaseStakeErrors(acc, acc, MinStakeVET, "next period stake is lower than minimum stake")
	// Instantly withdraw - This is bad, it pulls from the QueuedVET, which means total stake later will be 0.
	// The decrease previously marked as okay since the current TVL + pending TVL was greater than the min stake.
	staker.WithdrawStake(acc, acc, 0, MinStakeVET)

	// Housekeep, should move pending locked to locked, and pending withdraw to withdrawable
	staker.Housekeep(thor.LowStakingPeriod())

	// Withdraw again
	staker.WithdrawStake(acc, acc, thor.LowStakingPeriod()+thor.CooldownPeriod(), 0)
	staker.AssertValidation(acc).LockedVET(MinStakeVET)
}

func TestStaker_AddValidation_CannotAddValidationWithSameMaster(t *testing.T) {
	staker, _ := newStaker(t, 68, 101, true)

	address := datagen.RandAddress()
	err := staker.AddValidation(address, datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	assert.NoError(t, err)

	err = staker.AddValidation(address, datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	assert.Error(t, err, "validator already exists")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMasterAfterExit(t *testing.T) {
	staker, _ := newStakerV2(t, 68, 101, true)

	master := datagen.RandAddress()
	endorser := datagen.RandAddress()
	staker.
		AddValidation(master, endorser, thor.MediumStakingPeriod(), MinStakeVET).
		ActivateNext(0).
		SignalExit(master, endorser, 10).
		ExitValidator(master).
		AddValidationErrors(master, datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET, "validator already exists")
}

func TestStaker_HasDelegations(t *testing.T) {
	staker, _ := newStakerV2(t, 1, 1, true)

	validator, _ := staker.FirstActive()
	dStake := delegationStake()
	stakingPeriod := thor.MediumStakingPeriod()

	staker.
		// no delegations, should be false
		AssertHasDelegations(validator, false)
	// delegation added, housekeeping not performed, should be false
	delegationID := staker.AddDelegation(validator, dStake, 200, 10)

	staker.
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
	staker, _ := newStakerV2(t, 0, 1, false)

	master := datagen.RandAddress()
	endorser := datagen.RandAddress()
	beneficiary := datagen.RandAddress()

	// add validation without a beneficiary
	staker.AddValidation(master, endorser, thor.MediumStakingPeriod(), MinStakeVET).ActivateNext(0)
	assertValidation(t, staker, master).Beneficiary(thor.Address{})

	// negative cases
	staker.SetBeneficiaryErrors(master, master, beneficiary, "endorser required")
	staker.SetBeneficiaryErrors(endorser, endorser, beneficiary, "validation does not exist")

	// set beneficiary, should be successful
	staker.SetBeneficiary(master, endorser, beneficiary)
	assertValidation(t, staker, master).Beneficiary(beneficiary)

	// remove the beneficiary
	staker.SetBeneficiary(master, endorser, thor.Address{})
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
	staker, _ := newStakerV2(t, 1, 1, true)

	validator, val := staker.FirstActive()

	v1Totals := &validation.Totals{
		TotalLockedStake:  val.LockedVET,
		TotalLockedWeight: val.Weight,
		NextPeriodWeight:  val.Weight,
	}

	staker.
		AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(0).
		AssertTotals(validator, v1Totals)

	// one active validator without delegations, one queued delegator without delegations
	stake := MinStakeVET
	keys := createKeys(1)
	validator2 := thor.Address{}
	for _, key := range keys {
		validator2 = key.node
		staker.AddValidation(key.node, key.endorser, thor.MediumStakingPeriod(), stake)
	}

	v2Totals := &validation.Totals{
		TotalQueuedStake: stake,
		NextPeriodWeight: stake,
	}

	staker.
		AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(stake).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// active validator with queued delegation, queued validator
	dStake := stakes.NewWeightedStakeWithMultiplier(1, 255)
	delegationID := staker.AddDelegation(validator, dStake.VET, 255, 10)
	v1Totals.TotalQueuedStake = dStake.VET
	v1Totals.NextPeriodWeight += dStake.Weight + val.LockedVET

	staker.AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(stake+dStake.VET).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// second delegator shouldn't multiply
	delegationID2 := staker.AddDelegation(validator, dStake.VET, 255, 10)
	v1Totals.TotalQueuedStake += dStake.VET
	v1Totals.NextPeriodWeight += dStake.Weight

	staker.AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(stake+dStake.VET*2).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// delegator on queued should multiply
	delegationID3 := staker.AddDelegation(validator2, dStake.VET, 255, 10)
	v2Totals.TotalQueuedStake += dStake.VET
	v2Totals.NextPeriodWeight += dStake.Weight + stake

	staker.AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(stake+dStake.VET*3).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// second delegator on queued should not multiply
	delegationID4 := staker.AddDelegation(validator2, dStake.VET, 255, 10)
	v2Totals.TotalQueuedStake += dStake.VET
	v2Totals.NextPeriodWeight += dStake.Weight

	staker.AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(stake+dStake.VET*4).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// Housekeep, first validator should have both delegations active
	stakingPeriod := thor.MediumStakingPeriod()
	staker.Housekeep(stakingPeriod)

	v1Totals = &validation.Totals{
		TotalLockedStake:  val.LockedVET + dStake.VET*2,
		TotalLockedWeight: val.LockedVET*2 + dStake.Weight*2,
		NextPeriodWeight:  val.LockedVET*2 + dStake.Weight*2,
	}

	staker.AssertLockedVET(v1Totals.TotalLockedStake, v1Totals.TotalLockedWeight).
		AssertQueuedVET(stake+dStake.VET*2).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// exit queued
	staker.
		AssertGlobalWithdrawable(0).
		WithdrawDelegation(delegationID3, dStake.VET, 10).
		AssertGlobalWithdrawable(0)
	v2Totals.TotalQueuedStake -= dStake.VET
	v2Totals.NextPeriodWeight -= dStake.Weight

	staker.AssertLockedVET(v1Totals.TotalLockedStake, v1Totals.TotalLockedWeight).
		AssertQueuedVET(stake+dStake.VET).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// exit second queued, multiplier should be one
	stakeIncrease := uint64(1000)
	staker.
		WithdrawDelegation(delegationID4, dStake.VET, 10).
		IncreaseStake(validator, val.Endorser, stakeIncrease).
		AssertGlobalWithdrawable(0).
		Housekeep(stakingPeriod * 3).
		AssertGlobalWithdrawable(0)

	v1Totals.TotalLockedStake += stakeIncrease
	v1Totals.TotalLockedWeight += stakeIncrease * 2
	v1Totals.NextPeriodWeight += stakeIncrease * 2

	v2Totals = &validation.Totals{
		TotalQueuedStake: stake,
		NextPeriodWeight: stake,
	}

	staker.AssertLockedVET(v1Totals.TotalLockedStake, v1Totals.TotalLockedWeight).
		AssertQueuedVET(stake).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// exit first active, multiplier should not change
	staker.
		SignalDelegationExit(delegationID, stakingPeriod*3).
		AssertGlobalWithdrawable(0).
		Housekeep(stakingPeriod * 4).
		AssertGlobalWithdrawable(dStake.VET)
	v1Totals.TotalLockedStake -= dStake.VET
	v1Totals.TotalLockedWeight -= dStake.Weight
	v1Totals.NextPeriodWeight -= dStake.Weight

	staker.AssertLockedVET(v1Totals.TotalLockedStake, v1Totals.TotalLockedWeight).
		AssertQueuedVET(stake).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	// exit second active, multiplier should change to 1
	staker.SignalDelegationExit(delegationID2, stakingPeriod*4)
	v1Totals.NextPeriodWeight = val.LockedVET + stakeIncrease
	v1Totals.TotalExitingStake = dStake.VET

	staker.AssertLockedVET(v1Totals.TotalLockedStake, v1Totals.TotalLockedWeight).
		AssertQueuedVET(stake).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)

	staker.AssertGlobalWithdrawable(dStake.VET).
		Housekeep(stakingPeriod * 5).
		AssertGlobalWithdrawable(dStake.VET * 2)

	v1Totals = &validation.Totals{
		TotalLockedStake:  val.LockedVET + stakeIncrease,
		TotalLockedWeight: val.LockedVET + stakeIncrease,
		NextPeriodWeight:  val.LockedVET + stakeIncrease,
	}

	staker.AssertLockedVET(v1Totals.TotalLockedStake, v1Totals.TotalLockedWeight).
		AssertQueuedVET(stake).
		AssertTotals(validator, v1Totals).
		AssertTotals(validator2, v2Totals)
}

func TestStaker_TestWeights_IncreaseStake(t *testing.T) {
	staker, _ := newStakerV2(t, 1, 1, true)

	validator, val := staker.FirstActive()
	baseStake := val.LockedVET

	totals := &validation.Totals{
		TotalLockedStake:  baseStake,
		TotalLockedWeight: baseStake,
		NextPeriodWeight:  baseStake,
	}

	// one active validator without delegations
	staker.
		AssertLockedVET(baseStake, baseStake).
		AssertQueuedVET(0).
		AssertTotals(validator, totals)

	// one active validator without delegations, increase stake, multiplier should be 0, increase stake should be queued
	stakeIncrease := uint64(1500)
	staker.IncreaseStake(validator, val.Endorser, stakeIncrease)
	totals.NextPeriodWeight += stakeIncrease
	totals.TotalQueuedStake += stakeIncrease

	staker.
		AssertLockedVET(baseStake, baseStake).
		AssertQueuedVET(stakeIncrease).
		AssertTotals(validator, totals)

	// adding queued delegation, queued stake should multiply
	delStake := MinStakeVET
	staker.AddDelegation(validator, delStake, 200, 10)
	totals.TotalQueuedStake += delStake
	totals.NextPeriodWeight = totals.NextPeriodWeight*2 + delStake*2

	staker.
		AssertLockedVET(baseStake, baseStake).
		AssertQueuedVET(stakeIncrease+delStake).
		AssertTotals(validator, totals)

	// decreasing stake shouldn't affect multipliers
	stakeDecrease := uint64(500)
	staker.DecreaseStake(validator, val.Endorser, stakeDecrease)
	totals.TotalExitingStake += stakeDecrease
	totals.NextPeriodWeight -= stakeDecrease * 2

	staker.
		AssertLockedVET(baseStake, baseStake).
		AssertQueuedVET(stakeIncrease+delStake).
		AssertTotals(validator, totals)

	stakingPeriod := thor.MediumStakingPeriod()
	staker.Housekeep(stakingPeriod * 2)

	totals.TotalLockedStake = baseStake + stakeIncrease - stakeDecrease + delStake
	totals.TotalLockedWeight = totals.NextPeriodWeight
	totals.TotalExitingStake = 0
	totals.TotalQueuedStake = 0

	staker.
		AssertLockedVET(totals.TotalLockedStake, totals.TotalLockedWeight).
		AssertQueuedVET(0).
		AssertTotals(validator, totals)
}

func TestStaker_TestWeights_DecreaseStake(t *testing.T) {
	staker, _ := newStakerV2(t, 1, 1, true)

	validator, val := staker.FirstActive()
	vStake := val.LockedVET

	staker.
		AssertLockedVET(vStake, vStake).
		AssertQueuedVET(0).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  vStake,
			TotalLockedWeight: vStake,
			TotalExitingStake: 0,
			TotalQueuedStake:  0,
			NextPeriodWeight:  vStake,
		})

	// one active validator without delegations, increase stake, multiplier should be 0, decrease stake should be queued
	stakeDecrease := uint64(1500)
	staker.DecreaseStake(validator, val.Endorser, stakeDecrease)

	staker.
		AssertLockedVET(vStake, vStake).
		AssertQueuedVET(0).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  vStake,
			TotalLockedWeight: vStake,
			TotalExitingStake: stakeDecrease,
			TotalQueuedStake:  0,
			NextPeriodWeight:  vStake - stakeDecrease,
		})

	// adding queued delegation, queued stake should multiply
	dStake := MinStakeVET
	delegationID1 := staker.AddDelegation(validator, MinStakeVET, 200, 10)

	staker.
		AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(dStake).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  vStake,
			TotalLockedWeight: vStake,
			TotalExitingStake: stakeDecrease,
			TotalQueuedStake:  dStake,
			NextPeriodWeight:  (vStake-stakeDecrease)*2 + dStake*2,
		})

	// decreasing stake shouldn't affect multipliers
	additionalDecrease := uint64(500)
	stakeDecrease += additionalDecrease
	staker.DecreaseStake(validator, val.Endorser, additionalDecrease)
	staker.
		AssertLockedVET(val.LockedVET, val.Weight).
		AssertQueuedVET(dStake).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  vStake,
			TotalLockedWeight: vStake,
			TotalExitingStake: stakeDecrease,
			TotalQueuedStake:  dStake,
			NextPeriodWeight:  (vStake-stakeDecrease)*2 + dStake*2,
		})

	// housekeep to lock the delegator and decrease the validator stake
	stakingPeriod := thor.MediumStakingPeriod()
	staker.AssertGlobalWithdrawable(0).
		Housekeep(stakingPeriod * 2).
		AssertGlobalWithdrawable(stakeDecrease)

	lockedVET := vStake - stakeDecrease + dStake
	lockedWeight := (vStake-stakeDecrease)*2 + dStake*2
	staker.
		AssertLockedVET(lockedVET, lockedWeight).
		AssertQueuedVET(0).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  lockedVET,
			TotalLockedWeight: lockedWeight,
			TotalExitingStake: 0,
			TotalQueuedStake:  0,
			NextPeriodWeight:  lockedWeight,
		})

	// signal an exit for the delegation
	staker.SignalDelegationExit(delegationID1, stakingPeriod*2+1)
	staker.
		AssertLockedVET(lockedVET, lockedWeight).
		AssertQueuedVET(0).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  lockedVET,
			TotalLockedWeight: lockedWeight,
			TotalExitingStake: dStake,
			TotalQueuedStake:  0,
			NextPeriodWeight:  lockedVET - dStake,
		})

	// housekeep and check after exit
	lockedVET = lockedVET - dStake
	staker.AssertGlobalWithdrawable(stakeDecrease).
		Housekeep(stakingPeriod*3).
		AssertGlobalWithdrawable(stakeDecrease+dStake).
		AssertLockedVET(lockedVET, lockedVET).
		AssertQueuedVET(0).
		AssertTotals(validator, &validation.Totals{
			TotalLockedStake:  lockedVET,
			TotalLockedWeight: lockedVET,
			TotalExitingStake: 0,
			TotalQueuedStake:  0,
			NextPeriodWeight:  lockedVET,
		})
}

func TestStaker_OfflineValidator(t *testing.T) {
	staker, _ := newStakerV2(t, 5, 5, true)

	validator1, val1 := staker.FirstActive()

	// setting validator offline will record offline block
	offlineBlock := uint32(4)
	staker.SetOnline(validator1, offlineBlock, false)

	staker.AssertValidation(validator1).
		OfflineBlock(&offlineBlock).
		ExitBlock(nil)

	// setting validator online will clear offline block
	staker.SetOnline(validator1, 8, true)

	staker.AssertValidation(validator1).
		OfflineBlock(nil).
		ExitBlock(nil)

	// setting validator offline will not trigger eviction until threshold is met
	staker.SetOnline(validator1, 8, false)
	// Epoch length is 180, 336 is the number of epochs in 7 days which is threshold, 8 is the block number when val wen't offline
	staker.Housekeep(thor.EpochLength() * 336)

	expectedOfflineBlock := uint32(8)
	staker.AssertValidation(validator1).
		OfflineBlock(&expectedOfflineBlock).
		ExitBlock(nil)

	// exit status is set to first free epoch after current one
	staker.Housekeep(thor.EpochLength() * 48 * 3 * 3)
	expectedExitBlock := (thor.EpochLength() * 48 * 3 * 3) + 180

	staker.AssertValidation(validator1).
		OfflineBlock(&expectedOfflineBlock).
		ExitBlock(&expectedExitBlock).
		Status(validation.StatusActive)

	// validator should exit here
	staker.AssertGlobalWithdrawable(0).
		Housekeep(expectedExitBlock).
		AssertGlobalCooldown(val1.LockedVET).
		AssertGlobalWithdrawable(0)

	staker.AssertValidation(validator1).
		OfflineBlock(&expectedOfflineBlock).
		ExitBlock(&expectedExitBlock).
		Status(validation.StatusExit)
}

func TestStaker_Housekeep_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	paramsAddr := thor.BytesToAddress([]byte("params"))

	param := params.New(paramsAddr, st)

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(2)))
	staker := &testStaker{
		Staker: New(stakerAddr, st, param, nil),
		addr:   stakerAddr,
		state:  st,
	}

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
		if err := staker.AddValidation(key.node, key.endorser, thor.MediumStakingPeriod(), stake); err != nil {
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
	staker := &testStaker{
		Staker: New(stakerAddr, st, param, nil),
		addr:   stakerAddr,
		state:  st,
	}

	node1 := datagen.RandAddress()
	stake := RandomStake()
	err := staker.AddValidation(node1, node1, thor.MediumStakingPeriod(), stake)
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
	staker, _ := newStakerV2(t, 0, 1, false)
	addr := datagen.RandAddress()
	endorser := datagen.RandAddress()

	staker.AddValidation(addr, endorser, thor.MediumStakingPeriod(), MinStakeVET)

	overflowDecrease := math.MaxUint64 - MinStakeVET - 1
	staker.DecreaseStakeErrors(addr, endorser, overflowDecrease, "decrease amount is too large")

	assertValidation(t, staker, addr).QueuedVET(MinStakeVET)
}

func TestValidation_IncreaseOverflow(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 1, false)
	addr := datagen.RandAddress()
	endorser := datagen.RandAddress()

	staker.AddValidation(addr, endorser, thor.MediumStakingPeriod(), MinStakeVET)

	overflowIncrease := math.MaxUint64 - MinStakeVET + 1
	staker.IncreaseStakeErrors(addr, endorser, overflowIncrease, "increase amount is too large")

	assertValidation(t, staker, addr).QueuedVET(MinStakeVET)
}

func TestValidation_WithdrawBeforeAfterCooldown(t *testing.T) {
	staker, _ := newStakerV2(t, 2, 2, true)

	first, val := staker.FirstActive()
	stake := val.LockedVET

	staker.AssertGlobalWithdrawable(0).
		SignalExit(first, val.Endorser, 1).
		Housekeep(thor.MediumStakingPeriod())

	assertValidation(t, staker, first).
		Status(validation.StatusExit).
		WithdrawableVET(0).
		CooldownVET(stake)

	staker.AssertGlobalCooldown(stake).
		AssertGlobalWithdrawable(0).
		WithdrawStake(first, val.Endorser, thor.MediumStakingPeriod()+thor.CooldownPeriod(), stake).
		AssertGlobalWithdrawable(0).
		AssertGlobalCooldown(0)
}
