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

func newDelegationStaker(t *testing.T) (*Staker, []*testValidators) {
	staker, _ := newStaker(t, 75, 101, true)
	validations := make([]*testValidators, 0)
	err := staker.validationService.LeaderGroupIterator(func(validatorID thor.Address, validation *validation.Validation) error {
		validations = append(validations, &testValidators{
			ID:         validatorID,
			Validation: validation,
		})
		return nil
	})
	assert.NoError(t, err)
	return staker, validations
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

	id := big.NewInt(0)
	validatorID := validators[0].ID

	newTestSequence(t, staker).
		AddDelegation(validatorID, stake, 255, id, 10)

	assertDelegation(t, staker, id).
		IsStarted(false).
		LastIteration(nil)

	weightedStake := stakes.NewWeightedStakeWithMultiplier(stake, 255)

	assertAggregation(t, staker, validatorID).
		PendingVET(stake).
		PendingWeight(weightedStake.Weight)
}

func Test_AddDelegator_StakeRange(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	// should NOT be able to stake greater than max stake
	_, err := staker.AddDelegation(validators[1].ID, MaxStakeVET, 255, 10)
	assert.ErrorContains(t, err, "total stake would exceed maximum")

	// should be able stake 1 VET
	id1, err := staker.AddDelegation(validators[2].ID, 1, 255, 10)
	assert.NoError(t, err)
	delegation, _, err := staker.GetDelegation(id1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), delegation.Stake)
	aggregation, err := staker.aggregationService.GetAggregation(validators[2].ID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), aggregation.Pending.VET)

	// should be able stake for all remaining space
	validator := validators[3]
	validation, err := staker.GetValidation(validator.ID)
	assert.NoError(t, err)
	valNextPeriodTVL, err := validation.NextPeriodTVL()
	assert.NoError(t, err)
	remaining := MaxStakeVET - valNextPeriodTVL
	_, err = staker.AddDelegation(validator.ID, remaining, 255, 10)
	assert.NoError(t, err)

	// should not be able to stake more than max stake
	_, err = staker.AddDelegation(validator.ID, 1, 255, 10)
	assert.ErrorContains(t, err, "total stake would exceed maximum")
}

func Test_AddDelegator_ValidatorNotFound(t *testing.T) {
	staker, _ := newStaker(t, 75, 101, true)

	_, err := staker.AddDelegation(thor.Address{}, delegationStake(), 255, 10)
	assert.ErrorContains(t, err, "validation does not exist")
}

func Test_AddDelegator_ManyValidators(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := delegationStake()

	for _, validator := range validators {
		_, err := staker.AddDelegation(validator.ID, stake, 255, 10)
		assert.NoError(t, err)
		aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
		assert.NoError(t, err)
		assert.Equal(t, aggregation.Pending.VET, stake)
	}
}

func Test_AddDelegator_ZeroMultiplier(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	_, err := staker.AddDelegation(validators[0].ID, delegationStake(), 0, 10)
	assert.ErrorContains(t, err, "multiplier cannot be 0")
}

func Test_Delegator_DisableAutoRenew_PendingLocked(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.Pending.VET)

	// Then the delegation can't signal an exit until it has started
	assert.ErrorContains(t, staker.SignalDelegationExit(id, 20), "delegation has not started yet")
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	assert.NoError(t, staker.SignalDelegationExit(id, validator.Period))
	aggregation, err = staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.Locked.VET)  // This is the only delegator
	assert.Equal(t, stake, aggregation.Exiting.VET) // ExitingVET takes effect in next staking period

	// When the staking period is completed
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err = staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), aggregation.Locked.VET)  // LockedVET should be 0
	assert.Equal(t, uint64(0), aggregation.Exiting.VET) // WithdrawableVET should be equal to the stake

	// And the delegation should be withdrawable
	amount, err := staker.WithdrawDelegation(id, validator.Period*2)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)
}

func Test_QueuedDelegator_Withdraw_NonAutoRenew(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew disabled
	validator := validators[0]
	stake := delegationStake()
	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)

	// When the delegation withdraws
	amount, err := staker.WithdrawDelegation(id, 10)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the aggregation should be removed
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), aggregation.Pending.VET)
	assert.Equal(t, uint64(0), aggregation.Pending.Weight)

	// And the delegation should be removed
	delegation, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.False(t, delegation == nil)
}

func Test_QueuedDelegator_Withdraw_AutoRenew(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)

	// When the delegation withdraws before the next staking period
	amount, err := staker.WithdrawDelegation(id, 10)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the aggregation should be removed
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), aggregation.Pending.VET)
	assert.Equal(t, uint64(0), aggregation.Pending.Weight)

	// And the delegation should be removed
	delegation, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.False(t, delegation == nil)
}

func Test_Delegator_DisableAutoRenew_InAStakingPeriod(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	validation, err := staker.GetValidation(validator.ID)
	assert.NoError(t, err)
	validationStake := validation.LockedVET

	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)

	_, queuedVet, _, _, err := staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVet)

	// And the first staking period has occurred
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.Locked.VET)
	_, queuedVet, _, _, err = staker.Stakes()

	assert.NoError(t, err)
	assert.Equal(t, uint64(0), queuedVet)

	// When the delegation disables auto renew
	assert.NoError(t, staker.SignalDelegationExit(id, 129600))
	// Then the stake is moved to cooldown
	aggregation, err = staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.Locked.VET)
	_, queuedVet, _, _, err = staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), queuedVet)

	// And the funds should be withdrawable after the next iteration
	_, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	_, err = staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	validation, err = staker.GetValidation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, validationStake, validation.LockedVET)
}

func Test_Delegator_AutoRenew_ValidatorExits(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)

	// And the first staking period has occurred
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.Locked.VET)

	// When the validator signals an exit
	assert.NoError(t, staker.SignalExit(validator.ID, validator.Endorser, validator.Period*1))

	// And the next staking period is over
	_, err = staker.Housekeep(validator.Period * 2)
	assert.NoError(t, err)
	_, err = staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)

	// Then the funds should be withdrawable
	amount, err := staker.WithdrawDelegation(id, 20)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)
}

func Test_Delegator_WithdrawWhilePending(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := delegationStake()
	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)

	// When the delegation withdraws
	amount, err := staker.WithdrawDelegation(id, 10)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the aggregation should be removed
	aggregation, err := staker.aggregationService.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), aggregation.Pending.VET)

	// And the delegation should be removed
	delegation, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.False(t, delegation == nil)
}

func Test_Delegator_ID_ShouldBeIncremental(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	// Given the staker contract is setup
	validator := validators[0]
	stake := uint64(100)

	id, err := staker.AddDelegation(validator.ID, stake, 255, 10)
	assert.NoError(t, err)

	for range 100 {
		nextID, err := staker.AddDelegation(validator.ID, stake, 255, 10)
		assert.NoError(t, err)
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

	lockedVetBefore, lockedWeightBefore, err := staker.LockedStake()
	assert.NoError(t, err)
	_, queuedVetBefore, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, totalStaked, lockedVetBefore)
	assert.Equal(t, lockedVetBefore, lockedWeightBefore)
	assert.Equal(t, uint64(0), queuedVetBefore)

	node := datagen.RandAddress()
	endorser := datagen.RandAddress()
	err = staker.AddValidation(node, endorser, uint32(360)*24*15, validatorStake)
	assert.NoError(t, err)

	validator, err := staker.GetValidation(node)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusQueued, validator.Status)

	_, err = staker.AddDelegation(node, stake, 255, 10)
	assert.NoError(t, err)

	lockedVetAfter, lockedWeightAfter, err := staker.LockedStake()
	assert.NoError(t, err)
	_, queuedVetAfter, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, lockedVetBefore, lockedVetAfter)
	assert.Equal(t, lockedWeightBefore, lockedWeightAfter)
	assert.Equal(t, validatorStake+stake, queuedVetAfter)
}

func Test_Delegator_Queued_Weight_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	validatorAddr := datagen.RandAddress()
	err := staker.AddValidation(validatorAddr, validatorAddr, uint32(360)*24*15, MinStakeVET)
	assert.NoError(t, err)

	_, initialQueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	delegationStake := MinStakeVET / 4
	delegationID, err := staker.AddDelegation(validatorAddr, delegationStake, 255, 10)
	assert.NoError(t, err)

	_, afterAddQueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET+delegationStake, afterAddQueuedVET)

	withdrawnAmount, err := staker.WithdrawDelegation(delegationID, 10)
	assert.NoError(t, err)
	assert.Equal(t, delegationStake, withdrawnAmount)

	_, afterWithdrawQueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET, afterWithdrawQueuedVET)
}

func Test_Delegator_Queued_Weight_MultipleDelegations_Withdraw(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]
	stake1 := MinStakeVET
	stake2 := MinStakeVET / 2

	_, initialQueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	id1, err := staker.AddDelegation(validator.ID, stake1, 200, 10)
	assert.NoError(t, err)

	id2, err := staker.AddDelegation(validator.ID, stake2, 150, 10)
	assert.NoError(t, err)

	_, afterAddQueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET+stake1+stake2, afterAddQueuedVET)

	withdrawnAmount1, err := staker.WithdrawDelegation(id1, 10)
	assert.NoError(t, err)
	assert.Equal(t, stake1, withdrawnAmount1)

	_, afterWithdraw1QueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET+stake2, afterWithdraw1QueuedVET)

	withdrawnAmount2, err := staker.WithdrawDelegation(id2, 10)
	assert.NoError(t, err)
	assert.Equal(t, stake2, withdrawnAmount2)

	_, afterWithdraw2QueuedVET, _, _, err := staker.Stakes()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET, afterWithdraw2QueuedVET)
}

func Test_Delegations_EnableAutoRenew_MatchStakeReached(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]
	maxStake := MaxStakeVET - validator.LockedVET

	// Add a delegation with auto renew enabled
	delegationID, err := staker.AddDelegation(validator.ID, maxStake, 255, 10)
	assert.NoError(t, err)

	// Should be pending
	delegation1, _, err := staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	validation, err := staker.GetValidation(validator.ID)
	assert.NoError(t, err)

	started, err := delegation1.Started(validation, 10)
	assert.NoError(t, err)
	assert.False(t, started)

	// Delegation should become active
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	delegation1, _, err = staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	validation, err = staker.GetValidation(validator.ID)
	assert.NoError(t, err)
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
	staker, totalStake := newStaker(t, 1, 1, true)

	firstActive, err := staker.FirstActive()
	assert.NoError(t, err)

	stake, weight, err := staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, totalStake, stake)
	assert.Equal(t, totalStake, weight)
	_, qStake, _, _, err := staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	validator, err := staker.GetValidation(firstActive)
	assert.NoError(t, err)
	assert.Equal(t, validator.LockedVET, validator.Weight)

	delStake := uint64(1000)
	delegationID, err := staker.AddDelegation(firstActive, delStake, 200, 10)
	assert.NoError(t, err)

	delegation, validation, err := staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	started, err := delegation.Started(validation, 10)
	assert.NoError(t, err)
	assert.False(t, started)

	stake, weight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, totalStake, stake)
	assert.Equal(t, totalStake, weight)
	_, qStake, _, _, err = staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, delStake, qStake)

	_, err = staker.Housekeep(thor.MediumStakingPeriod())
	assert.NoError(t, err)

	delegation, validation, err = staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	started, err = delegation.Started(validation, 129600)
	assert.NoError(t, err)
	assert.True(t, started)

	assert.NoError(t, staker.SignalDelegationExit(delegationID, 129600))
	assert.NoError(t, staker.SignalExit(firstActive, validation.Endorser, 129600))

	_, err = staker.Housekeep(thor.MediumStakingPeriod() * 2)
	assert.NoError(t, err)

	lVet, lWeight, err := staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), lVet)
	assert.Equal(t, uint64(0), lWeight)

	_, qVet, _, _, err := staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVet)

	total, err := staker.GetValidationTotals(firstActive)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), total.TotalLockedStake)
	assert.Equal(t, uint64(0), total.TotalLockedWeight)
	assert.Equal(t, uint64(0), total.TotalQueuedStake)
	assert.Equal(t, uint64(0), total.TotalExitingStake)
	assert.Equal(t, uint64(0), total.NextPeriodWeight)
}

func TestStaker_DelegationWithdrawPending(t *testing.T) {
	staker, totalStake := newStaker(t, 1, 1, true)

	firstActive, err := staker.FirstActive()
	assert.NoError(t, err)

	stake, weight, err := staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, totalStake, stake)
	assert.Equal(t, totalStake, weight)
	_, qStake, _, _, err := staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qStake)

	validator, err := staker.GetValidation(firstActive)
	assert.NoError(t, err)
	assert.Equal(t, validator.LockedVET, validator.Weight)

	delStake := uint64(1000)
	delegationID, err := staker.AddDelegation(firstActive, delStake, 200, 10)
	assert.NoError(t, err)

	delegation, validation, err := staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	started, err := delegation.Started(validation, 10)
	assert.NoError(t, err)
	assert.False(t, started)

	stake, weight, err = staker.LockedStake()
	assert.NoError(t, err)
	assert.Equal(t, totalStake, stake)
	assert.Equal(t, totalStake, weight)
	_, qStake, _, _, err = staker.Stakes()
	assert.NoError(t, err)
	assert.Equal(t, delStake, qStake)

	withdrawnStake, err := staker.WithdrawDelegation(delegationID, 10)
	assert.NoError(t, err)
	assert.Equal(t, delStake, withdrawnStake)

	_, err = staker.Housekeep(thor.MediumStakingPeriod())
	assert.NoError(t, err)

	delegation, validation, err = staker.GetDelegation(delegationID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), delegation.Stake)
	assert.Nil(t, delegation.LastIteration)

	isLocked, err := delegation.IsLocked(validation, 20)
	assert.NoError(t, err)
	assert.False(t, isLocked)
}
