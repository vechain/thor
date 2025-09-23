// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/delegation"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

type testValidators struct {
	ID thor.Address
	*validation.Validation
}

func newDelegationStaker(t *testing.T) (*TestSequence, []*testValidators) {
	test, _ := newStakerV2(t, 75, 101, true)
	validations := make([]*testValidators, 0)
	err := test.Staker.validationService.LeaderGroupIterator(func(validatorID thor.Address, validation *validation.Validation) error {
		validations = append(validations, &testValidators{
			ID:         validatorID,
			Validation: validation,
		})
		return nil
	})
	assert.NoError(t, err)
	return test, validations
}

func delegationStake() uint64 {
	return 10_000 // 10_000 VET
}

func Test_IsLocked(t *testing.T) {
	t.Run("Completed Staking Periods", func(t *testing.T) {
		last := uint32(2)
		d := &delegation.Delegation{
			FirstIteration: 2,
			LastIteration:  &last,
			Stake:          uint64(1),
			Multiplier:     255,
		}

		v := &validation.Validation{
			Status: validation.StatusActive,
			Period: 5,
		}

		stared, err := d.Started(v, 10)
		assert.NoError(t, err)
		assert.True(t, stared, "should not be locked when complete iterations is equal to last iteration")
		ended, err := d.Ended(v, 15)
		assert.NoError(t, err)
		assert.True(t, ended, "should be locked when first is less than current and last is equal to current")
	})

	t.Run("Incomplete Staking Periods", func(t *testing.T) {
		last := uint32(5)
		d := &delegation.Delegation{
			FirstIteration: 2,
			LastIteration:  &last,
			Stake:          uint64(1),
			Multiplier:     255,
		}

		v := &validation.Validation{
			Status: validation.StatusActive,
			Period: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.True(t, started, "should be started when complete iterations is greater than first iteration")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when first is less than current and last is greater than current")
	})

	t.Run("Delegation Not Started", func(t *testing.T) {
		last := uint32(6)
		d := &delegation.Delegation{
			FirstIteration: 5,
			LastIteration:  &last,
			Stake:          uint64(1),
			Multiplier:     255,
		}

		v := &validation.Validation{
			Status: validation.StatusActive,
			Period: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.False(t, started, "should not be started when complete iterations is less than first iteration")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when first is greater than current and last is greater than current")
	})
	t.Run("Staker is Queued", func(t *testing.T) {
		d := &delegation.Delegation{
			FirstIteration: 1,
			LastIteration:  nil,
			Stake:          uint64(1),
			Multiplier:     255,
		}

		v := &validation.Validation{
			Status: validation.StatusQueued,
			Period: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.False(t, started, "should not be started when validation status is queued")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when validation status is queued")
	})

	t.Run("exit block not defined", func(t *testing.T) {
		d := &delegation.Delegation{
			FirstIteration: 1,
			LastIteration:  nil,
			Stake:          uint64(1),
			Multiplier:     255,
		}

		v := &validation.Validation{
			Status: validation.StatusActive,
			Period: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.True(t, started, "should be started when first iteration is less than current")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when last iteration is nil and first equals current")
	})
}

func Test_AddDelegator(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := MinStakeVET

	validatorID := validators[0].ID

	delegationID := staker.AddDelegation(validatorID, stake, 255, 10)

	staker.AssertDelegation(delegationID).
		IsStarted(false, 10).
		LastIteration(nil)

	weightedStake := stakes.NewWeightedStakeWithMultiplier(stake, 255)

	staker.AssertAggregation(validatorID).
		PendingVET(stake).
		PendingWeight(weightedStake.Weight)
}

func Test_AddDelegator_StakeRange(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	// should NOT be able to stake greater than max stake
	staker.AddDelegationErrors(validators[1].ID, MaxStakeVET, 255, 10, "total stake would exceed maximum")

	// should be able stake 1 VET
	id1 := staker.AddDelegation(validators[2].ID, 1, 255, 10)
	staker.AssertDelegation(id1).Stake(1)
	staker.AssertAggregation(validators[2].ID).PendingVET(1)

	// should be able stake for all remaining space
	validator := validators[3]
	validation := staker.GetValidation(validator.ID)
	valNextPeriodTVL, err := validation.NextPeriodTVL()
	assert.NoError(t, err)
	remaining := MaxStakeVET - valNextPeriodTVL
	staker.AddDelegation(validator.ID, remaining, 255, 10)

	// should not be able to stake more than max stake
	staker.AddDelegationErrors(validator.ID, 1, 255, 10, "total stake would exceed maximum")
}

func Test_AddDelegator_ValidatorNotFound(t *testing.T) {
	staker, _ := newStakerV2(t, 75, 101, true)

	staker.AddDelegationErrors(thor.Address{}, delegationStake(), 255, 10, "validation does not exist")
}

func Test_AddDelegator_ManyValidators(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := delegationStake()

	for _, validator := range validators {
		staker.AddDelegation(validator.ID, stake, 255, 10)
		staker.AssertAggregation(validator.ID).PendingVET(stake)
	}
}

func Test_AddDelegator_ZeroMultiplier(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	staker.AddDelegationErrors(validators[0].ID, delegationStake(), 0, 10, "multiplier cannot be 0")
}

func Test_Delegator_DisableAutoRenew_PendingLocked(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id := staker.AddDelegation(validator.ID, stake, 255, 10)
	staker.AssertAggregation(validator.ID).PendingVET(stake)

	// Then the delegation can't signal an exit until it has started
	staker.SignalDelegationExitErrors(id, 20, "delegation has not started yet")
	staker.Housekeep(validator.Period)
	staker.SignalDelegationExit(id, validator.Period)
	staker.AssertAggregation(validator.ID).LockedVET(stake).ExitingVET(stake)

	// When the staking period is completed
	staker.Housekeep(validator.Period)
	staker.AssertAggregation(validator.ID).LockedVET(0).ExitingVET(0)

	// And the delegation should be withdrawable
	staker.WithdrawDelegation(id, stake, validator.Period*2)
}

func Test_QueuedDelegator_Withdraw_NonAutoRenew(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew disabled
	validator := validators[0]
	stake := delegationStake()
	id := staker.AddDelegation(validator.ID, stake, 255, 10)

	// When the delegation withdraws
	staker.WithdrawDelegation(id, stake, 10)

	// Then the aggregation should be removed
	staker.AssertAggregation(validator.ID).PendingVET(0).PendingWeight(0)

	// And the delegation should be removed
	staker.AssertDelegation(id).Stake(0)
}

func Test_QueuedDelegator_Withdraw_AutoRenew(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id := staker.AddDelegation(validator.ID, stake, 255, 10)

	// When the delegation withdraws before the next staking period
	staker.WithdrawDelegation(id, stake, 10)

	// Then the aggregation should be removed
	staker.AssertAggregation(validator.ID).PendingVET(0).PendingWeight(0)

	// And the delegation should be removed
	staker.AssertDelegation(id).Stake(0)
}

func Test_Delegator_DisableAutoRenew_InAStakingPeriod(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()

	id := staker.AddDelegation(validator.ID, stake, 255, 10)
	staker.AssertQueuedVET(stake)
	staker.AssertAggregation(validator.ID).LockedVET(0).PendingVET(stake)

	// And the first staking period has occurred
	staker.Housekeep(validator.Period)
	staker.AssertAggregation(validator.ID).LockedVET(stake).PendingVET(0)

	// When the delegation disables auto renew
	staker.SignalDelegationExit(id, 129600)

	// Then the stake is moved to cooldown
	staker.AssertAggregation(validator.ID).LockedVET(stake).ExitingVET(stake).PendingVET(0)

	// And the funds should be withdrawable after the next iteration
	staker.Housekeep(2 * validator.Period)
	staker.AssertAggregation(validator.ID).LockedVET(0).ExitingVET(0).PendingVET(0)
}

func Test_Delegator_AutoRenew_ValidatorExits(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id := staker.AddDelegation(validator.ID, stake, 255, 10)

	// And the first staking period has occurred
	staker.Housekeep(validator.Period)
	staker.AssertAggregation(validator.ID).LockedVET(stake).PendingVET(0)

	// When the validator signals an exit
	staker.SignalExit(validator.ID, validator.Endorser, validator.Period*1)

	// And the next staking period is over
	staker.Housekeep(validator.Period * 2)
	staker.AssertAggregation(validator.ID).LockedVET(0).ExitingVET(0).PendingVET(0)

	// Then the funds should be withdrawable
	staker.WithdrawDelegation(id, stake, 20)
}

func Test_Delegator_WithdrawWhilePending(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id := staker.AddDelegation(validator.ID, stake, 255, 10)

	// When the delegation withdraws
	staker.WithdrawDelegation(id, stake, 10)

	// Then the aggregation should be removed
	staker.AssertAggregation(validator.ID).PendingVET(0).PendingWeight(0)

	// And the delegation should be removed
	delegation := staker.GetDelegation(id)
	assert.False(t, delegation == nil)
}

func Test_Delegator_ID_ShouldBeIncremental(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	// Given the staker contract is setup
	validator := validators[0]
	stake := uint64(100)

	id := staker.AddDelegation(validator.ID, stake, 255, 10)

	for range 100 {
		nextID := staker.AddDelegation(validator.ID, stake, 255, 10)
		prev := big.NewInt(0).SetBytes(id.Bytes())
		next := big.NewInt(0).SetBytes(nextID.Bytes())
		assert.Equal(t, prev.Add(prev, big.NewInt(1)), next)
		id = nextID
	}
}

func Test_Delegator_Queued_Weight(t *testing.T) {
	staker, validations := newDelegationStaker(t)
	totalStaked := uint64(0)
	for _, validation := range validations {
		totalStaked += validation.LockedVET
	}

	validatorStake := uint64(25000000)
	stake := uint64(100)

	lockedVetBefore, lockedWeightBefore := staker.LockedStake()
	queuedVetBefore := staker.QueuedStake()

	assert.Equal(t, totalStaked, lockedVetBefore)
	assert.Equal(t, lockedVetBefore, lockedWeightBefore)
	assert.Equal(t, uint64(0), queuedVetBefore)

	node := datagen.RandAddress()
	endorser := datagen.RandAddress()
	staker.AddValidation(node, endorser, uint32(360)*24*15, validatorStake)

	validator := staker.GetValidation(node)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	staker.AddDelegation(node, stake, 255, 10)

	lockedVetAfter, lockedWeightAfter := staker.LockedStake()
	queuedVetAfter := staker.QueuedStake()

	assert.Equal(t, lockedVetBefore, lockedVetAfter)
	assert.Equal(t, lockedWeightBefore, lockedWeightAfter)
	assert.Equal(t, validatorStake+stake, queuedVetAfter)
}

func Test_Delegator_Queued_Weight_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	validatorAddr := datagen.RandAddress()
	staker.AddValidation(validatorAddr, validatorAddr, uint32(360)*24*15, MinStakeVET)

	delegationStake := MinStakeVET / 4
	delegationID := staker.AddDelegation(validatorAddr, delegationStake, 255, 10)
	staker.AssertQueuedVET(MinStakeVET + delegationStake)

	staker.WithdrawDelegation(delegationID, delegationStake, 10)
	staker.AssertQueuedVET(MinStakeVET)
}

func Test_Delegator_Queued_Weight_MultipleDelegations_Withdraw(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]
	stake1 := MinStakeVET
	stake2 := MinStakeVET / 2

	initialQueuedVET := staker.QueuedStake()

	id1 := staker.AddDelegation(validator.ID, stake1, 200, 10)
	id2 := staker.AddDelegation(validator.ID, stake2, 150, 10)

	afterAddQueuedVET := staker.QueuedStake()
	assert.Equal(t, initialQueuedVET+stake1+stake2, afterAddQueuedVET)

	staker.WithdrawDelegation(id1, stake1, 10)
	afterWithdraw1QueuedVET := staker.QueuedStake()
	assert.Equal(t, initialQueuedVET+stake2, afterWithdraw1QueuedVET)

	staker.WithdrawDelegation(id2, stake2, 10)
	afterWithdraw2QueuedVET := staker.QueuedStake()
	assert.Equal(t, initialQueuedVET, afterWithdraw2QueuedVET)
}

func Test_Delegations_EnableAutoRenew_MatchStakeReached(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]
	maxStake := MaxStakeVET - validator.LockedVET

	// Add a delegation with auto renew enabled
	delegationID := staker.AddDelegation(validator.ID, maxStake, 255, 10)

	// Should be pending
	delegation1 := staker.GetDelegation(delegationID)
	validation := staker.GetValidation(validator.ID)

	started, err := delegation1.Started(validation, 10)
	assert.NoError(t, err)
	assert.False(t, started)

	// Delegation should become active
	staker.Housekeep(validator.Period)
	delegation1 = staker.GetDelegation(delegationID)
	validation = staker.GetValidation(validator.ID)
	started, err = delegation1.Started(validation, 129600)
	assert.NoError(t, err)
	assert.True(t, started)
	//
	//// Enable auto renew for delegation. Should be possible since the max stake won't be reached.
	//assert.NoError(t, staker.UpdateDelegationAutoRenew(delegationID, true))
	//// Immediately turn it off again for the test
	//assert.NoError(t, staker.UpdateDelegationAutoRenew(delegationID, false))
	//
	//// Add a new delegation. It should be possible to add the delegation since the previous one is due to withdraw.
	//_, err = staker.AddDelegation(validator.ID, maxStake, true, 255)
	//assert.NoError(t, err)
	//
	//// Enable auto renew for the first delegation - should fail since the presence of other delegator's exceeds max stake
	//assert.ErrorContains(t, staker.UpdateDelegationAutoRenew(delegationID, true), "validation's next period stake exceeds max stake")
}

func TestStaker_DelegationExitingVET(t *testing.T) {
	staker, totalStake := newStakerV2(t, 1, 1, true)

	firstActive, _ := staker.FirstActive()
	staker.AssertLockedVET(totalStake, totalStake).AssertQueuedVET(0)
	staker.AssertValidation(firstActive).LockedVET(totalStake).Weight(totalStake)

	delStake := uint64(1000)
	delegationID := staker.AddDelegation(firstActive, delStake, 200, 10)

	delegation := staker.GetDelegation(delegationID)
	validator := staker.GetValidation(firstActive)
	started, err := delegation.Started(validator, 10)
	assert.NoError(t, err)
	assert.False(t, started)

	staker.AssertLockedVET(totalStake, totalStake).AssertQueuedVET(delStake)

	staker.Housekeep(thor.MediumStakingPeriod())

	delegation = staker.GetDelegation(delegationID)
	validator = staker.GetValidation(firstActive)
	started, err = delegation.Started(validator, 129600)
	assert.NoError(t, err)
	assert.True(t, started)

	staker.SignalDelegationExit(delegationID, 129600)
	staker.SignalExit(firstActive, validator.Endorser, 129600)

	staker.Housekeep(thor.MediumStakingPeriod() * 2)

	staker.AssertLockedVET(0, 0).AssertQueuedVET(0)
	staker.AssertTotals(firstActive, &validation.Totals{
		TotalLockedStake:  0,
		TotalLockedWeight: 0,
		TotalQueuedStake:  0,
		TotalExitingStake: 0,
		NextPeriodWeight:  0,
	})
}

func TestStaker_DelegationWithdrawPending(t *testing.T) {
	staker, totalStake := newStakerV2(t, 1, 1, true)

	staker.AssertLockedVET(totalStake, totalStake).AssertQueuedVET(0)

	firstActive, validator := staker.FirstActive()
	assert.Equal(t, validator.LockedVET, validator.Weight)

	delStake := uint64(1000)
	delegationID := staker.AddDelegation(firstActive, delStake, 200, 10)

	delegation := staker.GetDelegation(delegationID)
	started, err := delegation.Started(validator, 10)
	assert.NoError(t, err)
	assert.False(t, started)

	staker.AssertLockedVET(totalStake, totalStake).AssertQueuedVET(delStake)
	staker.WithdrawDelegation(delegationID, delStake, 10)
	staker.Housekeep(thor.MediumStakingPeriod())

	_, validator = staker.FirstActive()
	delegation = staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), delegation.Stake)
	assert.Nil(t, delegation.LastIteration)

	isLocked, err := delegation.IsLocked(validator, 20)
	assert.NoError(t, err)
	assert.False(t, isLocked)
}
