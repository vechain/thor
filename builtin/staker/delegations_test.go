// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

type testValidators struct {
	ID thor.Bytes32
	*Validation
}

func newDelegationStaker(t *testing.T) (*Staker, []*testValidators) {
	staker, _ := newStaker(t, 75, 101, true)
	validations := make([]*testValidators, 0)
	err := staker.validations.LeaderGroupIterator(func(validatorID thor.Bytes32, validation *Validation) error {
		validations = append(validations, &testValidators{
			ID:         validatorID,
			Validation: validation,
		})
		return nil
	})
	assert.NoError(t, err)
	return staker, validations
}

func Test_IsLocked(t *testing.T) {
	t.Run("Completed Staking Periods", func(t *testing.T) {
		last := uint32(2)
		d := &Delegation{
			FirstIteration: 2,
			LastIteration:  &last,
			Stake:          big.NewInt(1),
			Multiplier:     255,
		}

		v := &Validation{
			Status:             StatusActive,
			CompleteIterations: 2,
		}

		assert.False(t, d.IsLocked(v), "should not be locked when complete iterations is equal to last iteration")
	})

	t.Run("Incomplete Staking Periods", func(t *testing.T) {
		last := uint32(5)
		d := &Delegation{
			FirstIteration: 2,
			LastIteration:  &last,
			Stake:          big.NewInt(1),
			Multiplier:     255,
		}

		v := &Validation{
			Status:             StatusActive,
			CompleteIterations: 3,
		}

		assert.True(t, d.IsLocked(v), "should be locked when first is less than current and last is greater")
	})

	t.Run("Delegation Not Started", func(t *testing.T) {
		last := uint32(6)
		d := &Delegation{
			FirstIteration: 5,
			LastIteration:  &last,
			Stake:          big.NewInt(1),
			Multiplier:     255,
		}

		v := &Validation{
			Status:             StatusActive,
			CompleteIterations: 3,
		}

		assert.False(t, d.IsLocked(v), "should not be locked if delegation has not started yet")
	})
	t.Run("Staker is Queued", func(t *testing.T) {
		d := &Delegation{
			FirstIteration: 1,
			LastIteration:  nil,
			Stake:          big.NewInt(1),
			Multiplier:     255,
		}

		v := &Validation{
			Status:             StatusQueued,
			CompleteIterations: 0,
		}

		assert.False(t, d.IsLocked(v), "should not be locked when validation status is queued")
	})

	t.Run("Exit block not defined", func(t *testing.T) {
		d := &Delegation{
			FirstIteration: 1,
			LastIteration:  nil,
			Stake:          big.NewInt(1),
			Multiplier:     255,
		}

		v := &Validation{
			Status:             StatusActive,
			CompleteIterations: 0,
		}

		assert.True(t, d.IsLocked(v), "should be locked when last iteration is nil and first equals current")
	})
}

func Test_AddDelegator_AutoRenew(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := big.NewInt(0).Set(minStake)

	// Auto Renew == true
	validator := validators[0]
	id1, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)
	assert.False(t, id1.IsZero())
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, aggregation.PendingRecurringVET, stake)
	delegation, _, err := staker.GetDelegation(id1)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.Stake)
	assert.Equal(t, uint8(255), delegation.Multiplier)
	assert.Equal(t, uint32(2), delegation.FirstIteration)
	assert.Nil(t, delegation.LastIteration) // auto renew, so exit iteration is nil

	// Auto Renew == false
	validator = validators[1]
	id2, err := staker.AddDelegation(validator.ID, stake, false, 255)
	assert.NoError(t, err)
	aggregation2, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, aggregation2.PendingOneTimeVET, stake)

	delegation, _, err = staker.GetDelegation(id2)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.Stake)
	assert.Equal(t, uint8(255), delegation.Multiplier)
	assert.Equal(t, uint32(2), delegation.FirstIteration)
	expectedExit := uint32(2)
	assert.Equal(t, &expectedExit, delegation.LastIteration) // auto renew, so we know when it will exit
}

func Test_AddDelegator_StakeRange(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	// should NOT be able to stake 0 VET
	_, err := staker.AddDelegation(validator.ID, big.NewInt(0), true, 255)
	assert.ErrorContains(t, err, "stake must be greater than 0")

	// should NOT be able to stake greater than max stake
	_, err = staker.AddDelegation(validator.ID, maxStake, true, 255)
	assert.ErrorContains(t, err, "validation's next period stake exceeds max stake")

	// should be able stake 1 VET
	_, err = staker.AddDelegation(validator.ID, big.NewInt(1), true, 255)
	assert.NoError(t, err)

	// should be able stake for all remaining space
	validator = validators[1]
	validation, err := staker.Get(validator.ID)
	assert.NoError(t, err)
	remaining := big.NewInt(0).Sub(maxStake, validation.NextPeriodStakes(newAggregation()))
	_, err = staker.AddDelegation(validator.ID, remaining, true, 255)
	assert.NoError(t, err)
}

func Test_AddDelegator_ValidatorNotFound(t *testing.T) {
	staker, _ := newStaker(t, 75, 101, true)

	_, err := staker.AddDelegation(datagen.RandomHash(), RandomStake(), true, 255)
	assert.ErrorContains(t, err, "validation not found")
}

func Test_AddDelegator_ManyValidators(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := RandomStake()

	for _, validator := range validators {
		_, err := staker.AddDelegation(validator.ID, stake, true, 255)
		assert.NoError(t, err)
		aggregation, err := staker.storage.GetAggregation(validator.ID)
		assert.NoError(t, err)
		assert.Equal(t, aggregation.PendingRecurringVET, stake)
	}
}

func Test_AddDelegator_ZeroMultiplier(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	_, err := staker.AddDelegation(validators[0].ID, RandomStake(), true, 0)
	assert.ErrorContains(t, err, "multiplier cannot be 0")
}

func Test_Delegator_DisableAutoRenew_PendingLocked(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.PendingRecurringVET)

	// When the delegation disables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, false))

	// Then the stake is moved to pending cooldown
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.PendingOneTimeVET)

	// When the delegation becomes active
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	// Then the stake is not withdrawable yet
	_, err = staker.WithdrawDelegation(id)
	assert.ErrorContains(t, err, "delegation is not eligible for withdraw")

	// And the staking period is completed
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	// step 2: end the first iteration
	_, _, err = staker.Housekeep(2 * validator.Period)

	assert.NoError(t, err)
	amount, err := staker.WithdrawDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)
}

func Test_QueuedDelegator_Withdraw_NonAutoRenew(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew disabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegation(validator.ID, stake, false, 255)
	assert.NoError(t, err)

	// When the delegation withdraws
	amount, err := staker.WithdrawDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the aggregation should be removed
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), aggregation.PendingOneTimeVET)

	// And the delegation should be removed
	delegation, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.False(t, delegation.IsEmpty())
}

func Test_Delegator_DisableAutoRenew_InAStakingPeriod(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()

	id, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	weight := big.NewInt(0).Mul(stake, big.NewInt(255))
	weight = big.NewInt(0).Quo(weight, big.NewInt(100))

	queuedVet, queuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVet)
	assert.Equal(t, weight, queuedWeight)

	// And the first staking period has occurred
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.CurrentRecurringVET)
	queuedVet, queuedWeight, err = staker.QueuedStake()

	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), queuedVet.String())
	assert.Equal(t, big.NewInt(0).String(), queuedWeight.String())

	// When the delegation disables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, false))
	// Then the stake is moved to cooldown
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.CurrentOneTimeVET)
	queuedVet, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), queuedVet.String())
	assert.Equal(t, big.NewInt(0).String(), queuedWeight.String())

	// And the funds should be withdrawable after the next iteration
	_, _, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.WithdrawableVET)
}

func Test_Delegator_EnableAutoRenew_PendingLocked(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew disabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegation(validator.ID, stake, false, 255)
	assert.NoError(t, err)
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.PendingOneTimeVET)

	// When the delegation enables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, true))

	// Then the stake is moved to pending locked
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.PendingRecurringVET)

	// And the funds should NOT be withdrawable after 1 iteration
	// step 1: start the first iteration
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	_, err = staker.WithdrawDelegation(id)
	assert.ErrorContains(t, err, "delegation is not eligible for withdraw")

	// step 2: end the first iteration
	_, _, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	_, err = staker.WithdrawDelegation(id)
	assert.ErrorContains(t, err, "delegation is not eligible for withdraw")
}

func Test_Delegator_EnableAutoRenew_InAStakingPeriod(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew disabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegation(validator.ID, stake, false, 255)
	assert.NoError(t, err)

	// And the first staking period has occurred
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.CurrentOneTimeVET)

	// When the delegation enables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, true))

	// Then the stake is moved to locked
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.CurrentRecurringVET)

	// And the funds should NOT be withdrawable after 1 iteration
	_, _, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	_, err = staker.WithdrawDelegation(id)
	assert.ErrorContains(t, err, "delegation is not eligible for withdraw")
}

func Test_Delegator_AutoRenew_ValidatorExits(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	// And the first staking period has occurred
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.CurrentRecurringVET)

	// When the validator signals an exit
	assert.NoError(t, staker.UpdateAutoRenew(validator.Endorsor, validator.ID, false))

	// And the next staking period is over
	_, _, err = staker.Housekeep(validator.Period * 2)
	assert.NoError(t, err)
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.WithdrawableVET)

	// Then the funds should be withdrawable
	amount, err := staker.WithdrawDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)
}

func Test_Delegator_WithdrawWhilePending(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegation is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	// When the delegation withdraws
	amount, err := staker.WithdrawDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the aggregation should be removed
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), aggregation.PendingRecurringVET)

	// And the delegation should be removed
	delegation, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.False(t, delegation.IsEmpty())
}

func Test_Delegator_ID_ShouldBeIncremental(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	// Given the staker contract is setup
	validator := validators[0]
	stake := big.NewInt(100)

	id, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	for range 100 {
		nextID, err := staker.AddDelegation(validator.ID, stake, true, 255)
		assert.NoError(t, err)
		prev := big.NewInt(0).SetBytes(id.Bytes())
		next := big.NewInt(0).SetBytes(nextID.Bytes())
		assert.Equal(t, prev.Add(prev, big.NewInt(1)), next)
		id = nextID
	}
}

func Test_Delegator_Queued_Weight(t *testing.T) {
	staker, validations := newDelegationStaker(t)
	totalStaked := big.NewInt(0)
	for _, validation := range validations {
		totalStaked = totalStaked.Add(totalStaked, validation.LockedVET)
	}

	validatorStake := big.NewInt(0).Mul(big.NewInt(25000000), big.NewInt(1e18))
	validatorWeight := big.NewInt(0).Mul(validatorStake, big.NewInt(2))
	stake := big.NewInt(100)

	lockedVetBefore, lockedWeightBefore, err := staker.LockedVET()
	assert.NoError(t, err)
	queuedVetBefore, queuedWeightBefore, err := staker.QueuedStake()
	assert.NoError(t, err)

	assert.Equal(t, totalStaked, lockedVetBefore)
	assert.Equal(t, big.NewInt(0).Mul(lockedVetBefore, big.NewInt(2)), lockedWeightBefore)
	assert.Equal(t, big.NewInt(0).String(), queuedVetBefore.String())
	assert.Equal(t, big.NewInt(0).String(), queuedWeightBefore.String())

	nodeMaster := datagen.RandAddress()
	endorsor := datagen.RandAddress()
	id, err := staker.AddValidator(endorsor, nodeMaster, uint32(360)*24*15, validatorStake, true, 0)
	assert.NoError(t, err)

	validator, err := staker.Get(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, validator.Status)

	_, err = staker.AddDelegation(id, stake, true, 255)
	assert.NoError(t, err)

	lockedVetAfter, lockedWeightAfter, err := staker.LockedVET()
	assert.NoError(t, err)
	queuedVetAfter, queuedWeightAfter, err := staker.QueuedStake()
	assert.NoError(t, err)

	delegatorWeight := big.NewInt(0).Mul(stake, big.NewInt(255))
	delegatorWeight = delegatorWeight.Div(delegatorWeight, big.NewInt(100))
	assert.Equal(t, lockedVetBefore, lockedVetAfter)
	assert.Equal(t, lockedWeightBefore, lockedWeightAfter)
	assert.Equal(t, big.NewInt(0).Add(validatorStake, stake), queuedVetAfter)
	assert.Equal(t, big.NewInt(0).Add(validatorWeight, delegatorWeight), queuedWeightAfter)
}

func Test_Delegator_Queued_Weight_QueuedValidator_Withdraw(t *testing.T) {
	staker, _ := newStaker(t, 0, 101, false)

	validatorAddr := datagen.RandAddress()
	validatorID, err := staker.AddValidator(validatorAddr, validatorAddr, uint32(360)*24*15, minStake, true, 0)
	assert.NoError(t, err)

	initialQueuedVET, initialQueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	delegationStake := new(big.Int).Div(minStake, big.NewInt(4))
	delegationID, err := staker.AddDelegation(validatorID, delegationStake, true, 255)
	assert.NoError(t, err)

	afterAddQueuedVET, afterAddQueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	expectedWeight := new(big.Int).Mul(delegationStake, big.NewInt(255))
	expectedWeight = new(big.Int).Quo(expectedWeight, big.NewInt(100))

	assert.Equal(t, new(big.Int).Add(initialQueuedVET, delegationStake), afterAddQueuedVET)
	assert.Equal(t, new(big.Int).Add(initialQueuedWeight, expectedWeight), afterAddQueuedWeight)

	withdrawnAmount, err := staker.WithdrawDelegation(delegationID)
	assert.NoError(t, err)
	assert.Equal(t, delegationStake, withdrawnAmount)

	afterWithdrawQueuedVET, afterWithdrawQueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET, afterWithdrawQueuedVET)
	assert.Equal(t, initialQueuedWeight, afterWithdrawQueuedWeight)
}

func Test_Delegator_Queued_Weight_MultipleDelegations_Withdraw(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]
	stake1 := new(big.Int).Set(minStake)
	stake2 := new(big.Int).Div(minStake, big.NewInt(2))

	initialQueuedVET, initialQueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	id1, err := staker.AddDelegation(validator.ID, stake1, true, 200)
	assert.NoError(t, err)

	id2, err := staker.AddDelegation(validator.ID, stake2, false, 150)
	assert.NoError(t, err)

	afterAddQueuedVET, afterAddQueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	expectedWeight1 := new(big.Int).Mul(stake1, big.NewInt(200))
	expectedWeight1 = new(big.Int).Quo(expectedWeight1, big.NewInt(100))
	expectedWeight2 := new(big.Int).Mul(stake2, big.NewInt(150))
	expectedWeight2 = new(big.Int).Quo(expectedWeight2, big.NewInt(100))
	totalExpectedWeight := new(big.Int).Add(expectedWeight1, expectedWeight2)

	assert.Equal(t, new(big.Int).Add(initialQueuedVET, new(big.Int).Add(stake1, stake2)), afterAddQueuedVET)
	assert.Equal(t, new(big.Int).Add(initialQueuedWeight, totalExpectedWeight), afterAddQueuedWeight)

	withdrawnAmount1, err := staker.WithdrawDelegation(id1)
	assert.NoError(t, err)
	assert.Equal(t, stake1, withdrawnAmount1)

	afterWithdraw1QueuedVET, afterWithdraw1QueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	assert.Equal(t, new(big.Int).Add(initialQueuedVET, stake2), afterWithdraw1QueuedVET)
	assert.Equal(t, new(big.Int).Add(initialQueuedWeight, expectedWeight2), afterWithdraw1QueuedWeight)

	withdrawnAmount2, err := staker.WithdrawDelegation(id2)
	assert.NoError(t, err)
	assert.Equal(t, stake2, withdrawnAmount2)

	afterWithdraw2QueuedVET, afterWithdraw2QueuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)

	assert.Equal(t, initialQueuedVET, afterWithdraw2QueuedVET)
	assert.Equal(t, initialQueuedWeight, afterWithdraw2QueuedWeight)
}
