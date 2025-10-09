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
	staker, _ := newStaker(t, 0, 3, false) // Max 3 validators

	// Create validator addresses
	validator1 := thor.BytesToAddress([]byte("validator1"))
	validator2 := thor.BytesToAddress([]byte("validator2"))
	validator3 := thor.BytesToAddress([]byte("validator3"))
	endorser1 := thor.BytesToAddress([]byte("endorser1"))
	endorser2 := thor.BytesToAddress([]byte("endorser2"))
	endorser3 := thor.BytesToAddress([]byte("endorser3"))

	// Step 1: Add three validators
	err := staker.AddValidation(validator1, endorser1, thor.MediumStakingPeriod(), MinStakeVET)
	require.NoError(t, err)
	err = staker.AddValidation(validator2, endorser2, thor.MediumStakingPeriod(), MinStakeVET)
	require.NoError(t, err)
	err = staker.AddValidation(validator3, endorser3, thor.MediumStakingPeriod(), MinStakeVET)
	require.NoError(t, err)

	// Step 2: Activate all validators
	_, err = staker.Housekeep(thor.MediumStakingPeriod())
	require.NoError(t, err)

	// Verify all validators are active
	val1, err := staker.GetValidation(validator1)
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val1.Status)

	val2, err := staker.GetValidation(validator2)
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val2.Status)

	val3, err := staker.GetValidation(validator3)
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val3.Status)

	// Step 3: Add delegations to all validators
	delegation1, err := staker.AddDelegation(validator1, 1000, 100, thor.MediumStakingPeriod()+1)
	require.NoError(t, err)
	delegation2, err := staker.AddDelegation(validator2, 2000, 150, thor.MediumStakingPeriod()+1)
	require.NoError(t, err)
	delegation3, err := staker.AddDelegation(validator3, 3000, 200, thor.MediumStakingPeriod()+1)
	require.NoError(t, err)

	// Step 4: Signal exit for validator1
	err = staker.SignalExit(validator1, endorser1, thor.MediumStakingPeriod()+2)
	require.NoError(t, err)

	// Step 5: Process the exit
	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 2)
	require.NoError(t, err)

	// Step 6: Verify validator1 is in exit state
	val1, err = staker.GetValidation(validator1)
	require.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val1.Status)

	// Step 7: Verify other validators are still active
	val2, err = staker.GetValidation(validator2)
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val2.Status)

	val3, err = staker.GetValidation(validator3)
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val3.Status)

	// Step 8: Verify delegations are still active on remaining validators
	del2, val2, err := staker.GetDelegation(delegation2)
	require.NoError(t, err)
	started, err := del2.Started(val2, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation2 should still be active")

	del3, val3, err := staker.GetDelegation(delegation3)
	require.NoError(t, err)
	started, err = del3.Started(val3, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation3 should still be active")

	// Step 9: Verify delegation1 is ended due to validator1 exit
	del1, val1, err := staker.GetDelegation(delegation1)
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

	withdrawAmt, err := staker.WithdrawDelegation(delegation1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	require.Equal(t, uint64(1000), withdrawAmt)
}

// TestCascadingExits tests multiple validators exiting in sequence
func TestCascadingExits(t *testing.T) {
	staker, _ := newStaker(t, 0, 7, false) // Max 5 validators

	// Create validator addresses
	validators := make([]thor.Address, 7)
	endorsers := make([]thor.Address, 7)
	for i := range 7 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 7 {
		err := staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
		require.NoError(t, err)
	}

	delegations := make([]*big.Int, 7)
	for i := range 7 {
		del, err := staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, thor.MediumStakingPeriod()+1)
		require.NoError(t, err)
		delegations[i] = del
	}

	// Step 2: Activate all validators
	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	require.NoError(t, err)

	// Step 4: Signal exits for validators 1, 3, and 5
	exitValidators := []int{0, 2, 4} // 1, 3, 5
	for _, i := range exitValidators {
		err = staker.SignalExit(validators[i], endorsers[i], thor.MediumStakingPeriod()+2)
		require.NoError(t, err)
	}

	// Step 5: Process 1st exit
	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 2)
	require.NoError(t, err)

	// Step 6: Process 2nd exit
	_, err = staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength())
	require.NoError(t, err)

	// Step 6: Process 3rd exit
	_, err = staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength()*2)
	require.NoError(t, err)

	// Step 6: Verify exit validators are in exit state
	for _, i := range exitValidators {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusExit, val.Status, "validator %d should be in exit state", i+1)
	}

	// Step 7: Verify remaining validators are still active
	remainingValidators := []int{1, 3} // 2, 4
	for _, i := range remainingValidators {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should still be active", i+1)
	}

	// Step 8: Verify delegations on remaining validators are still active
	for _, i := range remainingValidators {
		del, val, err := staker.GetDelegation(delegations[i])
		require.NoError(t, err)
		started, err := del.Started(val, thor.MediumStakingPeriod()*2+1)
		require.NoError(t, err)
		assert.True(t, started, "delegation on validator %d should still be active", i+1)
	}

	// Step 9: Verify delegations on exit validators are ended
	for _, i := range exitValidators {
		del, val, err := staker.GetDelegation(delegations[i])
		require.NoError(t, err)
		ended, err := del.Ended(val, thor.MediumStakingPeriod()*2+1)
		require.NoError(t, err)
		assert.True(t, ended, "delegation on validator %d should be ended", i+1)
	}
}

// TestQueueManagement tests validators moving through the queue
func TestQueueManagement(t *testing.T) {
	staker, _ := newStaker(t, 0, 2, false) // Max 2 active validators

	// Create validator addresses
	validators := make([]thor.Address, 5)
	endorsers := make([]thor.Address, 5)
	for i := range 5 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators (they will be queued)
	for i := range 5 {
		err := staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
		require.NoError(t, err)
	}

	// Step 2: Verify all validators are queued
	for i := range 5 {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusQueued, val.Status, "validator %d should be queued", i+1)
	}

	// Step 3: Activate first 2 validators
	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	require.NoError(t, err)

	// Step 4: Verify first 2 validators are active
	for i := range 2 {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should be active", i+1)
	}

	// Step 5: Verify remaining validators are still queued
	for i := 2; i < 5; i++ {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusQueued, val.Status, "validator %d should still be queued", i+1)
	}

	// Step 6: Signal exit for validator 1
	err = staker.SignalExit(validators[0], endorsers[0], thor.MediumStakingPeriod()+1)
	require.NoError(t, err)

	// Step 7: Process the exit
	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 2)
	require.NoError(t, err)

	// Step 8: Verify validator 1 is in exit state
	val1, err := staker.GetValidation(validators[0])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be in exit state")

	// Step 9: Verify validator 3 is now active (moved from queue)
	val3, err := staker.GetValidation(validators[2])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val3.Status, "validator 3 should now be active")

	// Step 10: Verify remaining validators are still queued
	for i := 3; i < 5; i++ {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusQueued, val.Status, "validator %d should still be queued", i+1)
	}
}

// TestLeaderGroupRotation tests active validators rotating out
func TestLeaderGroupRotation(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false) // Max 3 active validators

	// Create validator addresses
	validators := make([]thor.Address, 4)
	endorsers := make([]thor.Address, 4)
	for i := range 4 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 4 {
		err := staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
		require.NoError(t, err)
	}

	// Step 2: Activate first 3 validators
	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	require.NoError(t, err)

	// Step 3: Verify first 3 validators are active
	for i := range 3 {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should be active", i+1)
	}

	// Step 4: Verify validator 4 is queued
	val4, err := staker.GetValidation(validators[3])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, val4.Status, "validator 4 should be queued")

	// Step 5: Add delegations to active validators
	delegations := make([]*big.Int, 3)
	for i := range 3 {
		delegations[i], err = staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, thor.MediumStakingPeriod()+1)
		require.NoError(t, err)
	}

	// Step 6: Signal exit for validator 1
	err = staker.SignalExit(validators[0], endorsers[0], thor.MediumStakingPeriod()+2)
	require.NoError(t, err)

	// Step 7: Process the exit
	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 2)
	require.NoError(t, err)

	// Step 8: Verify validator 1 is in exit state
	val1, err := staker.GetValidation(validators[0])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be in exit state")

	// Step 9: Verify validator 4 is now active (rotated in)
	val4, err = staker.GetValidation(validators[3])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val4.Status, "validator 4 should now be active")

	// Step 10: Verify remaining validators are still active
	for i := 1; i < 3; i++ {
		val, err := staker.GetValidation(validators[i])
		require.NoError(t, err)
		assert.Equal(t, validation.StatusActive, val.Status, "validator %d should still be active", i+1)
	}

	// Step 11: Verify delegation on validator 1 is ended
	del1, val1, err := staker.GetDelegation(delegations[0])
	require.NoError(t, err)
	started, err := del1.Started(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.False(t, started, "delegation on validator 1 shouldn't be started")
	ended, err := del1.Ended(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.False(t, ended, "delegation on validator 1 shouldn't be ended")

	// Step 12: Verify delegations on remaining validators are still active
	for i := 1; i < 3; i++ {
		del, val, err := staker.GetDelegation(delegations[i])
		require.NoError(t, err)
		started, err := del.Started(val, thor.MediumStakingPeriod()*2+1)
		require.NoError(t, err)
		assert.True(t, started, "delegation on validator %d should still be active", i+1)
	}
}

// TestValidatorEviction tests offline validator being evicted
func TestValidatorEviction(t *testing.T) {
	staker, _ := newStaker(t, 0, 3, false) // Max 3 active validators

	// Create validator addresses
	validators := make([]thor.Address, 4)
	endorsers := make([]thor.Address, 4)
	for i := range 4 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 4 {
		err := staker.AddValidation(validators[i], endorsers[i], thor.LowStakingPeriod(), MinStakeVET)
		require.NoError(t, err)
	}

	delegations := make([]*big.Int, 3)
	for i := range 3 {
		del, err := staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, 15)
		require.NoError(t, err)
		delegations[i] = del
	}

	// Step 2: Activate first 3 validators
	_, err := staker.Housekeep(thor.LowStakingPeriod())
	require.NoError(t, err)

	// Step 4: Simulate validator 1 going offline by setting offline block
	val1, err := staker.GetValidation(validators[0])
	require.NoError(t, err)
	val1.OfflineBlock = &[]uint32{1}[0]
	err = staker.validationService.UpdateOfflineBlock(validators[0], 1, false)
	require.NoError(t, err)

	// Step 5: Wait for eviction threshold
	evictionBlock := thor.EvictionCheckInterval() * 3
	_, err = staker.Housekeep(evictionBlock)
	require.NoError(t, err)

	_, err = staker.Housekeep(evictionBlock + thor.EpochLength())
	require.NoError(t, err)

	// Step 6: Verify validator 1 is evicted (should be in exit state)
	val1, err = staker.GetValidation(validators[0])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be evicted")

	// Step 7: Verify validator 4 is now active (replaced evicted validator)
	val4, err := staker.GetValidation(validators[3])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val4.Status, "validator 4 should now be active")

	// Step 8: Verify delegation on evicted validator is ended
	del1, val1, err := staker.GetDelegation(delegations[0])
	require.NoError(t, err)
	started, err := del1.Started(val1, evictionBlock+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on evicted validator should be started")
	ended, err := del1.Ended(val1, evictionBlock+1)
	require.NoError(t, err)
	assert.True(t, ended, "delegation on evicted validator should be ended")

	// Step 9: Verify delegations on remaining validators are still active
	for i := 1; i < 3; i++ {
		del, val, err := staker.GetDelegation(delegations[i])
		require.NoError(t, err)
		started, err := del.Started(val, evictionBlock+1)
		require.NoError(t, err)
		assert.True(t, started, "delegation on validator %d should still be active", i+1)
	}
}

// TestComplexMultiValidatorScenario tests a complex scenario with multiple validators and delegations
func TestComplexMultiValidatorScenario(t *testing.T) {
	staker, _ := newStaker(t, 0, 4, false) // Max 4 active validators

	// Create validator addresses
	validators := make([]thor.Address, 6)
	endorsers := make([]thor.Address, 6)
	for i := range 6 {
		validators[i] = thor.BytesToAddress([]byte("validator" + string(rune(i+1))))
		endorsers[i] = thor.BytesToAddress([]byte("endorser" + string(rune(i+1))))
	}

	// Step 1: Add all validators
	for i := range 6 {
		err := staker.AddValidation(validators[i], endorsers[i], thor.MediumStakingPeriod(), MinStakeVET)
		require.NoError(t, err)
	}

	delegations := make([]*big.Int, 4)
	for i := range 4 {
		del, err := staker.AddDelegation(validators[i], uint64(1000*(i+1)), 100, 10)
		require.NoError(t, err)
		delegations[i] = del
	}

	// Step 2: Activate first 4 validators
	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	require.NoError(t, err)

	// Step 4: Signal exits for validators 1 and 3
	err = staker.SignalExit(validators[0], endorsers[0], thor.MediumStakingPeriod()+2)
	require.NoError(t, err)
	err = staker.SignalExit(validators[2], endorsers[2], thor.MediumStakingPeriod()+2)
	require.NoError(t, err)

	// Step 5: Process the exits
	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 2)
	require.NoError(t, err)

	_, err = staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength())
	require.NoError(t, err)

	// Step 6: Verify exit validators are in exit state
	val1, err := staker.GetValidation(validators[0])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val1.Status, "validator 1 should be in exit state")

	val3, err := staker.GetValidation(validators[2])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusExit, val3.Status, "validator 3 should be in exit state")

	// Step 7: Verify validators 5 and 6 are now active (moved from queue)
	val5, err := staker.GetValidation(validators[4])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val5.Status, "validator 5 should now be active")

	val6, err := staker.GetValidation(validators[5])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val6.Status, "validator 6 should now be active")

	// Step 8: Verify remaining validators are still active
	val2, err := staker.GetValidation(validators[1])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val2.Status, "validator 2 should still be active")

	val4, err := staker.GetValidation(validators[3])
	require.NoError(t, err)
	assert.Equal(t, validation.StatusActive, val4.Status, "validator 4 should still be active")

	// Step 9: Add delegations to new active validators
	delegation5, err := staker.AddDelegation(validators[4], 5000, 150, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)

	_, err = staker.Housekeep(thor.MediumStakingPeriod()*2 + thor.EpochLength()*2)
	require.NoError(t, err)
	delegation6, err := staker.AddDelegation(validators[5], 6000, 200, thor.MediumStakingPeriod()*2+thor.EpochLength()*2+1)
	require.NoError(t, err)

	// Step 10: Verify delegations on exit validators are ended
	del1, val1, err := staker.GetDelegation(delegations[0])
	require.NoError(t, err)
	ended, err := del1.Ended(val1, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, ended, "delegation on validator 1 should be ended")

	del3, val3, err := staker.GetDelegation(delegations[2])
	require.NoError(t, err)
	ended, err = del3.Ended(val3, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, ended, "delegation on validator 3 should be ended")

	// Step 11: Verify delegations on remaining validators are still active
	del2, val2, err := staker.GetDelegation(delegations[1])
	require.NoError(t, err)
	started, err := del2.Started(val2, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 2 should still be active")

	del4, val4, err := staker.GetDelegation(delegations[3])
	require.NoError(t, err)
	started, err = del4.Started(val4, thor.MediumStakingPeriod()*2+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 4 should still be active")

	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 3)
	require.NoError(t, err)
	// Step 12: Verify new delegations are active
	del5, val5, err := staker.GetDelegation(delegation5)
	require.NoError(t, err)
	started, err = del5.Started(val5, thor.MediumStakingPeriod()*3+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 5 should be active")

	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 4)
	require.NoError(t, err)
	del6, val6, err := staker.GetDelegation(delegation6)
	require.NoError(t, err)
	started, err = del6.Started(val6, thor.MediumStakingPeriod()*4+1)
	require.NoError(t, err)
	assert.True(t, started, "delegation on validator 6 should be active")
}
