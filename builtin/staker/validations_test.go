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

	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
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

func TestStaker_TotalStake(t *testing.T) {
	staker := newTest(t).SetMBP(14)
	totalStaked := uint64(0)

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
	staker := newTest(t).SetMBP(14)

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

	staker.AssertValidation(addr).
		Status(validation.StatusExit).
		CooldownVET(stakeAmount)

	staker.
		AssertWithdrawable(addr, period+thor.CooldownPeriod(), stakeAmount).
		WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stakeAmount)

	staker.AssertValidation(addr).
		Status(validation.StatusExit).
		WithdrawableVET(0)

	staker.
		AssertLockedVET(0, 0).
		AssertQueuedVET(0)
}

func TestStaker_AddValidation_MinimumStake(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

	tooLow := MinStakeVET - 1
	staker.AddValidationErrors(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), tooLow, "stake is below minimum")
	staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
}

func TestStaker_AddValidation_MaximumStake(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

	tooHigh := MaxStakeVET + 1
	staker.AddValidationErrors(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), tooHigh, "stake is above maximum")
	staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MaxStakeVET)
}

func TestStaker_AddValidation_MaximumStakingPeriod(t *testing.T) {
	staker := newTest(t).Fill(101).Transition(0)

	staker.AddValidationErrors(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*400, MinStakeVET, "period is out of boundaries")
	staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
}

func TestStaker_AddValidation_MinimumStakingPeriod(t *testing.T) {
	staker := newTest(t).Fill(101).Transition(0)

	staker.AddValidationErrors(datagen.RandAddress(), datagen.RandAddress(), uint32(360)*24*1, MinStakeVET, "period is out of boundaries")
	staker.AddValidationErrors(datagen.RandAddress(), datagen.RandAddress(), 100, MinStakeVET, "period is out of boundaries")
	staker.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
}

func TestStaker_AddValidation_Duplicate(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

	addr := datagen.RandAddress()
	stake := uint64(25e6)
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.AddValidationErrors(addr, addr, thor.MediumStakingPeriod(), stake, "validator already exists")
}

func TestStaker_AddValidation(t *testing.T) {
	staker := newTest(t).Fill(101).Transition(0)

	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()
	addr4 := datagen.RandAddress()

	stake := RandomStake()
	staker.AddValidation(addr1, addr1, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr1).Status(validation.StatusQueued).QueuedVET(stake)

	staker.AddValidation(addr2, addr2, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr2).Status(validation.StatusQueued).QueuedVET(stake)

	staker.AddValidation(addr3, addr3, thor.HighStakingPeriod(), stake)
	staker.AssertValidation(addr3).Status(validation.StatusQueued).QueuedVET(stake)

	staker.AddValidationErrors(addr4, addr4, uint32(360)*24*14, stake, "period is out of boundaries")
	val := staker.GetValidation(addr4)
	assert.Nil(t, val)
}

func TestStaker_QueueUpValidators(t *testing.T) {
	staker := newTest(t)

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
	staker := newTest(t).Fill(101).Transition(0)

	id := datagen.RandAddress()
	validator := staker.GetValidation(id)
	assert.Nil(t, validator)
}

func TestStaker_Get(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

	addr := datagen.RandAddress()
	stake := RandomStake()
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake)
	staker.ActivateNext(0)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake)
}

func TestStaker_Get_FullFlow_Renewal_Off(t *testing.T) {
	staker := newTest(t).SetMBP(3)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.AddValidation(addr, addr, period, stake)
	staker.AddValidation(addr1, addr1, period, stake)
	staker.AddValidation(addr2, addr2, period, stake)

	staker.AssertValidationNums(0, 3)
	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// activate the validator
	staker.ActivateNext(0)
	staker.AssertValidationNums(1, 2)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake).Weight(stake)

	// activate next
	staker.ActivateNext(0)
	staker.AssertValidationNums(2, 1)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake).Weight(stake)

	// activate next
	staker.ActivateNext(0)
	staker.AssertValidationNums(3, 0)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake).Weight(stake)

	// signal exit
	staker.SignalExit(addr, addr, 10)

	// housekeep the validator
	staker.Housekeep(period)
	staker.AssertValidation(addr).Status(validation.StatusExit).LockedVET(0).Weight(0).CooldownVET(stake)
	staker.AssertValidationNums(2, 0)

	// withdraw the stake
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stake)
}

func TestStaker_WithdrawQueued(t *testing.T) {
	staker := newTest(t)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// verify queued empty
	staker.AssertFirstQueued(thor.Address{})

	// add the validator
	period := thor.MediumStakingPeriod()
	staker.AddValidation(addr, addr, period, stake)
	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// verify queued
	staker.AssertFirstQueued(addr)

	// withraw queued
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stake)

	// verify removed queued
	staker.AssertFirstQueued(thor.Address{})
}

func TestStaker_IncreaseQueued(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)
	addr := datagen.RandAddress()
	stake := RandomStake()

	staker.IncreaseStakeErrors(addr, thor.Address{}, stake, "validation does not exist")

	// add the validator
	staker.AddValidation(addr, addr, uint32(360)*24*15, stake)

	validator := staker.GetValidation(addr)
	assert.Equal(t, validation.StatusQueued, validator.Status)
	assert.Equal(t, stake, validator.QueuedVET)
	assert.Equal(t, uint64(0), validator.Weight)

	staker.Housekeep(validator.Period)

	// increase stake queued
	expectedStake := 1000 + stake
	staker.IncreaseStake(addr, addr, 1000)
	validator = staker.GetValidation(addr)
	newAmount := validator.QueuedVET + validator.LockedVET
	assert.Equal(t, newAmount, expectedStake)

	staker.Housekeep(validator.Period)

	validator = staker.GetValidation(addr)
	assert.False(t, validator == nil)
	assert.Equal(t, validator.Status, validation.StatusActive)
	assert.Equal(t, validator.LockedVET, expectedStake)
	assert.Equal(t, expectedStake, validator.Weight)
}

func TestStaker_IncreaseActive(t *testing.T) {
	staker := newTest(t)
	addr := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.AddValidation(addr, addr, period, stake).ActivateNext(0)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake).Weight(stake)
	// increase stake of an active validator
	staker.IncreaseStake(addr, addr, 1000)
	staker.AssertValidation(addr).QueuedVET(1000).LockedVET(stake)

	// verify withdraw amount decrease
	staker.Housekeep(period)
	staker.AssertValidation(addr).Weight(stake + 1000).LockedVET(stake + 1000)
}

func TestStaker_ChangeStakeActiveValidatorWithQueued(t *testing.T) {
	staker := newTest(t).SetMBP(1)
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
	staker := newTest(t)
	addr := datagen.RandAddress()
	stake := MaxStakeVET
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.AddValidation(addr, addr, period, stake).ActivateNext(0)
	staker.AssertValidation(addr).
		Status(validation.StatusActive).
		LockedVET(stake).
		Weight(stake).
		// verify withdraw is empty
		WithdrawableVET(0)

	// decrease stake of an active validator
	decrease := uint64(1000)
	staker.DecreaseStake(addr, addr, decrease)
	staker.AssertValidation(addr).LockedVET(stake).PendingUnlockVET(decrease).Weight(stake)

	// verify withdraw amount decrease
	staker.Housekeep(period)
	staker.AssertValidation(addr).LockedVET(stake - decrease).PendingUnlockVET(0).WithdrawableVET(decrease)
}

func TestStaker_DecreaseActiveThenExit(t *testing.T) {
	staker := newTest(t)
	addr := datagen.RandAddress()
	stake := MaxStakeVET
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.AddValidation(addr, addr, period, stake).ActivateNext(0)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake).Weight(stake).WithdrawableVET(0)

	// decrease stake of an active validator
	decrease := uint64(1000)
	expectedStake := stake - decrease
	staker.DecreaseStake(addr, addr, decrease)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake).Weight(stake).PendingUnlockVET(decrease)

	// verify withdraw amount decrease
	staker.Housekeep(period)
	staker.AssertValidation(addr).WithdrawableVET(decrease).LockedVET(stake - decrease).PendingUnlockVET(0)

	staker.SignalExit(addr, addr, 129600)
	staker.Housekeep(period * 2)
	staker.AssertValidation(addr).Status(validation.StatusExit).WithdrawableVET(decrease).LockedVET(0).CooldownVET(expectedStake)
}

func TestStaker_Get_FullFlow(t *testing.T) {
	staker := newTest(t).SetMBP(3)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.AddValidation(addr, addr, period, stake)
	staker.AddValidation(addr1, addr1, period, stake)
	staker.AddValidation(addr2, addr2, period, stake)

	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)
	staker.ActivateNext(0)

	// activate the validator
	staker.AssertValidation(addr).Status(validation.StatusActive).QueuedVET(0).Weight(stake).LockedVET(stake)
	staker.ActivateNext(0)
	staker.ActivateNext(0)

	staker.SignalExit(addr, addr, 10)

	// housekeep the validator
	staker.Housekeep(period)
	staker.AssertValidation(addr).Status(validation.StatusExit).QueuedVET(0).Weight(0).LockedVET(0).CooldownVET(stake)

	staker.Housekeep(period + thor.CooldownPeriod())
	staker.AssertValidation(addr).Status(validation.StatusExit).QueuedVET(0).Weight(0).LockedVET(0).CooldownVET(stake)

	// withdraw the stake
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stake)
}

func TestStaker_Get_FullFlow_Renewal_On(t *testing.T) {
	staker := newTest(t)
	addr := datagen.RandAddress()
	stake := RandomStake()

	// add the validator
	period := thor.MediumStakingPeriod()
	staker.AddValidation(addr, addr, period, stake)
	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// activate the validator
	staker.ActivateNext(0)
	staker.AssertValidation(addr).Status(validation.StatusActive).QueuedVET(0).Weight(stake).LockedVET(stake)

	// housekeep the validator
	staker.Housekeep(period)
	staker.AssertValidation(addr).Status(validation.StatusActive).QueuedVET(0).Weight(stake).LockedVET(stake)

	// withdraw the stake
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), 0)
}

func TestStaker_Get_FullFlow_Renewal_On_Then_Off(t *testing.T) {
	staker := newTest(t).SetMBP(3)
	addr := datagen.RandAddress()
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// add the validator
	staker.
		AddValidation(addr, addr, period, stake).
		AddValidation(addr1, addr1, period, stake).
		AddValidation(addr2, addr2, period, stake)

	staker.AssertValidation(addr).Status(validation.StatusQueued).QueuedVET(stake).Weight(0)

	// activate the validator
	staker.ActivateNext(0)
	staker.AssertValidation(addr).Status(validation.StatusActive).LockedVET(stake).Weight(stake)

	staker.ActivateNext(0)
	staker.ActivateNext(0)

	// housekeep the validator
	staker.Housekeep(period)
	staker.AssertValidation(addr).Status(validation.StatusActive).QueuedVET(0).Weight(stake)

	// withdraw the stake
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), 0)
	staker.SignalExit(addr, addr, 0)

	// housekeep the validator
	staker.Housekeep(period)
	staker.AssertValidation(addr).Status(validation.StatusExit).Weight(0).LockedVET(0).CooldownVET(stake)

	// withdraw the stake
	staker.WithdrawStake(addr, addr, period+thor.CooldownPeriod(), stake)
	staker.AssertValidation(addr).Status(validation.StatusExit).WithdrawableVET(0)
}

func TestStaker_ActivateNextValidator_LeaderGroupFull(t *testing.T) {
	staker := newTest(t)

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
	staker := newTest(t)
	staker.ActivateNextErrors(0, "no validator in the queue")
}

func TestStaker_ActivateNextValidator(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

	addr := datagen.RandAddress()
	stake := RandomStake()
	staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
	staker.ActivateNext(0)

	staker.AssertValidation(addr).Status(validation.StatusActive)
}

func TestStaker_RemoveValidator_NonExistent(t *testing.T) {
	staker := newTest(t).Fill(101).Transition(0)

	addr := datagen.RandAddress()
	staker.ExitValidatorErrors(addr, "failed to get existing validator")
}

func TestStaker_RemoveValidator(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

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
	test := newTest(t).Fill(68).Transition(0)

	added := make(map[thor.Address]bool)
	for range 10 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		test.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
		test.ActivateNext(0)
	}

	leaderGroup, err := test.LeaderGroup()
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
	staker := newTest(t).Fill(101).Transition(0)

	id := datagen.RandAddress()
	next, _ := staker.Next(id)
	assert.True(t, next.IsZero())
}

func TestStaker_Next(t *testing.T) {
	staker := newTest(t)

	leaderGroup := make([]thor.Address, 0)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake).ActivateNext(0)
		leaderGroup = append(leaderGroup, addr)
	}

	queuedGroup := make([]thor.Address, 0, 100)
	for range 100 {
		addr := datagen.RandAddress()
		stake := RandomStake()
		staker.AddValidation(addr, addr, thor.MediumStakingPeriod(), stake)
		queuedGroup = append(queuedGroup, addr)
	}

	firstLeader, _ := staker.FirstActive()
	assert.Equal(t, leaderGroup[0], firstLeader)

	for i := range 99 {
		next, _ := staker.Next(leaderGroup[i])
		assert.Equal(t, leaderGroup[i+1], next)
	}

	firstQueued, _ := staker.FirstQueued()

	current := firstQueued
	for i := range 100 {
		staker.GetValidation(current)
		assert.Equal(t, queuedGroup[i], current)

		next, _ := staker.Next(current)
		current = next
	}
}

func TestStaker_Initialise(t *testing.T) {
	test := newTest(t).SetMBP(3)
	addr := datagen.RandAddress()

	for range 3 {
		test.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	}

	transitioned, err := test.transition(0)
	assert.NoError(t, err) // should succeed
	assert.True(t, transitioned)
	// should be able to add validations after initialisation
	test.AddValidation(addr, addr, thor.MediumStakingPeriod(), MinStakeVET)

	test = newTest(t).Fill(101).Transition(0)
	first, _ := test.FirstActive()
	assert.False(t, first.IsZero())

	expectedLength := uint64(101)
	length, err := test.validationService.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, expectedLength, length)
}

func TestStaker_Housekeep_TooEarly(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)
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
	staker := newTest(t).SetMBP(3)
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
	staker := newTest(t).SetMBP(3)
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
	staker := newTest(t).SetMBP(3)
	addr1 := datagen.RandAddress()
	addr2 := datagen.RandAddress()
	addr3 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.
		AddValidation(addr1, addr1, period, stake).ActivateNext(0).
		AddValidation(addr2, addr2, period, stake).ActivateNext(0).
		AddValidation(addr3, addr3, period, stake).ActivateNext(0)

	// disable auto renew
	staker.
		SignalExit(addr1, addr1, 10).
		SignalExit(addr2, addr2, 10).
		SignalExit(addr3, addr3, 10)

	staker.Housekeep(period)
	staker.AssertValidation(addr1).Status(validation.StatusExit)
	staker.AssertValidation(addr2).Status(validation.StatusActive)

	staker.Housekeep(period + thor.EpochLength())
	staker.AssertValidation(addr1).Status(validation.StatusExit)
	staker.AssertValidation(addr2).Status(validation.StatusExit)
}

func TestStaker_Housekeep_ExitOrder(t *testing.T) {
	staker := newTest(t).SetMBP(3)
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
	staker := newTest(t)
	addr1 := datagen.RandAddress()

	stake := MinStakeVET
	period := thor.MediumStakingPeriod()

	// auto renew is turned on
	staker.AddValidation(addr1, addr1, period, stake).ActivateNext(0)
	staker.IncreaseStake(addr1, addr1, 1)

	// housekeep half way through the period, validator's locked vet should not change
	staker.Housekeep(period / 2)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake).QueuedVET(1).Weight(stake)

	staker.Housekeep(period)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake + 1).Weight(stake + 1).QueuedVET(0).WithdrawableVET(0)
}

func TestStaker_Housekeep_RecalculateDecrease(t *testing.T) {
	staker := newTest(t)
	addr1 := datagen.RandAddress()

	stake := MaxStakeVET
	period := thor.MediumStakingPeriod()

	// auto renew is turned on
	staker.AddValidation(addr1, addr1, period, stake).ActivateNext(0)

	decrease := uint64(1)
	staker.DecreaseStake(addr1, addr1, decrease)

	block := uint32(360) * 24 * 13
	staker.Housekeep(block)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake).Weight(stake).PendingUnlockVET(decrease)

	block = thor.MediumStakingPeriod()
	staker.Housekeep(block)
	staker.AssertValidation(addr1).
		Status(validation.StatusActive).
		LockedVET(stake - decrease).
		Weight(stake - decrease).
		PendingUnlockVET(0).
		WithdrawableVET(decrease)
}

func TestStaker_Housekeep_DecreaseThenWithdraw(t *testing.T) {
	staker := newTest(t)
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
	staker := newTest(t)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	// auto renew is turned on
	staker.AddValidation(addr1, addr1, period, stake).ActivateNext(0)
	staker.DecreaseStake(addr1, addr1, 1)
	staker.AssertValidation(addr1).LockedVET(stake).PendingUnlockVET(1)

	staker.DecreaseStake(addr1, addr1, 1)
	staker.AssertValidation(addr1).LockedVET(stake).PendingUnlockVET(2)

	staker.Housekeep(period)
	staker.AssertValidation(addr1).LockedVET(stake - 2).WithdrawableVET(2).CooldownVET(0)
}

func TestStaker_Housekeep_Exit_Decrements_Leader_Group_Size(t *testing.T) {
	staker := newTest(t).SetMBP(3)
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

	staker.AssertValidation(addr1).Status(validation.StatusExit)
	staker.AssertValidation(addr2).Status(validation.StatusActive)

	block := period + thor.EpochLength()
	staker.
		AssertGlobalCooldown(stake).
		AssertGlobalWithdrawable(0).
		Housekeep(block).
		AssertGlobalCooldown(stake * 2).
		AssertGlobalWithdrawable(0).
		AssertLeaderGroupSize(0).
		AssertFirstActive(thor.Address{})

	staker.AssertValidation(addr1).Status(validation.StatusExit)

	staker.
		AddValidation(addr3, addr3, period, stake).
		ActivateNext(block).
		SignalExit(addr3, addr3, block+period-1).
		AssertFirstActive(addr3).
		AssertLeaderGroupSize(1)

	staker.AssertValidation(addr3).Status(validation.StatusActive)

	block = block + period
	staker.Housekeep(block).AssertGlobalWithdrawable(0).AssertGlobalCooldown(stake * 3)

	staker.AssertValidation(addr3).Status(validation.StatusExit)
}

func TestStaker_Housekeep_Adds_Queued_Validators_Up_To_Limit(t *testing.T) {
	staker := newTest(t).SetMBP(2)
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
	staker := newTest(t).SetMBP(3)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(addr1, addr1, period, stake)
	staker.WithdrawStake(addr1, addr1, period, stake)
	staker.AssertValidation(addr1).Status(validation.StatusExit).QueuedVET(0).WithdrawableVET(0).LockedVET(0)
}

func TestStaker_IncreaseStake_Withdraw(t *testing.T) {
	staker := newTest(t).SetMBP(3)
	addr1 := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(addr1, addr1, period, stake)
	staker.Housekeep(period)
	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake)

	staker.IncreaseStake(addr1, addr1, 100)
	staker.WithdrawStake(addr1, addr1, period+thor.CooldownPeriod(), 100)

	staker.AssertValidation(addr1).Status(validation.StatusActive).LockedVET(stake).WithdrawableVET(0).QueuedVET(0)
}

func TestStaker_GetRewards(t *testing.T) {
	staker := newTest(t).SetMBP(3)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(proposerAddr, proposerAddr, period, stake).ActivateNext(0)
	staker.AssertDelegatorRewards(proposerAddr, 1, big.NewInt(0))

	reward := big.NewInt(1000)
	staker.IncreaseDelegatorsReward(proposerAddr, reward, 10)

	staker.AssertDelegatorRewards(proposerAddr, 1, reward)
}

func TestStaker_GetCompletedPeriods(t *testing.T) {
	staker := newTest(t).SetMBP(3)

	proposerAddr := datagen.RandAddress()

	stake := RandomStake()
	period := thor.MediumStakingPeriod()

	staker.AddValidation(proposerAddr, proposerAddr, period, stake).ActivateNext(0)
	staker.AssertValidation(proposerAddr).CompletedIterations(0, period-1)

	staker.Housekeep(period)
	staker.AssertValidation(proposerAddr).CompletedIterations(1, period)
}

func TestStaker_MultipleUpdates_CorrectWithdraw(t *testing.T) {
	staker := newTest(t).SetMBP(1)

	acc := datagen.RandAddress()
	initialStake := RandomStake()
	increases := uint64(0)
	withdrawnTotal := uint64(0)
	thousand := uint64(1000)
	fiveHundred := uint64(500)

	period := uint32(360) * 24 * 15

	// QUEUED
	staker.AddValidation(acc, acc, period, initialStake)

	validator := staker.GetValidation(acc)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	// 1st STAKING PERIOD
	staker.Housekeep(period)

	validator = staker.GetValidation(acc)
	assert.Equal(t, validation.StatusActive, validator.Status)
	assert.Equal(t, initialStake, validator.LockedVET)

	// Now that validator is active, we can increase/decrease stake
	increases += thousand
	staker.IncreaseStake(acc, acc, thousand)
	// 1st decrease
	staker.DecreaseStake(acc, acc, fiveHundred)

	validator = staker.GetValidation(acc)
	// LockedVET stays the same, increases go to QueuedVET
	assert.Equal(t, initialStake, validator.LockedVET)
	assert.Equal(t, thousand, validator.QueuedVET)
	assert.Equal(t, fiveHundred, validator.PendingUnlockVET)

	// There should be some withdrawable amount available
	staker.WithdrawStake(acc, acc, period+1, 1000)
	withdrawnTotal += 1000

	validator = staker.GetValidation(acc)
	assert.Equal(t, initialStake, validator.LockedVET)

	// 2nd decrease
	staker.DecreaseStake(acc, acc, thousand)
	increases += fiveHundred
	staker.IncreaseStake(acc, acc, fiveHundred)

	// 2nd STAKING PERIOD - this will process the increases and decreases
	staker.Housekeep(period * 2)
	validator = staker.GetValidation(acc)
	assert.Equal(t, validation.StatusActive, validator.Status)

	// Check what's withdrawable after housekeep
	staker.WithdrawStake(acc, acc, period*2+thor.CooldownPeriod(), 1500)
	withdrawnTotal += 1500

	staker.SignalExit(acc, acc, period*2)

	// EXITED
	staker.Housekeep(period * 3)

	validator = staker.GetValidation(acc)
	assert.Equal(t, validation.StatusExit, validator.Status)

	// Withdraw the final amount
	expectedWithdraw := initialStake - withdrawnTotal + increases
	staker.WithdrawStake(acc, acc, period*3+thor.CooldownPeriod(), expectedWithdraw)
	withdrawnTotal += expectedWithdraw

	// Total deposits = initial + increases, total withdrawals should match
	depositTotal := initialStake + increases
	assert.Equal(t, depositTotal, withdrawnTotal)
}

func Test_GetValidatorTotals_ValidatorExiting(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	dStake := stakes.NewWeightedStakeWithMultiplier(MinStakeVET, 255)
	staker.AddDelegation(validator.ID, dStake.VET, 255, 10)

	vStake := stakes.NewWeightedStakeWithMultiplier(validators[0].LockedVET, validation.Multiplier)

	staker.AssertTotals(validator.ID, &validation.Totals{
		TotalQueuedStake:  dStake.VET,
		TotalLockedWeight: vStake.Weight,
		TotalLockedStake:  vStake.VET,
		TotalExitingStake: 0,
		NextPeriodWeight:  vStake.Weight + dStake.Weight + vStake.Weight,
	})

	vStake.Weight += validators[0].LockedVET
	staker.
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

	vStake := stakes.NewWeightedStakeWithMultiplier(validators[0].LockedVET, validation.Multiplier)
	dStake := stakes.NewWeightedStakeWithMultiplier(MinStakeVET, 255)

	delegationID := staker.AddDelegation(validator.ID, dStake.VET, 255, 10)

	staker.AssertTotals(validator.ID, &validation.Totals{
		TotalQueuedStake:  dStake.VET,
		TotalLockedWeight: vStake.Weight,
		TotalLockedStake:  vStake.VET,
		NextPeriodWeight:  vStake.Weight + dStake.Weight + vStake.VET,
	})

	vStake.Weight += validators[0].LockedVET
	staker.
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
	staker := newTest(t).SetMBP(3)

	acc := datagen.RandAddress()

	originalStake := uint64(3) * MinStakeVET
	staker.AddValidation(acc, acc, thor.LowStakingPeriod(), originalStake).ActivateNext(0)

	// Decrease stake
	decrease := uint64(2) * MinStakeVET
	staker.DecreaseStake(acc, acc, decrease)

	// Turn off auto-renew  - can't decrease if auto-renew is false
	staker.SignalExit(acc, acc, thor.LowStakingPeriod()-1)

	// Housekeep, should exit the validator
	staker.Housekeep(thor.LowStakingPeriod())

	staker.AssertValidation(acc).Status(validation.StatusExit).CooldownVET(originalStake)
}

func Test_Validator_Decrease_SeveralTimes(t *testing.T) {
	staker := newTest(t).SetMBP(1)

	acc := datagen.RandAddress()

	originalStake := uint64(3) * MinStakeVET
	staker.AddValidation(acc, acc, thor.LowStakingPeriod(), originalStake).ActivateNext(0)

	staker.DecreaseStake(acc, acc, MinStakeVET)
	staker.DecreaseStake(acc, acc, MinStakeVET)

	// Decrease stake - should fail, min stake is 25m
	staker.DecreaseStakeErrors(acc, acc, MinStakeVET, "next period stake is lower than minimum stake")
}

func Test_Validator_IncreaseDecrease_Combinations(t *testing.T) {
	staker := newTest(t).SetMBP(1)
	acc := datagen.RandAddress()

	// Add validator
	staker.AddValidation(acc, acc, thor.LowStakingPeriod(), MinStakeVET)

	// Increase and decrease should fail on queued validators
	staker.IncreaseStakeErrors(acc, acc, MinStakeVET, "can't increase stake while validator not active")
	staker.DecreaseStakeErrors(acc, acc, MinStakeVET, "can't decrease stake while validator not active")

	// Activate the validator
	staker.ActivateNext(0)

	// Verify validator is active with expected initial state
	val := staker.GetValidation(acc)
	assert.Equal(t, validation.StatusActive, val.Status)
	assert.Equal(t, MinStakeVET, val.LockedVET)
	assert.Equal(t, uint64(0), val.QueuedVET)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(0), val.WithdrawableVET)

	// Now operations should work on active validator
	staker.IncreaseStake(acc, acc, MinStakeVET)

	// Check validator state after operations
	val = staker.GetValidation(acc)
	assert.Equal(t, MinStakeVET, val.LockedVET)
	assert.Equal(t, MinStakeVET, val.QueuedVET)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)

	staker.DecreaseStakeErrors(acc, acc, MinStakeVET, "next period stake is lower than minimum stake")

	// Check if there are any withdrawals available (there shouldn't be any yet)
	staker.WithdrawStake(acc, acc, 0, MinStakeVET)

	// Process through housekeep to apply the changes
	staker.Housekeep(thor.LowStakingPeriod())

	// After housekeep, check final state
	validator := staker.GetValidation(acc)
	assert.True(t, validator.LockedVET >= MinStakeVET, "locked VET should be at least minimum stake")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMaster(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

	address := datagen.RandAddress()
	staker.AddValidation(address, datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET)
	staker.AddValidationErrors(address, datagen.RandAddress(), thor.MediumStakingPeriod(), MinStakeVET, "validator already exists")
}

func TestStaker_AddValidation_CannotAddValidationWithSameMasterAfterExit(t *testing.T) {
	staker := newTest(t).Fill(68).Transition(0)

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
	staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

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
	staker := newTest(t).SetMBP(1)

	master := datagen.RandAddress()
	endorser := datagen.RandAddress()
	beneficiary := datagen.RandAddress()

	// add validation without a beneficiary
	staker.AddValidation(master, endorser, thor.MediumStakingPeriod(), MinStakeVET).ActivateNext(0)
	staker.AssertValidation(master).Beneficiary(thor.Address{})

	// negative cases
	staker.SetBeneficiaryErrors(master, master, beneficiary, "endorser required")
	staker.SetBeneficiaryErrors(endorser, endorser, beneficiary, "validation does not exist")

	// set beneficiary, should be successful
	staker.SetBeneficiary(master, endorser, beneficiary)
	staker.AssertValidation(master).Beneficiary(beneficiary)

	// remove the beneficiary
	staker.SetBeneficiary(master, endorser, thor.Address{})
	staker.AssertValidation(master).Beneficiary(thor.Address{})
}

func TestStaker_TestWeights(t *testing.T) {
	staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

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
	validator2 := datagen.RandAddress()
	endorser := datagen.RandAddress()
	staker.AddValidation(validator2, endorser, thor.MediumStakingPeriod(), stake)

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
	staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

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
	staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

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
	staker := newTest(t).SetMBP(5).Fill(5).Transition(0)

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
	test := newTest(t).SetMBP(2)

	housekeep, err := test.Staker.Housekeep(thor.EpochLength() - 1)
	assert.NoError(t, err)
	assert.False(t, housekeep)

	st := test.State()
	stakerAddr := test.Address()

	activeHeadSlot := thor.BytesToBytes32([]byte(("validations-active-head")))
	st.SetRawStorage(stakerAddr, activeHeadSlot, rlp.RawValue{0xFF})

	_, err = test.Staker.Housekeep(thor.EpochLength() * 48 * 3)
	assert.Error(t, err)

	st.SetRawStorage(stakerAddr, activeHeadSlot, rlp.RawValue{0x0})
	slotLockedVET := thor.BytesToBytes32([]byte(("total-weighted-stake")))
	valAddr := datagen.RandAddress()
	test.AddValidation(valAddr, datagen.RandAddress(), thor.MediumStakingPeriod(), RandomStake())
	test.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), RandomStake())

	lockedVet, err := st.GetRawStorage(stakerAddr, slotLockedVET)
	assert.NoError(t, err)
	st.SetRawStorage(stakerAddr, slotLockedVET, rlp.RawValue{0xFF})
	_, err = test.Staker.Housekeep(thor.EpochLength())
	assert.Error(t, err)

	_, err = test.Staker.Housekeep(thor.EpochLength() * 2)
	assert.Error(t, err)

	slotQueuedGroupSize := thor.BytesToBytes32([]byte(("validations-queued-group-size")))
	st.SetRawStorage(stakerAddr, slotLockedVET, lockedVet)
	st.SetRawStorage(stakerAddr, slotQueuedGroupSize, rlp.RawValue{0xFF})
	_, err = test.Staker.Housekeep(thor.EpochLength() * 4)
	assert.Error(t, err)

	st.SetRawStorage(stakerAddr, slotLockedVET, rlp.RawValue{0xc2, 0x80, 0x80})
	st.SetRawStorage(stakerAddr, slotQueuedGroupSize, rlp.RawValue{0x0})

	slotActiveGroupSize := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	st.SetRawStorage(stakerAddr, slotActiveGroupSize, rlp.RawValue{0xFF})
	count, err := test.computeActivationCount(true)
	assert.Error(t, err)
	assert.Equal(t, uint64(0), count)

	st.SetRawStorage(stakerAddr, slotActiveGroupSize, rlp.RawValue{0x0})
	st.SetRawStorage(thor.BytesToAddress([]byte("params")), thor.KeyMaxBlockProposers, rlp.RawValue{0xFF})
	count, err = test.computeActivationCount(true)
	assert.Error(t, err)
	assert.Equal(t, uint64(0), count)

	slotAggregations := thor.BytesToBytes32([]byte("aggregated-delegations"))
	validatorAddr := thor.BytesToAddress([]byte("renewal1"))
	slot := thor.Blake2b(validatorAddr.Bytes(), slotAggregations.Bytes())
	st.SetRawStorage(stakerAddr, slot, []byte{0xFF, 0xFF, 0xFF, 0xFF})
	assert.NoError(t, test.params.Set(thor.KeyMaxBlockProposers, big.NewInt(0)))
	err = test.applyEpochTransition(&EpochTransition{
		Block:           0,
		Renewals:        []thor.Address{validatorAddr},
		ExitValidator:   thor.Address{},
		Evictions:       nil,
		ActivationCount: 0,
	})
	assert.ErrorContains(t, err, "failed to get validator aggregation")
	re2 := thor.BytesToAddress([]byte("renewal2"))
	err = test.applyEpochTransition(&EpochTransition{
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

	err = test.applyEpochTransition(&EpochTransition{
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
	err = test.applyEpochTransition(&EpochTransition{
		Block:           0,
		Renewals:        []thor.Address{},
		ExitValidator:   valAddr,
		Evictions:       nil,
		ActivationCount: 0,
	})

	assert.ErrorContains(t, err, "failed to get validator")
}

func TestValidation_NegativeCases(t *testing.T) {
	staker := newTest(t).SetMBP(2)

	node1 := datagen.RandAddress()
	stake := RandomStake()
	staker.AddValidation(node1, node1, thor.MediumStakingPeriod(), stake)

	validationsSlot := thor.BytesToBytes32([]byte(("validations")))
	slot := thor.Blake2b(node1.Bytes(), validationsSlot.Bytes())
	staker.State().SetRawStorage(staker.Address(), slot, rlp.RawValue{0xFF})
	_, err := staker.GetWithdrawable(node1, thor.EpochLength())
	assert.Error(t, err)

	_, err = staker.GetValidationTotals(node1)
	assert.Error(t, err)

	staker.WithdrawStakeErrors(node1, node1, thor.EpochLength(), "state: rlp")
	staker.SignalExitErrors(node1, node1, 10, "state: rlp")
	staker.SignalDelegationExitErrors(big.NewInt(0), 10, "delegation is empty")
	staker.GetValidationErrors(node1, "state: rlp")
}

func TestValidation_DecreaseOverflow(t *testing.T) {
	staker := newTest(t).SetMBP(1)
	addr := datagen.RandAddress()
	endorser := datagen.RandAddress()

	staker.AddValidation(addr, endorser, thor.MediumStakingPeriod(), MinStakeVET)

	overflowDecrease := math.MaxUint64 - MinStakeVET - 1
	staker.DecreaseStakeErrors(addr, endorser, overflowDecrease, "decrease amount is too large")

	staker.AssertValidation(addr).QueuedVET(MinStakeVET)
}

func TestValidation_IncreaseOverflow(t *testing.T) {
	staker := newTest(t).SetMBP(1)
	addr := datagen.RandAddress()
	endorser := datagen.RandAddress()

	staker.AddValidation(addr, endorser, thor.MediumStakingPeriod(), MinStakeVET)
	staker.Housekeep(thor.MediumStakingPeriod())

	overflowIncrease := math.MaxUint64 - MinStakeVET + 1
	staker.IncreaseStakeErrors(addr, endorser, overflowIncrease, "increase amount is too large")

	staker.AssertValidation(addr).LockedVET(MinStakeVET)
}

func TestValidation_WithdrawBeforeAfterCooldown(t *testing.T) {
	staker := newTest(t).SetMBP(2).Fill(2).Transition(0)

	first, val := staker.FirstActive()
	stake := val.LockedVET

	staker.AssertGlobalWithdrawable(0).
		SignalExit(first, val.Endorser, 1).
		Housekeep(thor.MediumStakingPeriod())

	staker.AssertValidation(first).
		Status(validation.StatusExit).
		WithdrawableVET(0).
		CooldownVET(stake)

	staker.AssertGlobalCooldown(stake).
		AssertGlobalWithdrawable(0).
		WithdrawStake(first, val.Endorser, thor.MediumStakingPeriod()+thor.CooldownPeriod(), stake).
		AssertGlobalWithdrawable(0).
		AssertGlobalCooldown(0)
}
