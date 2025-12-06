// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

func TestMultipleDelegations(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	var delegationIDs []*big.Int
	for i := range 5 {
		amount := uint64(1000 + i*100)
		multiplier := uint8(100 + i*10)
		delegationID := staker.AddDelegation(validator, amount, multiplier, 10)
		delegationIDs = append(delegationIDs, delegationID)
	}

	// Verify all delegations were created successfully
	assert.Len(t, delegationIDs, 5, "should have created 5 delegations")

	// Verify each delegation exists and has correct properties
	for i, delegationID := range delegationIDs {
		delegation := staker.GetDelegation(delegationID)
		assert.NotNil(t, delegation, "delegation %d should exist", i)
		assert.Equal(t, validator, delegation.Validation(), "delegation %d should point to correct validator", i)

		expectedAmount := uint64(1000 + i*100)
		expectedMultiplier := uint8(100 + i*10)
		assert.Equal(t, expectedAmount, delegation.Stake(), "delegation %d stake should be correct", i)
		assert.Equal(t, expectedMultiplier, delegation.Multiplier(), "delegation %d multiplier should be correct", i)
	}

	staker.Housekeep(thor.MediumStakingPeriod())

	// Verify validator has delegations
	hasDelegations, err := staker.HasDelegations(validator)
	assert.NoError(t, err, "should be able to check delegations")
	assert.True(t, hasDelegations, "validator should have delegations")
}

func TestMultipleDelegationsSignalingExit(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	// Add multiple delegations first
	delegationIDs := make([]*big.Int, 5)
	for i := range 5 {
		amount := uint64(1000 + i*100)
		multiplier := uint8(100 + i*10)
		delegationIDs[i] = staker.AddDelegation(validator, amount, multiplier, 10)
	}

	// Wait for delegations to start
	staker.Housekeep(thor.MediumStakingPeriod())

	for _, delegationID := range delegationIDs {
		staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)
	}

	// Verify all delegations are signaled for exit
	for i, delegationID := range delegationIDs {
		delegation := staker.GetDelegation(delegationID)
		assert.NotNil(t, delegation.LastIteration(), "delegation %d should be signaled for exit", i)
		assert.Equal(t, uint64(1000+i*100), delegation.Stake(), "delegation %d should retain stake", i)
	}
}

func TestValidatorAndDelegationOperations(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	leaders, err := staker.LeaderGroup()
	require.NoError(t, err, "should be able to get leader group")
	require.Len(t, leaders, 3, "should have 3 active validators")

	validator1 := leaders[0].Address
	validator2 := leaders[1].Address
	val1 := staker.GetValidation(validator1)
	val2 := staker.GetValidation(validator2)

	require.NotEqual(t, thor.Address{}, validator1, "validator1 should be active")
	require.NotEqual(t, thor.Address{}, validator2, "validator2 should be active")

	// Sequential operations (simulating multiple transactions in order)

	// Transaction 1: Add delegation to validator1
	delegationID1 := staker.AddDelegation(validator1, 1000, 150, 10)
	assert.NotNil(t, delegationID1, "should be able to add delegation to validator1")

	// Transaction 2: Add delegation to validator2
	delegationID2 := staker.AddDelegation(validator2, 2000, 200, 10)
	assert.NotNil(t, delegationID2, "should be able to add delegation to validator2")

	// Transaction 3: Signal validator1 exit
	staker.SignalExit(validator1, val1.Endorser(), thor.MediumStakingPeriod()+20)

	// Transaction 4: Increase stake for validator2
	staker.IncreaseStake(validator2, val2.Endorser(), 500)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Verify final states
	val1Final := staker.GetValidation(validator1)
	val2Final := staker.GetValidation(validator2)

	// Validator1 should have exit block set
	assert.NotNil(t, val1Final.ExitBlock(), "validator1 should have exit block set")

	// Validator2 should have increased stake
	assert.Greater(t, val2Final.LockedVET(), val2.LockedVET(), "validator2 should have increased stake")

	// Verify delegations exist
	del1 := staker.GetDelegation(delegationID1)
	del2 := staker.GetDelegation(delegationID2)
	assert.NotNil(t, del1, "delegation1 should exist")
	assert.NotNil(t, del2, "delegation2 should exist")
}

func TestHousekeepAndOperations(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	// Add some delegations
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Transaction 2: Add another delegation after housekeep
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)
	assert.NotNil(t, delegation3, "should be able to add delegation after housekeep")

	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+20)

	// Verify final states
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	// Delegation1 should be signaled for exit
	assert.NotNil(t, del1.LastIteration(), "delegation1 should be signaled for exit")

	// Delegation2 should remain active
	assert.Nil(t, del2.LastIteration(), "delegation2 should remain active")

	// Delegation3 should be active
	assert.Nil(t, del3.LastIteration(), "delegation3 should be active")
}

func TestStakeOperations(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, val := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	// Add delegations first
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)

	// Wait for delegations to start
	staker.Housekeep(thor.MediumStakingPeriod())

	// Transaction 1: Increase validator stake
	staker.IncreaseStake(validator, val.Endorser(), 500)

	// Transaction 2: Decrease validator stake
	staker.DecreaseStake(validator, val.Endorser(), 200)

	// Transaction 3: Signal delegation1 exit
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+20)

	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Transaction 4: Withdraw delegation2
	staker.WithdrawDelegation(delegation1, uint64(1000), thor.MediumStakingPeriod()*2+20)

	// Verify final states
	valFinal := staker.GetValidation(validator)
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)

	assert.Greater(t, valFinal.LockedVET(), val.LockedVET(), "validator should have net stake increase")

	// Delegation1 should be signaled for exit
	assert.NotNil(t, del1.LastIteration(), "delegation1 should be signaled for exit")

	// Delegation1 should be withdrawn
	assert.Equal(t, uint64(0), del1.Stake(), "delegation1 should be withdrawn")

	// Delegation2 should be signaled for exit
	assert.Nil(t, del2.LastIteration(), "delegation2 should be active")
}

func TestValidatorExits(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	// Get multiple validators
	leaders, err := staker.LeaderGroup()
	require.NoError(t, err, "should be able to get leader group")
	require.Len(t, leaders, 3, "should have 3 active validators")

	validator1 := leaders[0].Address
	validator2 := leaders[1].Address
	validator3 := leaders[2].Address

	// Add delegations to all validators
	delegations := make([]*big.Int, 9) // 3 delegations per validator
	delegationAmounts := make([]*uint64, 9)
	for i, validator := range []thor.Address{validator1, validator2, validator3} {
		for j := range 3 {
			amount := uint64(1000 + i*100 + j*50)
			multiplier := uint8(100 + i*10 + j*5)
			delegationAmounts[i*3+j] = &amount
			delegations[i*3+j] = staker.AddDelegation(validator, amount, multiplier, 10)
		}
	}

	// Wait for delegations to start
	staker.Housekeep(thor.MediumStakingPeriod())

	// Transaction 1: Signal validator1 exit
	val1 := staker.GetValidation(validator1)
	staker.SignalExit(validator1, val1.Endorser(), thor.MediumStakingPeriod()+20)

	// Transaction 2: Signal validator2 exit
	val2 := staker.GetValidation(validator2)
	staker.SignalExit(validator2, val2.Endorser(), thor.MediumStakingPeriod()+20)

	// Transaction 3: Signal exit for some delegations
	for i := range 3 {
		staker.SignalDelegationExit(delegations[i], thor.MediumStakingPeriod()+20)
	}

	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Transaction 4: Withdraw some delegations
	for i := range 3 {
		staker.WithdrawDelegation(delegations[i], *delegationAmounts[i], thor.MediumStakingPeriod()*2+20)
	}

	// Transaction 5: Add new delegation to validator3
	delegationID := staker.AddDelegation(validator3, 1500, 180, thor.MediumStakingPeriod()*2+10)
	assert.NotNil(t, delegationID, "should be able to add delegation to validator3")

	// Transaction 6: Perform housekeep
	staker.Housekeep(thor.MediumStakingPeriod() * 3)

	// Verify final states
	val1Final := staker.GetValidation(validator1)
	val2Final := staker.GetValidation(validator2)
	val3Final := staker.GetValidation(validator3)

	// Validators 1 and 2 should have exit blocks set
	assert.NotNil(t, val1Final.ExitBlock(), "validator1 should have exit block set")
	assert.NotNil(t, val2Final.ExitBlock(), "validator2 should have exit block set")

	// Validator3 should still be active
	assert.Equal(t, validation.StatusActive, val3Final.Status(), "validator3 should still be active")
}

func TestExitedValidatorNegativeCases(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, val := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	// Add initial delegation
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)

	// Wait for delegation to start
	staker.Housekeep(thor.MediumStakingPeriod())

	// Transaction 1: Signal exit
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+20)

	staker.WithdrawDelegationErrors(delegation1, thor.MediumStakingPeriod()+20, "delegation is not eligible for withdraw")

	// Transaction 3: Signal validator exit
	staker.SignalExit(validator, val.Endorser(), thor.MediumStakingPeriod()+20)

	// Transaction 4: Try to add delegation (should fail because validator is exiting)
	staker.AddDelegationErrors(validator, 500, 100, 10, "cannot add delegation to exiting validator")

	// Transaction 5: Try to increase stake (should fail because validator is exiting)
	staker.IncreaseStakeErrors(validator, val.Endorser(), 300, "validator has signaled exit, cannot increase stake")
}

func TestSequentialStressTest(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	leaders, err := staker.LeaderGroup()
	require.NoError(t, err, "should be able to get leader group")
	require.Len(t, leaders, 3, "should have 3 active validators")

	// Stress test: many sequential operations
	var delegationIDs []*big.Int
	for i := range 20 {
		validator := leaders[i%3].Address
		amount := uint64(100 + i*10)
		multiplier := uint8(100 + i%50)

		delegationID := staker.AddDelegation(validator, amount, multiplier, 10)
		assert.NotNil(t, delegationID, "should be able to add delegation %d", i)
		delegationIDs = append(delegationIDs, delegationID)
	}

	// Wait for delegations to start
	staker.Housekeep(thor.MediumStakingPeriod())

	// Perform various operations
	for i := range 10 {
		validator := leaders[i%3].Address
		val := staker.GetValidation(validator)

		if i%2 == 0 {
			// Increase stake
			staker.IncreaseStake(validator, val.Endorser(), 100)
		} else {
			staker.DecreaseStake(validator, val.Endorser(), 50)
		}
	}

	// Perform housekeep operations
	for i := range 5 {
		staker.Housekeep(thor.MediumStakingPeriod() + uint32(i)*thor.EpochLength())
	}

	// Verify some delegations were created
	assert.Len(t, delegationIDs, 20, "should have created 20 delegations")

	// Verify delegations exist
	for i, delegationID := range delegationIDs {
		delegation := staker.GetDelegation(delegationID)
		assert.NotNil(t, delegation, "delegation %d should exist", i)
	}
}

func TestTimingDependencies(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	// Add delegation
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)

	// Transaction 1: Try to signal delegation exit before housekeep (should fail)
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+20)

	// Transaction 2: Try to withdraw before delegation starts (should fail)
	staker.WithdrawDelegationErrors(delegation1, thor.MediumStakingPeriod()+20, "delegation is not eligible for withdraw")

	// Transaction 3: Perform housekeep to start delegation
	staker.Housekeep(thor.MediumStakingPeriod())

	staker.SignalDelegationExitErrors(delegation1, thor.MediumStakingPeriod()+20, "delegation is already signaled exit")

	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Transaction 5: Now withdraw delegation (should succeed)
	staker.WithdrawDelegation(delegation1, 1000, thor.MediumStakingPeriod()*2+20)
}

func TestSequentialComplexScenario(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	// Get all active validators
	leaders, err := staker.LeaderGroup()
	require.NoError(t, err, "should be able to get leader group")
	require.Len(t, leaders, 3, "should have 3 active validators")

	validator1 := leaders[0].Address
	validator2 := leaders[1].Address
	validator3 := leaders[2].Address

	// Add delegations to different validators
	del1v1 := staker.AddDelegation(validator1, 1000, 150, 10)
	del2v1 := staker.AddDelegation(validator1, 2000, 200, 10)

	del1v2 := staker.AddDelegation(validator2, 1500, 180, 10)
	del2v2 := staker.AddDelegation(validator2, 500, 100, 10)

	del1v3 := staker.AddDelegation(validator3, 3000, 250, 10)

	// Wait for delegations to start
	staker.Housekeep(thor.MediumStakingPeriod())

	// Transaction 1: Signal exit for validator1
	val1 := staker.GetValidation(validator1)
	staker.SignalExit(validator1, val1.Endorser(), thor.MediumStakingPeriod()+20)

	// Transaction 2: Signal exit for some delegations
	staker.SignalDelegationExit(del2v1, thor.MediumStakingPeriod()+20)

	staker.SignalDelegationExit(del1v2, thor.MediumStakingPeriod()+20)

	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Transaction 3: Withdraw some delegations
	staker.WithdrawDelegation(del1v2, 1500, thor.MediumStakingPeriod()*2+20)

	// Transaction 4: Perform housekeep
	staker.Housekeep(thor.MediumStakingPeriod() * 3)

	// Verify final states
	hasDel1, err := staker.HasDelegations(validator1)
	assert.NoError(t, err)
	assert.False(t, hasDel1, "validator1 has exited")

	hasDel2, err := staker.HasDelegations(validator2)
	assert.NoError(t, err)
	assert.True(t, hasDel2, "validator2 should have delegations")

	hasDel3, err := staker.HasDelegations(validator3)
	assert.NoError(t, err)
	assert.True(t, hasDel3, "validator3 should have delegations")

	// Verify specific delegation states
	del1v1Final := staker.GetDelegation(del1v1)
	del2v1Final := staker.GetDelegation(del2v1)
	del1v2Final := staker.GetDelegation(del1v2)
	del2v2Final := staker.GetDelegation(del2v2)
	del1v3Final := staker.GetDelegation(del1v3)

	// Validator1 delegations
	assert.Nil(t, del1v1Final.LastIteration(), "del1v1 should not be signaled for exit")
	assert.NotNil(t, del2v1Final.LastIteration(), "del2v1 should be signaled for exit")

	// Validator2 delegations
	assert.NotNil(t, del1v2Final.LastIteration(), "del1v2 should be signaled for exit")
	assert.Equal(t, uint64(0), del1v2Final.Stake(), "del1v2 should be withdrawn")
	assert.Nil(t, del2v2Final.LastIteration(), "del2v2 should not be signaled for exit")

	// Validator3 delegations
	assert.Nil(t, del1v3Final.LastIteration(), "del1v3 should not be signaled for exit")
	assert.Equal(t, uint64(3000), del1v3Final.Stake(), "del1v3 should retain stake")
}
