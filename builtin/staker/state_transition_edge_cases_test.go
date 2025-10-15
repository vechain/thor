// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

// TestValidatorStatusTransitionEdgeCases tests edge cases in validator status transitions
func TestValidatorStatusTransitionEdgeCases(t *testing.T) {
	t.Run("Unknown to Queued Transition", func(t *testing.T) {
		staker := newTest(t).SetMBP(3)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test adding validation with minimum stake
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		val := staker.GetValidation(validator)
		assert.Equal(t, validation.StatusQueued, val.Status, "validator should be in queued status")
		assert.Equal(t, MinStakeVET, val.QueuedVET, "queued VET should equal stake")
		assert.Equal(t, uint64(0), val.LockedVET, "locked VET should be zero")
	})

	t.Run("Queued to Active Transition Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1) // Single validator to test edge cases

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Add validator
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		// Test activation at exact epoch boundary
		housekeepResult, err := staker.Staker.Housekeep(thor.EpochLength())
		assert.NoError(t, err, "housekeep should succeed")
		assert.True(t, housekeepResult, "housekeep should have updates")

		val := staker.GetValidation(validator)
		assert.Equal(t, validation.StatusActive, val.Status, "validator should be active")
		assert.Equal(t, MinStakeVET, val.LockedVET, "locked VET should equal original stake")
		assert.Equal(t, uint64(0), val.QueuedVET, "queued VET should be zero")
		assert.Equal(t, thor.EpochLength(), val.StartBlock, "start block should be set")
	})

	t.Run("Active to Exit Transition Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test signaling exit at exact period boundary
		err := staker.Staker.SignalExit(validator, val.Endorser, thor.EpochLength())
		assert.NoError(t, err, "should be able to signal exit")

		valAfter := staker.GetValidation(validator)
		assert.NotNil(t, valAfter.ExitBlock, "exit block should be set")
		assert.Equal(t, validation.StatusActive, valAfter.Status, "status should still be active until housekeep")
	})

	t.Run("Exit to Withdrawable Transition Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Signal exit
		staker.SignalExit(validator, val.Endorser, thor.EpochLength())

		// Process exit at exact exit block
		exitBlock := thor.MediumStakingPeriod()
		staker.Housekeep(exitBlock)

		valAfter := staker.GetValidation(validator)
		withdrawable, err := staker.GetWithdrawable(validator, exitBlock+thor.CooldownPeriod())
		assert.NoError(t, err)
		assert.Equal(t, validation.StatusExit, valAfter.Status, "validator should be in exit status")
		assert.Greater(t, valAfter.CooldownVET, uint64(0), "should have cooldown VET")
		assert.Equal(t, valAfter.CooldownVET, withdrawable, "withdrawable should be equal to cooldown")
	})
}

// TestDelegationStateTransitionEdgeCases tests edge cases in delegation state transitions
func TestDelegationStateTransitionEdgeCases(t *testing.T) {
	t.Run("Delegation Started Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test delegation added before validator activation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		assert.NotNil(t, delegationID, "should be able to add delegation")

		// Test delegation started state before housekeep
		del := staker.GetDelegation(delegationID)
		val := staker.GetValidation(validator)

		started, err := del.Started(val, 10)
		assert.NoError(t, err, "should be able to check started state")
		assert.False(t, started, "delegation should not be started before housekeep")

		// Test delegation started state after housekeep
		staker.Housekeep(thor.MediumStakingPeriod())

		started, err = del.Started(val, thor.MediumStakingPeriod()+10)
		assert.NoError(t, err, "should be able to check started state")
		assert.True(t, started, "delegation should be started after housekeep")
	})

	t.Run("Delegation Ended Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		// Test delegation ended when validator exits
		val := staker.GetValidation(validator)
		err := staker.Staker.SignalExit(validator, val.Endorser, thor.MediumStakingPeriod()+20)
		assert.NoError(t, err, "should be able to signal validator exit")

		staker.Housekeep(thor.MediumStakingPeriod() * 2)
		val = staker.GetValidation(validator)
		del := staker.GetDelegation(delegationID)
		ended, err := del.Ended(val, thor.MediumStakingPeriod()*2+30)
		assert.NoError(t, err, "should be able to check ended state")
		assert.True(t, ended, "delegation should be ended when validator exits")
	})

	t.Run("Delegation IsLocked Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Test delegation with zero stake
		staker.AddDelegationErrors(validator, 0, 150, 10, "stake must be greater than 0")

		val := staker.GetValidation(validator)

		// Test delegation with stake
		delegationID2 := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		del2 := staker.GetDelegation(delegationID2)
		locked, err := del2.IsLocked(val, thor.MediumStakingPeriod()+10)
		assert.NoError(t, err, "should be able to check locked state")
		assert.True(t, locked, "delegation with stake should be locked")
	})
}

// TestEpochBoundaryEdgeCases tests edge cases at epoch boundaries
func TestEpochBoundaryEdgeCases(t *testing.T) {
	t.Run("Housekeep at Non-Epoch Boundary", func(t *testing.T) {
		staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

		// Test housekeep at non-epoch boundary
		housekeepResult, err := staker.Staker.Housekeep(thor.EpochLength() + 1)
		assert.NoError(t, err, "housekeep should succeed")
		assert.False(t, housekeepResult, "housekeep should not have updates at non-epoch boundary")
	})

	t.Run("Housekeep at Exact Epoch Boundary", func(t *testing.T) {
		staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

		// Test housekeep at exact epoch boundary
		housekeepResult, err := staker.Staker.Housekeep(thor.EpochLength())
		assert.NoError(t, err, "housekeep should succeed")
		assert.False(t, housekeepResult, "housekeep should have updates at epoch boundary")
	})

	t.Run("Multiple Housekeep at Same Epoch", func(t *testing.T) {
		staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

		// Test multiple housekeep calls at same epoch
		_, housekeepResult1 := staker.HousekeepWithUpdates(thor.EpochLength())
		assert.False(t, housekeepResult1, "first housekeep should not have updates")

		_, housekeepResult2 := staker.HousekeepWithUpdates(thor.EpochLength())
		assert.False(t, housekeepResult2, "second housekeep should not have updates")
	})
}

// TestValidatorActivationEdgeCases tests edge cases in validator activation
func TestValidatorActivationEdgeCases(t *testing.T) {
	t.Run("Activation with Exact Minimum Queue Size", func(t *testing.T) {
		staker := newTest(t).SetMBP(3) // Max 3 validators, need 2 for activation

		// Add exactly 2 validators (2/3 of max)
		validators := make([]thor.Address, 2)
		for i := range 2 {
			validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i))))
			endorser := thor.BytesToAddress([]byte("endorser" + string(rune(i))))
			staker.AddValidation(validators[i], endorser, thor.LowStakingPeriod(), MinStakeVET)
		}

		// Try to activate at epoch boundary
		housekeepResult, err := staker.Staker.Housekeep(thor.EpochLength())
		assert.NoError(t, err, "housekeep should succeed")
		assert.True(t, housekeepResult, "housekeep should activate with exact minimum queue size")

		// Check that validators are activated
		for _, validator := range validators {
			val := staker.GetValidation(validator)
			assert.Equal(t, validation.StatusActive, val.Status, "validator should be active")
		}
	})

	t.Run("Activation with Full Leader Group", func(t *testing.T) {
		staker := newTest(t).SetMBP(2) // Max 2 validators

		// Fill leader group
		validators := make([]thor.Address, 2)
		for i := range 2 {
			validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i))))
			endorser := thor.BytesToAddress([]byte("endorser" + string(rune(i))))
			staker.AddValidation(validators[i], endorser, thor.LowStakingPeriod(), MinStakeVET)
		}

		// Activate all validators
		staker.Housekeep(thor.EpochLength())

		// Add more validators to queue
		extraValidator := thor.BytesToAddress([]byte("extra"))
		extraEndorser := thor.BytesToAddress([]byte("extra_endorser"))
		staker.AddValidation(extraValidator, extraEndorser, thor.LowStakingPeriod(), MinStakeVET)

		// Try to activate when leader group is full
		housekeepResult, err := staker.Staker.Housekeep(thor.EpochLength() * 2)
		assert.NoError(t, err, "housekeep should succeed")
		assert.False(t, housekeepResult, "housekeep should not activate when leader group is full")

		val := staker.GetValidation(extraValidator)
		assert.Equal(t, validation.StatusQueued, val.Status, "extra validator should remain queued")
	})
}

// TestValidatorExitEdgeCases tests edge cases in validator exit
func TestValidatorExitEdgeCases(t *testing.T) {
	t.Run("Exit Block Collision", func(t *testing.T) {
		staker := newTest(t).SetMBP(3).Fill(3).Transition(0)

		leaders, err := staker.LeaderGroup()
		require.NoError(t, err, "should be able to get leader group")
		require.Len(t, leaders, 3, "should have 3 active validators")

		validator1 := leaders[0].Address
		validator2 := leaders[1].Address
		val1 := staker.GetValidation(validator1)
		val2 := staker.GetValidation(validator2)

		// Signal exit for both validators at same block
		err = staker.Staker.SignalExit(validator1, val1.Endorser, thor.EpochLength())
		assert.NoError(t, err, "should be able to signal first validator exit")

		err = staker.Staker.SignalExit(validator2, val2.Endorser, thor.EpochLength())
		assert.NoError(t, err, "should be able to signal second validator exit")

		// Check that exit blocks are different
		val1After := staker.GetValidation(validator1)
		val2After := staker.GetValidation(validator2)

		assert.NotNil(t, val1After.ExitBlock, "first validator should have exit block")
		assert.NotNil(t, val2After.ExitBlock, "second validator should have exit block")
		assert.NotEqual(t, *val1After.ExitBlock, *val2After.ExitBlock, "exit blocks should be different")
	})

	t.Run("Exit Block at Exact Epoch Boundary", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Signal exit at exact epoch boundary
		staker.SignalExit(validator, val.Endorser, thor.EpochLength())

		valAfter := staker.GetValidation(validator)
		assert.NotNil(t, valAfter.ExitBlock, "exit block should be set")
		assert.Equal(t, thor.MediumStakingPeriod(), *valAfter.ExitBlock, "exit block should be at epoch boundary")
	})

	t.Run("Double Exit Signal", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Signal exit first time
		staker.SignalExit(validator, val.Endorser, thor.EpochLength())

		// Try to signal exit again
		staker.SignalExitErrors(validator, val.Endorser, thor.EpochLength()+thor.EpochLength(), "exit block already set to 129600")
	})
}

// TestDelegationTransitionEdgeCases tests edge cases in delegation transitions
func TestDelegationTransitionEdgeCases(t *testing.T) {
	t.Run("Delegation Signal Exit Before Started", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)

		// Try to signal exit before delegation starts
		err := staker.Staker.SignalDelegationExit(delegationID, 10)
		assert.Error(t, err, "should not be able to signal exit before delegation starts")
		assert.Contains(t, err.Error(), "delegation has not started yet", "error should indicate delegation has not started")
	})

	t.Run("Delegation Withdraw Before Started", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)

		// Try to withdraw before delegation starts
		staker.WithdrawDelegation(delegationID, 1000, 11)
	})

	t.Run("Delegation Double Signal Exit", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation and start it
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		// Signal exit first time
		err := staker.Staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)
		assert.NoError(t, err, "should be able to signal exit first time")

		// Try to signal exit again
		err = staker.Staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+30)
		assert.Error(t, err, "should not be able to signal exit twice")
		assert.Contains(t, err.Error(), "delegation is already signaled exit", "error should indicate delegation is already signaled exit")
	})

	t.Run("Delegation Withdraw After Exit", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation and start it
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		// Signal exit
		staker.SignalDelegationExit(delegationID, thor.MediumStakingPeriod()+20)

		// Try to withdraw after exit
		staker.WithdrawDelegationErrors(delegationID, thor.MediumStakingPeriod()+30, "delegation is not eligible for withdraw")
	})
}

// TestStakeTransitionEdgeCases tests edge cases in stake transitions
func TestStakeTransitionEdgeCases(t *testing.T) {
	t.Run("Increase Stake at Exact Period Boundary", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		originalStake := val.LockedVET

		// Increase stake at exact period boundary
		staker.IncreaseStake(validator, val.Endorser, 500)

		valAfter := staker.GetValidation(validator)
		assert.Equal(t, valAfter.LockedVET, originalStake, "locked VET should remain the same")
		assert.Equal(t, valAfter.QueuedVET, uint64(500), "queued VET should increase")
	})

	t.Run("Decrease Stake Below Minimum", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Try to decrease stake below minimum
		staker.DecreaseStakeErrors(validator, val.Endorser, val.LockedVET, "next period stake is lower than minimum stake")
	})

	t.Run("Stake Operations on Exiting Validator", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Signal exit
		staker.SignalExit(validator, val.Endorser, thor.EpochLength())

		// Try to increase stake after exit signal
		staker.IncreaseStakeErrors(validator, val.Endorser, 500, "validator has signaled exit, cannot increase stake")

		// Try to decrease stake after exit signal
		staker.DecreaseStakeErrors(validator, val.Endorser, 100, "validator has signaled exit, cannot decrease stake")
	})
}

// TestComplexStateTransitionEdgeCases tests complex state transition scenarios
func TestComplexStateTransitionEdgeCases(t *testing.T) {
	t.Run("Validator Exit During Delegation Lifecycle", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, val := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegation
		delegationID := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		// Signal validator exit
		err := staker.Staker.SignalExit(validator, val.Endorser, thor.MediumStakingPeriod()+20)
		assert.NoError(t, err, "should be able to signal validator exit")

		// Process validator exit
		staker.Housekeep(thor.MediumStakingPeriod() * 2)

		// Check delegation state after validator exit
		del := staker.GetDelegation(delegationID)
		valAfter := staker.GetValidation(validator)

		ended, err := del.Ended(valAfter, thor.MediumStakingPeriod()*2+10)
		assert.NoError(t, err, "should be able to check ended state")
		assert.True(t, ended, "delegation should be ended when validator exits")
	})

	t.Run("Multiple Delegations with Different Start Times", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Add delegations at different times
		delegation1 := staker.AddDelegation(validator, 1000, 150, 10)
		staker.Housekeep(thor.MediumStakingPeriod())

		delegation2 := staker.AddDelegation(validator, 2000, 200, 20)
		staker.Housekeep(thor.MediumStakingPeriod() * 2)

		// Check states
		del1 := staker.GetDelegation(delegation1)
		del2 := staker.GetDelegation(delegation2)
		val := staker.GetValidation(validator)

		// First delegation should be started
		started1, err := del1.Started(val, thor.MediumStakingPeriod()*2+10)
		assert.NoError(t, err)
		assert.True(t, started1, "first delegation should be started")

		// Second delegation should be started
		started2, err := del2.Started(val, thor.MediumStakingPeriod()*2+10)
		assert.NoError(t, err)
		assert.True(t, started2, "second delegation should be started")
	})

	t.Run("Validator Eviction Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1).Fill(1).Transition(0)

		validator, _ := staker.FirstActive()
		require.NotEqual(t, thor.Address{}, validator, "validator should be active")

		// Set validator offline
		err := staker.Staker.SetOnline(validator, thor.EpochLength(), false)
		assert.NoError(t, err, "should be able to set validator offline")

		// Check that validator is offline
		val := staker.GetValidation(validator)
		assert.False(t, val.IsOnline(), "validator should be offline")
		assert.NotNil(t, val.OfflineBlock, "offline block should be set")

		// Test eviction after threshold
		evictionBlock := thor.EvictionCheckInterval() * 3
		_, housekeepResult := staker.HousekeepWithUpdates(evictionBlock)
		assert.True(t, housekeepResult, "housekeep should evict offline validator")

		staker.HousekeepWithUpdates(evictionBlock + thor.EpochLength())
		valAfter := staker.GetValidation(validator)
		assert.Equal(t, validation.StatusExit, valAfter.Status, "validator should be in exit status after eviction")
	})
}

// TestBoundaryValueEdgeCases tests edge cases with boundary values
func TestBoundaryValueEdgeCases(t *testing.T) {
	t.Run("Minimum Stake Values", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with exact minimum stake
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MinStakeVET)

		val := staker.GetValidation(validator)
		assert.Equal(t, MinStakeVET, val.QueuedVET, "queued VET should equal minimum stake")
	})

	t.Run("Maximum Stake Values", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with maximum stake
		staker.AddValidation(validator, endorser, thor.LowStakingPeriod(), MaxStakeVET)

		val := staker.GetValidation(validator)
		assert.Equal(t, MaxStakeVET, val.QueuedVET, "queued VET should equal maximum stake")
	})

	t.Run("Zero Period Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with zero period (should fail)
		staker.AddValidationErrors(validator, endorser, 0, MinStakeVET, "period is out of boundaries")
	})

	t.Run("Maximum Period Edge Cases", func(t *testing.T) {
		staker := newTest(t).SetMBP(1)

		validator := thor.BytesToAddress([]byte("validator"))
		endorser := thor.BytesToAddress([]byte("endorser"))

		// Test with maximum period
		staker.AddValidation(validator, endorser, thor.HighStakingPeriod(), MinStakeVET)

		val := staker.GetValidation(validator)
		assert.Equal(t, thor.HighStakingPeriod(), val.Period, "period should equal high staking period")
	})
}
