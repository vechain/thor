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

// TestValidationErrorRecovery tests error recovery in validation operations
func TestValidationErrorRecovery(t *testing.T) {
	t.Run("AddValidation Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(3)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test recovery from insufficient stake error
		staker.AddValidationErrors(validator, endorser, thor.LowStakingPeriod(), MinStakeVET-1, "stake is below minimum")

		// Test recovery by adding with correct stake
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		// Verify validator was added successfully
		val := staker.GetValidation(validator)
		assert.NotNil(t, val, "validator should exist after recovery")
		assert.Equal(t, validation.StatusQueued, val.Status, "validator should be queued")
	})

	t.Run("AddValidation Duplicate Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(3)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Add validator first time
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		// Try to add same validator again
		staker.AddValidationErrors(validator, endorser, thor.LowStakingPeriod(), MinStakeVET, "validator already exists")

		// Verify original validator is still intact
		val := staker.GetValidation(validator)
		assert.NotNil(t, val, "original validator should still exist")
		assert.Equal(t, validation.StatusQueued, val.Status, "original validator should be queued")
	})

	t.Run("AddValidation Invalid Period Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(3)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with invalid period
		staker.AddValidationErrors(validator, endorser, 999, MinStakeVET, "period is out of boundaries")

		// Test recovery with valid period
		staker.Staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		val := staker.GetValidation(validator)
		assert.NotNil(t, val, "validator should exist after recovery")
	})

	t.Run("AddValidation Zero Address Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(3)

		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with zero validator address
		staker.AddValidationErrors(thor.Address{}, endorser, thor.LowStakingPeriod(), MinStakeVET, "validator cannot be zero")

		// Test recovery with valid address
		validator := thor.BytesToAddress([]byte("validator"))
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)
	})
}

// TestDelegationErrorRecovery tests error recovery in delegation operations
func TestDelegationErrorRecovery(t *testing.T) {
	t.Run("AddDelegation Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test recovery from zero stake error
		staker.AddDelegationErrors(validator, 0, 150, 10, "stake must be greater than 0")

		// Test recovery with valid stake
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		assert.NotNil(t, delegationID, "delegation ID should be returned")

		// Verify delegation was added
		del := staker.GetDelegation(delegationID)
		assert.NotNil(t, del, "delegation should exist after recovery")
	})

	t.Run("AddDelegation Zero Multiplier Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test with zero multiplier
		_ = staker.AddDelegationErrors(validator, 1000, 0, 10, "multiplier cannot be 0")

		// Test recovery with valid multiplier
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		assert.NotNil(t, delegationID, "delegation ID should be returned")
	})

	t.Run("AddDelegation Invalid Validator Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		// Test with non-existent validator
		nonExistentValidator := thor.BytesToAddress([]byte("nonexistent"))
		_ = staker.AddDelegationErrors(nonExistentValidator, 1000, 150, 10, "validation does not exist")

		// Test recovery with valid validator
		validator, _ := staker.FirstActive()
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		assert.NotNil(t, delegationID, "delegation ID should be returned")
	})

	t.Run("AddDelegation Exiting Validator Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Signal validator exit
		err := staker.Staker.SignalExit(validator, val.Endorser, thor.EpochLength())
		assert.NoError(t, err, "should be able to signal exit")

		// Try to add delegation to exiting validator
		_, err = staker.Staker.AddDelegation(validator, 1000, 150, 10)
		assert.Error(t, err, "should fail with exiting validator")
		assert.Contains(t, err.Error(), "cannot add delegation to exiting validator", "error should indicate exiting validator")

		// Verify validator is still in correct state
		valAfter := staker.GetValidation(validator)
		assert.NotNil(t, valAfter.ExitBlock, "exit block should be set")
		assert.Equal(t, validation.StatusActive, valAfter.Status, "validator should still be active")
	})
}

// TestStakeOperationErrorRecovery tests error recovery in stake operations
func TestStakeOperationErrorRecovery(t *testing.T) {
	t.Run("IncreaseStake Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test with wrong endorser
		wrongEndorser := thor.BytesToAddress([]byte("wrong"))
		staker.IncreaseStakeErrors(validator, wrongEndorser, 500, "endorser required")

		// Test recovery with correct endorser
		staker.IncreaseStake(validator, val.Endorser, 500)

		// Verify stake was increased
		valAfter := staker.GetValidation(validator)
		assert.Equal(t, valAfter.LockedVET, val.LockedVET, "locked VET should remain the same")
		assert.Greater(t, valAfter.QueuedVET, uint64(0), "queued VET should increase")
	})

	t.Run("DecreaseStake Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test decreasing below minimum
		staker.DecreaseStakeErrors(validator, val.Endorser, val.LockedVET, "next period stake is lower than minimum stake")

		// Test recovery with valid decrease
		staker.DecreaseStake(validator, val.Endorser, 100)

		// Verify stake was decreased
		valAfter := staker.GetValidation(validator)
		assert.Equal(t, valAfter.LockedVET, val.LockedVET, "locked VET should remain the same")
		assert.Greater(t, valAfter.PendingUnlockVET, uint64(0), "pending unlock VET should increase")
	})
}

// TestExitOperationErrorRecovery tests error recovery in exit operations
func TestExitOperationErrorRecovery(t *testing.T) {
	t.Run("SignalExit Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test with wrong endorser
		wrongEndorser := thor.BytesToAddress([]byte("wrong"))
		err := staker.Staker.SignalExit(validator, wrongEndorser, thor.EpochLength())
		assert.Error(t, err, "should fail with wrong endorser")
		assert.Contains(t, err.Error(), "endorser required", "error should indicate wrong endorser")

		// Test recovery with correct endorser
		err = staker.Staker.SignalExit(validator, val.Endorser, thor.EpochLength())
		assert.NoError(t, err, "should succeed with correct endorser")

		// Verify exit was signaled
		valAfter := staker.GetValidation(validator)
		assert.NotNil(t, valAfter.ExitBlock, "exit block should be set")
	})

	t.Run("SignalExit Double Signal Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Signal exit first time
		staker.SignalExit(validator, val.Endorser, thor.EpochLength())

		// Try to signal exit again
		staker.SignalExitErrors(validator, val.Endorser, thor.EpochLength()+thor.EpochLength(), "exit block already set to 129600")

		// Verify original exit is still intact
		valAfter := staker.GetValidation(validator)
		assert.NotNil(t, valAfter.ExitBlock, "original exit block should still be set")
	})

	t.Run("SignalExit Non-Active Validator Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Add validator but don't activate
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		// Try to signal exit on queued validator
		err := staker.Staker.SignalExit(validator, endorser, thor.EpochLength())
		assert.Error(t, err, "should fail on queued validator")
		assert.Contains(t, err.Error(), "can't signal exit while not active", "error should indicate not active")

		// Activate validator
		staker.Housekeep(thor.EpochLength())

		// Test recovery after activation
		err = staker.Staker.SignalExit(validator, endorser, thor.EpochLength()*2)
		assert.NoError(t, err, "should succeed after activation")
	})
}

// TestDelegationExitErrorRecovery tests error recovery in delegation exit operations
func TestDelegationExitErrorRecovery(t *testing.T) {
	t.Run("SignalDelegationExit Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		// Test with non-existent delegation
		nonExistentID := big.NewInt(99999)
		err := staker.Staker.SignalDelegationExit(nonExistentID, 10)
		assert.Error(t, err, "should fail with non-existent delegation")
		assert.Contains(t, err.Error(), "delegation is empty", "error should indicate empty delegation")

		// Test recovery with valid delegation
		err = staker.Staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)
		assert.NoError(t, err, "should succeed with valid delegation")

		// Verify exit was signaled
		del := staker.GetDelegation(delegationID)
		assert.NotNil(t, del.LastIteration, "last iteration should be set")
	})

	t.Run("SignalDelegationExit Before Started Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation but don't start it
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)

		// Try to signal exit before started
		err := staker.Staker.SignalDelegationExit(delegationID, 10)
		assert.Error(t, err, "should fail before delegation started")
		assert.Contains(t, err.Error(), "delegation has not started yet", "error should indicate not started")

		// Start delegation
		staker.Housekeep(thor.MediumStakingPeriod())

		// Test recovery after started
		err = staker.Staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)
		assert.NoError(t, err, "should succeed after started")
	})

	t.Run("SignalDelegationExit Double Signal Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add and start delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		// Signal exit first time
		err := staker.Staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)
		assert.NoError(t, err, "first signal should succeed")

		// Try to signal exit again
		err = staker.Staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+30)
		assert.Error(t, err, "should fail with double signal")
		assert.Contains(t, err.Error(), "delegation is already signaled exit", "error should indicate already signaled")

		// Verify original exit is still intact
		del := staker.GetDelegation(delegationID)
		assert.NotNil(t, del.LastIteration, "original last iteration should still be set")
	})
}

// TestWithdrawalErrorRecovery tests error recovery in withdrawal operations
func TestWithdrawalErrorRecovery(t *testing.T) {
	t.Run("WithdrawDelegation Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test with non-existent delegation
		nonExistentID := big.NewInt(99999)
		staker.WithdrawDelegationErrors(nonExistentID, 10, "delegation is empty")

		// Add and start delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())
		staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+1)
		staker.Housekeep(thor.MediumStakingPeriod() * 2)

		// Test recovery with valid delegation
		staker.WithdrawDelegation(delegationID, 1000, thor.MediumStakingPeriod()*2+10)
	})

	t.Run("WithdrawDelegation After Exit Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add, start, and exit delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())
		staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)

		// Try to withdraw after exit
		_ = staker.WithdrawDelegationErrors(delegationID, thor.MediumStakingPeriod()+30, "delegation is not eligible for withdraw")

		// Verify delegation is in correct state
		del := staker.GetDelegation(delegationID)
		assert.NotNil(t, del.LastIteration, "last iteration should be set")
	})
}

// TestBalanceErrorRecovery tests error recovery in balance-related operations
func TestBalanceErrorRecovery(t *testing.T) {
	t.Run("Contract Balance Check Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		// Test with insufficient contract balance
		originalBalance, err := staker.State().GetBalance(staker.Address())
		require.NoError(t, err, "should be able to get balance")

		// Reduce contract balance to cause error
		reducedBalance := big.NewInt(0).Div(originalBalance, big.NewInt(2))
		err = staker.State().SetBalance(staker.Address(), reducedBalance)
		require.NoError(t, err, "should be able to set reduced balance")

		// Test that balance check fails
		err = staker.ContractBalanceCheck(0)
		assert.Error(t, err, "should fail with insufficient balance")
		assert.Contains(t, err.Error(), "balance check failed", "error should indicate balance check failure")

		// Test recovery by restoring balance
		err = staker.State().SetBalance(staker.Address(), originalBalance)
		require.NoError(t, err, "should be able to restore balance")

		// Test that balance check succeeds after recovery
		err = staker.ContractBalanceCheck(0)
		assert.NoError(t, err, "should succeed after balance recovery")
	})

	t.Run("Stake Validation Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test increasing stake beyond maximum
		staker.IncreaseStakeErrors(validator, val.Endorser, MaxStakeVET, "total stake would exceed maximum")

		// Test recovery with valid increase
		staker.IncreaseStake(validator, val.Endorser, 1000)
	})
}

// TestStateConsistencyErrorRecovery tests error recovery in state consistency
func TestStateConsistencyErrorRecovery(t *testing.T) {
	t.Run("Housekeep Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		// Test housekeep at non-epoch boundary
		_, housekeepResult := staker.HousekeepWithUpdates(thor.EpochLength() + 1)
		assert.False(t, housekeepResult, "housekeep should not have updates at non-epoch boundary")

		validator, val := staker.FirstActive()
		staker.SignalExit(validator, val.Endorser, thor.EpochLength()+1)

		// Test recovery at epoch boundary
		_, housekeepResult = staker.HousekeepWithUpdates(thor.MediumStakingPeriod())
		assert.True(t, housekeepResult, "housekeep should have updates at epoch boundary")
	})

	t.Run("Global Stats Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		// Test that global stats are consistent
		lockedStake, _ := staker.LockedStake()
		assert.Greater(t, lockedStake, uint64(0), "locked stake should be positive")

		queuedStake := staker.QueuedStake()

		// Test that totals are consistent
		totalStake := lockedStake + queuedStake
		effectiveVET, err := staker.GetEffectiveVET()
		assert.NoError(t, err, "should be able to get effective VET")
		assert.Equal(t, effectiveVET, totalStake, "effective VET should equal total stake")
	})
}

// TestComplexErrorRecovery tests complex error recovery scenarios
func TestComplexErrorRecovery(t *testing.T) {
	t.Run("Cascading Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test multiple error conditions in sequence
		// 1. Try to add delegation with zero stake
		staker.AddDelegationErrors(validator, 0, 150, 10, "stake must be greater than 0")

		// 2. Try to add delegation with zero multiplier
		staker.AddDelegationErrors(validator, 1000, 0, 10, "multiplier cannot be 0")

		// 3. Try to increase stake with wrong endorser
		wrongEndorser := thor.BytesToAddress([]byte("wrong"))
		staker.IncreaseStakeErrors(validator, wrongEndorser, 500, "endorser required")

		// 4. Test recovery with all correct parameters
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)

		staker.IncreaseStake(validator, val.Endorser, 500)

		// Verify final state is correct
		valAfter := staker.GetValidation(validator)
		assert.Equal(t, valAfter.LockedVET, val.LockedVET, "stake should remain the same")
		assert.Greater(t, valAfter.QueuedVET, uint64(0), "queued stake should be increased")

		del := staker.GetDelegation(delegationID)
		assert.NotNil(t, del, "delegation should exist")
	})

	t.Run("State Corruption Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test that operations work correctly after errors
		// 1. Add delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)

		// 2. Try invalid operation
		_ = staker.AddDelegationErrors(validator, 0, 150, 10, "stake must be greater than 0")

		// 3. Verify state is still consistent
		del := staker.GetDelegation(delegationID)
		assert.NotNil(t, del, "original delegation should still exist")

		valAfter := staker.GetValidation(validator)
		assert.Equal(t, validation.StatusActive, valAfter.Status, "validator should still be active")

		// 4. Test that valid operations still work
		staker.IncreaseStake(validator, val.Endorser, 500)
	})
}

// TestBoundaryErrorRecovery tests error recovery at boundary conditions
func TestBoundaryErrorRecovery(t *testing.T) {
	t.Run("Minimum Stake Boundary Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with exactly minimum stake - 1
		staker.AddValidationErrors(validator, endorser, thor.LowStakingPeriod(), MinStakeVET-1, "stake is below minimum")

		// Test recovery with exactly minimum stake
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)
	})

	t.Run("Maximum Stake Boundary Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with exactly maximum stake + 1
		staker.AddValidationErrors(validator, endorser, thor.LowStakingPeriod(), MaxStakeVET+1, "stake is above maximum")

		// Test recovery with exactly maximum stake
		staker.Staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MaxStakeVET)
	})

	t.Run("Period Boundary Error Recovery", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with invalid period
		staker.AddValidationErrors(validator, endorser, 999, MinStakeVET, "period is out of boundaries")

		// Test recovery with valid periods
		validPeriods := []uint32{thor.LowStakingPeriod(), thor.MediumStakingPeriod(), thor.HighStakingPeriod()}
		for _, period := range validPeriods {
			validator := thor.BytesToAddress([]byte("validator" + string(rune(period))))
			staker.AddValidation(validator, endorser, period, MinStakeVET)
		}
	})
}
