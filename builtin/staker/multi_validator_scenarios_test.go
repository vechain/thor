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

// TestValidatorReplacement tests the scenario where a new validator replaces an exiting one
func TestValidatorExitInSamePeriod(t *testing.T) {
	staker := newTest(t).SetMBP(3)

	// Create validator addresses
	validator1 := thor.BytesToAddress([]byte("validator1"))
	validator2 := thor.BytesToAddress([]byte("validator2"))
	validator3 := thor.BytesToAddress([]byte("validator3"))
	endorser1 := thor.BytesToAddress([]byte("endorser1"))
	endorser2 := thor.BytesToAddress([]byte("endorser2"))
	endorser3 := thor.BytesToAddress([]byte("endorser3"))

	// Step 1: Add three validators
	staker.AddValidation(validator1, endorser1, thor.MediumStakingPeriod(), MinStakeVET)
	staker.AddValidation(validator2, endorser2, thor.MediumStakingPeriod(), MinStakeVET)
	staker.AddValidation(validator3, endorser3, thor.MediumStakingPeriod(), MinStakeVET)

	// Step 2: Activate all validators
	staker.Housekeep(thor.MediumStakingPeriod())

	// Verify all validators are active
	val1 := staker.GetValidation(validator1)
	assert.Equal(t, validation.StatusActive, val1.Status)

	val2 := staker.GetValidation(validator2)
	assert.Equal(t, validation.StatusActive, val2.Status)

	val3 := staker.GetValidation(validator3)
	assert.Equal(t, validation.StatusActive, val3.Status)

	// Step 3: Add delegations to all validators
	delegation1 := staker.AddDelegation(validator1, 1000, 100, thor.MediumStakingPeriod()+1)
	delegation2 := staker.AddDelegation(validator2, 2000, 150, thor.MediumStakingPeriod()+1)
	delegation3 := staker.AddDelegation(validator3, 3000, 200, thor.MediumStakingPeriod()+1)

	// Step 4: Signal exit for validator1
	staker.SignalExit(validator1, endorser1, thor.MediumStakingPeriod()+2)

	// Step 5: Process the exit
	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Step 6: Verify validator1 is in exit state
	val1 = staker.GetValidation(validator1)
	assert.Equal(t, validation.StatusExit, val1.Status)

	// Step 7: Verify other validators are still active
	staker.GetValidation(validator2)
	assert.Equal(t, validation.StatusActive, val2.Status)

	staker.GetValidation(validator3)
	assert.Equal(t, validation.StatusActive, val3.Status)

	// Step 8: Verify delegations are still active on remaining validators
	del2 := staker.GetDelegation(delegation2)
	started, err := del2.Started(val2, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation2 should still be active")

	del3 := staker.GetDelegation(delegation3)
	started, err = del3.Started(val3, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation3 should still be active")

	// Step 9: Verify delegation1 is ended due to validator1 exit
	del1 := staker.GetDelegation(delegation1)
	require.NoError(t, err)

	isLocked, err := del1.IsLocked(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	require.False(t, isLocked)

	ended, err := del1.Ended(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.False(t, ended, "delegation1 should be not ended due to validator1 exit")

	started, err = del1.Started(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.False(t, started, "delegation1 should not be started due to validator1 exit")

	staker.WithdrawDelegation(delegation1, uint64(1000), thor.MediumStakingPeriod()*2+1)
}

// TestCascadingExits tests multiple validators exiting in sequence
func TestCascadingExits(t *testing.T) {
	staker := newTest(t).SetMBP(5) // Max 5 validators

	// Create validator addresses
	validators := make([]thor.Address, 7)
	endorsers := make([]thor.Address, 7)
	for i := range 7 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 7 {
		staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
	}

	delegations := make([]*big.Int, 7)
	for i := range 7 {
		del := staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, thor.MediumStakingPeriod()+1)
		delegations[i] = del
	}

	// Step 2: Activate all validators
	staker.Housekeep(thor.MediumStakingPeriod())

	// Step 4: Signal exits for validators 1, 3, and 5
	exitValidators := []int{0, 2, 4} // 1, 3, 5
	for _, i := range exitValidators {
		staker.SignalExit(validators[i], endorsers[i], thor.MediumStakingPeriod()+2)
	}

	// Step 5: Process 1st exit
	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Step 6: Process 2nd exit
	staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength())

	// Step 6: Process 3rd exit
	staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength()*2)

	// Step 6: Verify exit validators are in exit state
	for _, i := range exitValidators {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusExit, val.Status, "validator %d should be in exit state", i+1)
	}

	// Step 7: Verify remaining validators are still active
	remainingValidators := []int{1, 3} // 2, 4
	for _, i := range remainingValidators {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should still be active", i+1)
	}

	// Step 8: Verify delegations on remaining validators are still active
	for _, i := range remainingValidators {
		del := staker.GetDelegation(delegations[i])
		val := staker.GetValidation(del.Validation)
		started, err := del.Started(val, thor.MediumStakingPeriod()*2+1)
		require.NoError(t, err)
		assert.True(t, started, "delegation on validator %d should still be active", i+1)
	}

	// Step 9: Verify delegations on exit validators are ended
	for _, i := range exitValidators {
		del := staker.GetDelegation(delegations[i])
		val := staker.GetValidation(del.Validation)
		ended, err := del.Ended(val, thor.MediumStakingPeriod()*2+1)
		require.NoError(t, err)
		assert.True(t, ended, "delegation on validator %d should be ended", i+1)
	}
}

// TestQueueManagement tests validators moving through the queue
func TestQueueManagement(t *testing.T) {
	staker := newTest(t).SetMBP(2) // Max 2 active validators

	// Create validator addresses
	validators := make([]thor.Address, 5)
	endorsers := make([]thor.Address, 5)
	for i := range 5 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators (they will be queued)
	for i := range 5 {
		staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
	}

	// Step 2: Verify all validators are queued
	for i := range 5 {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusQueued, val.Status, "validator %d should be queued", i+1)
	}

	// Step 3: Activate first 2 validators
	staker.Housekeep(thor.MediumStakingPeriod())

	// Step 4: Verify first 2 validators are active
	for i := range 2 {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should be active", i+1)
	}

	// Step 5: Verify remaining validators are still queued
	for i := 2; i < 5; i++ {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusQueued, val.Status, "validator %d should still be queued", i+1)
	}

	// Step 6: Signal exit for validator 1
	staker.SignalExit(validators[0], endorsers[0], thor.MediumStakingPeriod()+1)

	// Step 7: Process the exit
	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Step 8: Verify validator 1 is in exit state
	val1 := staker.GetValidation(validators[0])
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be in exit state")

	// Step 9: Verify validator 3 is now active (moved from queue)
	val3 := staker.GetValidation(validators[2])
	assert.Equal(t, validation.StatusActive, val3.Status, "validator 3 should now be active")

	// Step 10: Verify remaining validators are still queued
	for i := 3; i < 5; i++ {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusQueued, val.Status, "validator %d should still be queued", i+1)
	}
}

// TestLeaderGroupRotation tests active validators rotating out
func TestLeaderGroupRotation(t *testing.T) {
	staker := newTest(t).SetMBP(3) // Max 3 active validators

	// Create validator addresses
	validators := make([]thor.Address, 4)
	endorsers := make([]thor.Address, 4)
	for i := range 4 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 4 {
		staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
	}

	// Step 2: Activate first 3 validators
	staker.Housekeep(thor.MediumStakingPeriod())

	// Step 3: Verify first 3 validators are active
	for i := range 3 {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should be active", i+1)
	}

	// Step 4: Verify validator 4 is queued
	val4 := staker.GetValidation(validators[3])
	assert.Equal(t, validation.StatusQueued, val4.Status, "validator 4 should be queued")

	// Step 5: Add delegations to active validators
	delegations := make([]*big.Int, 3)
	for i := range 3 {
		delegations[i] = staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, thor.MediumStakingPeriod()+1)
	}

	// Step 6: Signal exit for validator 1
	staker.SignalExit(validators[0], endorsers[0], thor.MediumStakingPeriod()+2)

	// Step 7: Process the exit
	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	// Step 8: Verify validator 1 is in exit state
	val1 := staker.GetValidation(validators[0])
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be in exit state")

	// Step 9: Verify validator 4 is now active (rotated in)
	val4 = staker.GetValidation(validators[3])
	assert.Equal(t, validation.StatusActive, val4.Status, "validator 4 should now be active")

	// Step 10: Verify remaining validators are still active
	for i := 1; i < 3; i++ {
		val := staker.GetValidation(validators[i])
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should still be active", i+1)
	}

	// Step 11: Verify delegation on validator 1 is ended
	del1 := staker.GetDelegation(delegations[0])
	val1 = staker.GetValidation(del1.Validation)
	started, err := del1.Started(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.False(t, started, "delegation on validator 1 shouldn't be started")
	ended, err := del1.Ended(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.False(t, ended, "delegation on validator 1 shouldn't be ended")

	// Step 12: Verify delegations on remaining validators are still active
	for i := 1; i < 3; i++ {
		del := staker.GetDelegation(delegations[i])
		val := staker.GetValidation(del.Validation)
		started, err := del.Started(val, thor.MediumStakingPeriod()*2+1)
		require.NoError(t, err)
		assert.True(t, started, "delegation on validator %d should still be active", i+1)
	}
}

// TestValidatorEviction tests offline validator being evicted
func TestValidatorEviction(t *testing.T) {
	staker := newTest(t).SetMBP(3)

	// Create validator addresses
	validators := make([]thor.Address, 4)
	endorsers := make([]thor.Address, 4)
	for i := range 4 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 4 {
		staker.AddValidation(validators[i], endorsers[i], thor.LowStakingPeriod(), MinStakeVET)
	}

	delegations := make([]*big.Int, 3)
	for i := range 3 {
		del := staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, 15)
		delegations[i] = del
	}

	// Step 2: Activate first 3 validators
	staker.Housekeep(thor.LowStakingPeriod())

	// Step 4: Simulate validator 1 going offline by setting offline block
	val1 := staker.GetValidation(validators[0])
	val1.OfflineBlock = &[]uint32{1}[0]
	err := staker.validationService.UpdateOfflineBlock(validators[0], 1, false)
	assert.NoError(t, err)

	// Step 5: Wait for eviction threshold
	evictionBlock := thor.EvictionCheckInterval() * 3
	staker.Housekeep(evictionBlock)

	staker.Housekeep(evictionBlock + thor.EpochLength())

	// Step 6: Verify validator 1 is evicted (should be in exit state)
	val1 = staker.GetValidation(validators[0])
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be evicted")

	// Step 7: Verify validator 4 is now active (replaced evicted validator)
	val4 := staker.GetValidation(validators[3])
	assert.Equal(t, validation.StatusActive, val4.Status, "validator 4 should now be active")

	// Step 8: Verify delegation on evicted validator is ended
	del1 := staker.GetDelegation(delegations[0])
	val1 = staker.GetValidation(del1.Validation)
	started, err := del1.Started(val1, evictionBlock+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on evicted validator should be started")
	ended, err := del1.Ended(val1, evictionBlock+1)
	require.NoError(t, err)
	assert.True(t, ended, "delegation on evicted validator should be ended")

	// Step 9: Verify delegations on remaining validators are still active
	for i := 1; i < 3; i++ {
		del := staker.GetDelegation(delegations[i])
		val := staker.GetValidation(del.Validation)
		started, err := del.Started(val, evictionBlock+1)
		require.NoError(t, err)
		assert.True(t, started, "delegation on validator %d should still be active", i+1)
	}
}

// TestComplexMultiValidatorScenario tests a complex scenario with multiple validators and delegations
func TestComplexMultiValidatorScenario(t *testing.T) {
	staker := newTest(t).SetMBP(4) // Max 4 active validators

	// Create validator addresses
	validators := make([]thor.Address, 6)
	endorsers := make([]thor.Address, 6)
	for i := range 6 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 6 {
		staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
	}

	delegations := make([]*big.Int, 4)
	for i := range 4 {
		del := staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, 10)
		delegations[i] = del
	}

	// Step 2: Activate first 4 validators
	staker.Housekeep(thor.MediumStakingPeriod())

	// Step 4: Signal exits for validators 1 and 3
	staker.SignalExit(validators[0], endorsers[0], thor.MediumStakingPeriod()+2)
	staker.SignalExit(validators[2], endorsers[2], thor.MediumStakingPeriod()+2)

	// Step 5: Process the exits
	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength())

	// Step 6: Verify exit validators are in exit state
	val1 := staker.GetValidation(validators[0])
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be in exit state")

	val3 := staker.GetValidation(validators[2])
	assert.Equal(t, validation.StatusExit, val3.Status, "validator 3 should be in exit state")

	// Step 7: Verify validators 5 and 6 are now active (moved from queue)
	val5 := staker.GetValidation(validators[4])
	assert.Equal(t, validation.StatusActive, val5.Status, "validator 5 should now be active")

	val6 := staker.GetValidation(validators[5])
	assert.Equal(t, validation.StatusActive, val6.Status, "validator 6 should now be active")

	// Step 8: Verify remaining validators are still active
	val2 := staker.GetValidation(validators[1])
	assert.Equal(t, validation.StatusActive, val2.Status, "validator 2 should still be active")

	val4 := staker.GetValidation(validators[3])
	assert.Equal(t, validation.StatusActive, val4.Status, "validator 4 should still be active")

	// Step 9: Add delegations to new active validators
	delegation5 := staker.AddDelegation(validators[4], 5000, 150, thor.MediumStakingPeriod()*2+1)

	staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength()*2)
	delegation6 := staker.AddDelegation(validators[5], 6000, 200, thor.MediumStakingPeriod()*2+thor.EpochLength()*2+1)

	// Step 10: Verify delegations on exit validators are ended
	del1 := staker.GetDelegation(delegations[0])
	val1 = staker.GetValidation(del1.Validation)
	ended, err := del1.Ended(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, ended, "delegation on validator 1 should be ended")

	del3 := staker.GetDelegation(delegations[2])
	val3 = staker.GetValidation(del3.Validation)
	ended, err = del3.Ended(val3, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, ended, "delegation on validator 3 should be ended")

	// Step 11: Verify delegations on remaining validators are still active
	del2 := staker.GetDelegation(delegations[1])
	val2 = staker.GetValidation(del2.Validation)
	started, err := del2.Started(val2, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 2 should still be active")

	del4 := staker.GetDelegation(delegations[3])
	val4 = staker.GetValidation(del4.Validation)
	started, err = del4.Started(val4, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 4 should still be active")

	staker.Housekeep(thor.MediumStakingPeriod() * 3)
	// Step 12: Verify new delegations are active
	del5 := staker.GetDelegation(delegation5)
	val5 = staker.GetValidation(del5.Validation)
	started, err = del5.Started(val5, thor.MediumStakingPeriod()*3+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 5 should be active")

	staker.Housekeep(thor.MediumStakingPeriod() * 4)
	require.NoError(t, err)
	del6 := staker.GetDelegation(delegation6)
	val6 = staker.GetValidation(del6.Validation)
	started, err = del6.Started(val6, thor.MediumStakingPeriod()*4+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 6 should be active")
}
