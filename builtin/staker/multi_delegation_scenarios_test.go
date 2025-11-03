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

	"github.com/vechain/thor/v2/thor"
)

// TestMultiDelegationToSingleValidator tests multiple delegations to the same validator
func TestMultiDelegationToSingleValidator(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotEqual(t, thor.Address{}, validator, "validator should be active")

	// Add multiple delegations to the same validator
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)

	// Verify all delegations exist
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	assert.NotNil(t, del1, "delegation 1 should exist")
	assert.NotNil(t, del2, "delegation 2 should exist")
	assert.NotNil(t, del3, "delegation 3 should exist")

	// Verify delegation details
	assert.Equal(t, validator, del1.Validation, "delegation 1 should point to correct validator")
	assert.Equal(t, validator, del2.Validation, "delegation 2 should point to correct validator")
	assert.Equal(t, validator, del3.Validation, "delegation 3 should point to correct validator")

	assert.Equal(t, uint64(1000), del1.Stake, "delegation 1 stake should be correct")
	assert.Equal(t, uint64(2000), del2.Stake, "delegation 2 stake should be correct")
	assert.Equal(t, uint64(500), del3.Stake, "delegation 3 stake should be correct")

	assert.Equal(t, uint8(150), del1.Multiplier, "delegation 1 multiplier should be correct")
	assert.Equal(t, uint8(200), del2.Multiplier, "delegation 2 multiplier should be correct")
	assert.Equal(t, uint8(100), del3.Multiplier, "delegation 3 multiplier should be correct")

	staker.Housekeep(thor.MediumStakingPeriod() * thor.EpochLength())

	// Verify validator has delegations
	hasDelegations, err := staker.HasDelegations(validator)
	assert.NoError(t, err, "should be able to check delegations")
	assert.True(t, hasDelegations, "validator should have delegations")
}

// TestDelegationCascadeExit tests cascading exits of multiple delegations
func TestDelegationCascadeExit(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")

	// Add multiple delegations
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Signal exits for all delegations
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+20)
	staker.SignalDelegationExit(delegation2, thor.MediumStakingPeriod()+20)
	staker.SignalDelegationExit(delegation3, thor.MediumStakingPeriod()+20)

	// Verify all delegations are signaled for exit
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	assert.NotNil(t, del1.LastIteration, "delegation 1 should be signaled for exit")
	assert.NotNil(t, del2.LastIteration, "delegation 2 should be signaled for exit")
	assert.NotNil(t, del3.LastIteration, "delegation 3 should be signaled for exit")

	// Verify they can't be signaled again
	staker.SignalDelegationExitErrors(delegation1, thor.MediumStakingPeriod()+21, "delegation is already signaled exit")
	staker.SignalDelegationExitErrors(delegation2, thor.MediumStakingPeriod()+21, "delegation is already signaled exit")
	staker.SignalDelegationExitErrors(delegation3, thor.MediumStakingPeriod()+21, "delegation is already signaled exit")
}

// TestDelegationPartialWithdrawal tests partial withdrawal scenarios
func TestDelegationPartialWithdrawal(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")

	// Add multiple delegations
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)

	// Withdraw only some delegations
	staker.WithdrawDelegation(delegation1, 1000, 30)
	staker.WithdrawDelegation(delegation3, 500, 30)

	// Verify withdrawal results
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	assert.Equal(t, uint64(0), del1.Stake, "delegation 1 should be fully withdrawn")
	assert.Equal(t, uint64(2000), del2.Stake, "delegation 2 should remain unchanged")
	assert.Equal(t, uint64(0), del3.Stake, "delegation 3 should be fully withdrawn")

	staker.Housekeep(thor.MediumStakingPeriod())

	// Verify validator still has delegations (delegation2)
	hasDelegations, err := staker.HasDelegations(validator)
	assert.NoError(t, err, "should be able to check delegations")
	assert.True(t, hasDelegations, "validator should still have delegations")
}

// TestDelegationMixedStates tests delegations in different states
func TestDelegationMixedStates(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")

	// Add delegations at different times
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10) // Started
	delegation2 := staker.AddDelegation(validator, 2000, 200, 20) // Started
	delegation3 := staker.AddDelegation(validator, 500, 100, 30)  // Started

	// Withdraw delegation2
	staker.WithdrawDelegation(delegation2, 2000, 50)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Signal exit for delegation1 only
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+40)

	// Verify mixed states
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	// Delegation1: signaled for exit
	assert.NotNil(t, del1.LastIteration, "delegation 1 should be signaled for exit")
	assert.Equal(t, uint64(1000), del1.Stake, "delegation 1 should retain stake")

	// Delegation2: withdrawn
	assert.Equal(t, uint64(0), del2.Stake, "delegation 2 should be withdrawn")

	// Delegation3: active
	assert.Nil(t, del3.LastIteration, "delegation 3 should not be signaled for exit")
	assert.Equal(t, uint64(500), del3.Stake, "delegation 3 should retain stake")

	// Verify validator still has delegations (delegation1 and delegation3)
	hasDelegations, err := staker.HasDelegations(validator)
	assert.NoError(t, err, "should be able to check delegations")
	assert.True(t, hasDelegations, "validator should still have delegations")
}

// TestDelegationConcurrentOperations tests concurrent operations on multiple delegations
func TestDelegationConcurrentOperations(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")

	// Add multiple delegations
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Withdraw delegation2
	staker.SignalDelegationExit(delegation2, thor.MediumStakingPeriod()+20)

	staker.Housekeep(thor.MediumStakingPeriod() * 2)
	// Perform different operations on different delegations
	// Signal exit for delegation1
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()*2+20)
	staker.WithdrawDelegation(delegation2, 2000, thor.MediumStakingPeriod()*2+20)

	// Leave delegation3 active

	// Verify final states
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	// Delegation1: signaled for exit
	assert.NotNil(t, del1.LastIteration, "delegation 1 should be signaled for exit")
	assert.Equal(t, uint64(1000), del1.Stake, "delegation 1 should retain stake")

	// Delegation2: withdrawn
	assert.Equal(t, uint64(0), del2.Stake, "delegation 2 should be withdrawn")

	// Delegation3: active
	assert.Nil(t, del3.LastIteration, "delegation 3 should not be signaled for exit")
	assert.Equal(t, uint64(500), del3.Stake, "delegation 3 should retain stake")
}

// TestDelegationValidatorExit tests behavior when validator exits while having multiple delegations
func TestDelegationValidatorExit(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, val := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")

	// Add multiple delegations
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10)
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Signal validator exit
	staker.SignalExit(validator, val.Endorser(), thor.MediumStakingPeriod()+20)

	// Verify delegations are still accessible
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	assert.NotNil(t, del1, "delegation 1 should still exist")
	assert.NotNil(t, del2, "delegation 2 should still exist")
	assert.NotNil(t, del3, "delegation 3 should still exist")

	// Verify validator still has delegations
	hasDelegations, err := staker.HasDelegations(validator)
	assert.NoError(t, err, "should be able to check delegations")
	assert.True(t, hasDelegations, "validator should still have delegations")

	// Verify delegations can still be signaled for exit
	staker.SignalDelegationExit(delegation1, thor.MediumStakingPeriod()+30)
	staker.SignalDelegationExit(delegation2, thor.MediumStakingPeriod()+30)
	staker.SignalDelegationExit(delegation3, thor.MediumStakingPeriod()+30)

	// Verify final states
	del1 = staker.GetDelegation(delegation1)
	del2 = staker.GetDelegation(delegation2)
	del3 = staker.GetDelegation(delegation3)

	assert.NotNil(t, del1.LastIteration, "delegation 1 should be signaled for exit")
	assert.NotNil(t, del2.LastIteration, "delegation 2 should be signaled for exit")
	assert.NotNil(t, del3.LastIteration, "delegation 3 should be signaled for exit")
}

// TestDelegationValidatorReplacement tests behavior when validator is replaced
func TestDelegationValidatorReplacement(t *testing.T) {
	staker := newTest(t).SetMBP(2).Fill(2).Transition(0)

	// Get first validator
	validator1, val1 := staker.FirstActive()
	require.NotNil(t, validator1, "validator1 should be active")

	// Add delegations to first validator
	delegation1 := staker.AddDelegation(validator1, 1000, 150, 10)
	delegation2 := staker.AddDelegation(validator1, 2000, 200, 10)

	// Signal exit for first validator
	staker.SignalExit(validator1, val1.Endorser(), 20)

	// Housekeep to complete validator exit
	staker.Housekeep(val1.Period())

	// Verify delegations still exist but validator is no longer active
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)

	assert.NotNil(t, del1, "delegation 1 should still exist")
	assert.NotNil(t, del2, "delegation 2 should still exist")

	// Verify validator no longer has delegations (since it's exited)
	hasDelegations, err := staker.HasDelegations(validator1)
	assert.NoError(t, err, "should be able to check delegations")
	assert.False(t, hasDelegations, "exited validator should not have delegations")

	// Verify delegations can still be withdrawn
	staker.WithdrawDelegation(delegation1, 1000, 30)
	staker.WithdrawDelegation(delegation2, 2000, 30)
}

// TestDelegationStakeAggregation tests stake aggregation across multiple delegations
func TestDelegationStakeAggregation(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")
	valBefore := staker.GetValidation(validator)

	// Add multiple delegations with different multipliers
	delegation1 := staker.AddDelegation(validator, 1000, 150, 10) // Weight: 1000 * 150 = 150,000
	delegation2 := staker.AddDelegation(validator, 2000, 200, 10) // Weight: 2000 * 200 = 400,000
	delegation3 := staker.AddDelegation(validator, 500, 100, 10)  // Weight: 500 * 100 = 50,000

	// Verify individual delegation weights
	del1 := staker.GetDelegation(delegation1)
	del2 := staker.GetDelegation(delegation2)
	del3 := staker.GetDelegation(delegation3)

	expectedWeight1 := uint64(1000*150) / 100
	expectedWeight2 := uint64(2000*200) / 100
	expectedWeight3 := uint64(500*100) / 100

	assert.Equal(t, expectedWeight1, del1.WeightedStake().Weight, "delegation 1 weight should be correct")
	assert.Equal(t, expectedWeight2, del2.WeightedStake().Weight, "delegation 2 weight should be correct")
	assert.Equal(t, expectedWeight3, del3.WeightedStake().Weight, "delegation 3 weight should be correct")

	staker.Housekeep(thor.MediumStakingPeriod())

	// Verify total aggregated stake
	val := staker.GetValidation(validator)
	assert.NotNil(t, val, "validation should exist")

	// The total locked weight should include validator's own stake plus all delegations
	expectedTotalWeight := valBefore.LockedVET()*2 + 1500 + 4000 + 500
	assert.Equal(t, valBefore.LockedVET(), val.LockedVET(), "total locked stake should include all delegations")
	assert.Equal(t, expectedTotalWeight, val.Weight(), "total locked weight should include all delegations")
}

// TestDelegationEdgeCases tests edge cases for multiple delegations
func TestDelegationEdgeCases(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	validator, _ := staker.FirstActive()
	require.NotNil(t, validator, "validator should be active")

	// Test adding delegation with zero amount (should fail)
	staker.AddDelegationErrors(validator, 0, 100, 10, "stake must be greater than 0")

	// Test adding delegation with zero multiplier (should fail)
	staker.AddDelegationErrors(validator, 1000, 0, 10, "multiplier cannot be 0")

	// Test adding delegation to non-existent validator (should fail)
	nonExistentValidator := thor.BytesToAddress([]byte("nonexistent"))
	staker.AddDelegationErrors(nonExistentValidator, 1000, 100, 10, "validation does not exist")

	// Test signaling exit for non-existent delegation (should fail)
	staker.SignalDelegationExitErrors(big.NewInt(99999), 10, "delegation is empty")

	// Test withdrawing non-existent delegation (should fail)
	_, err := staker.Staker.WithdrawDelegation(big.NewInt(99999), 10)
	assert.Error(t, err, "should fail to withdraw non-existent delegation")
	assert.Contains(t, err.Error(), "delegation is empty", "error should indicate delegation is empty")
}

// TestDelegationComplexScenario tests a complex scenario with multiple validators and delegations
func TestDelegationComplexScenario(t *testing.T) {
	staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

	// Get all active validators
	validator1, val1 := staker.FirstActive()

	// Get leader group to access all validators
	leaders, err := staker.LeaderGroup()
	require.NoError(t, err, "should be able to get leader group")
	require.Len(t, leaders, 3, "should have 3 active validators")

	validator2 := leaders[1].Address
	validator3 := leaders[2].Address

	require.NotNil(t, validator1, "validator1 should be active")
	require.NotZero(t, validator2, "validator2 should be active")
	require.NotZero(t, validator3, "validator3 should be active")

	// Add delegations to different validators
	del1v1 := staker.AddDelegation(validator1, 1000, 150, 10)
	del2v1 := staker.AddDelegation(validator1, 2000, 200, 10)

	del1v2 := staker.AddDelegation(validator2, 1500, 180, 10)
	del2v2 := staker.AddDelegation(validator2, 500, 100, 10)

	del1v3 := staker.AddDelegation(validator3, 3000, 250, 10)

	staker.Housekeep(thor.MediumStakingPeriod())

	// Signal exit for validator1
	staker.SignalExit(validator1, val1.Endorser(), thor.MediumStakingPeriod()+20)

	// Signal exit for some delegations
	staker.SignalDelegationExit(del2v1, thor.MediumStakingPeriod()+20) // Exit from validator1
	staker.SignalDelegationExit(del1v2, thor.MediumStakingPeriod()+20) // Exit from validator2

	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Withdraw some delegations
	staker.WithdrawDelegation(del1v2, 1500, thor.MediumStakingPeriod()*2+20) // Withdraw from validator2

	// Verify final states
	hasDel1, err := staker.HasDelegations(validator1)
	assert.NoError(t, err)
	assert.False(t, hasDel1, "validator1 has exited")

	hasDel2, err := staker.HasDelegations(validator2)
	assert.NoError(t, err)
	assert.True(t, hasDel2, "validator2 should have delegations")

	// Validator3: 1 active delegation
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
	assert.Nil(t, del1v1Final.LastIteration)
	assert.NotNil(t, del2v1Final.LastIteration)

	// Validator2 delegations
	assert.NotNil(t, del1v2Final.LastIteration)
	assert.Nil(t, del2v2Final.LastIteration)

	// Validator3 delegations
	assert.Nil(t, del1v3Final.LastIteration)
	assert.Equal(t, uint64(3000), del1v3Final.Stake)
}
