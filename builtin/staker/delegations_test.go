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

func Test_AddDelegator_AutoRenew(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := big.NewInt(0).Set(minStake)

	// Auto Renew == true
	validator := validators[0]
	id1, err := staker.AddDelegation(validator.ID, stake, true, 255)
	assert.NoError(t, err)
	assert.False(t, id1.IsZero())
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, aggregation.LockedVET, stake)
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
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	aggregation2, err := staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, aggregation2.CooldownVET, stake)

	delegation, _, err = staker.GetDelegation(id2)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.Stake)
	assert.Equal(t, uint8(255), delegation.Multiplier)
	assert.Equal(t, uint32(3), delegation.FirstIteration)
	assert.Equal(t, uint32(3), *delegation.LastIteration) // auto renew, so we know when it will exit
}

func Test_AddDelegator_StakeRange(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	// should NOT be able to stake 0 VET
	_, err := staker.AddDelegation(validator.ID, big.NewInt(0), true, 255)
	assert.ErrorContains(t, err, "stake must be greater than 0")

	// should NOT be able to stake greater than max stake
	_, err = staker.AddDelegation(validator.ID, maxStake, true, 255)
	assert.ErrorContains(t, err, "validator's next period stake exceeds max stake")

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
	assert.ErrorContains(t, err, "validator not found")
}

func Test_AddDelegator_ManyValidators(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := RandomStake()

	for _, validator := range validators {
		_, err := staker.AddDelegation(validator.ID, stake, true, 255)
		assert.NoError(t, err)
		aggregation, err := staker.storage.GetAggregation(validator.ID)
		assert.NoError(t, err)
		assert.Equal(t, aggregation.PendingLockedVET, stake)
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
	assert.Equal(t, stake, aggregation.PendingLockedVET)

	// When the delegation disables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, false))

	// Then the stake is moved to pending cooldown
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.PendingCooldownVET)

	// And the delegation becomes active
	// step 1: start the first iteration
	_, _, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	delegation, err := staker.storage.GetDelegation(id)
	assert.NoError(t, err)
	validation, err := staker.storage.GetValidator(validator.ID)
	assert.NoError(t, err)
	assert.True(t, delegation.IsLocked(validation))
	_, err = staker.WithdrawDelegation(id)
	assert.ErrorContains(t, err, "delegation is not eligible for withdraw")
	// step 2: end the first iteration
	_, _, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)

	// And the delegation ends due to auto renew being disabled
	_, _, err = staker.Housekeep(validator.Period * 2)
	assert.NoError(t, err)
	delegation, err = staker.storage.GetDelegation(id)
	assert.NoError(t, err)
	validation, err = staker.storage.GetValidator(validator.ID)
	assert.NoError(t, err)
	assert.False(t, delegation.IsLocked(validation))

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
	assert.Equal(t, big.NewInt(0), aggregation.PendingCooldownVET)

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
	assert.Equal(t, stake, aggregation.LockedVET)
	queuedVet, queuedWeight, err = staker.QueuedStake()

	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), queuedVet.String())
	assert.Equal(t, big.NewInt(0).String(), queuedWeight.String())

	// When the delegation disables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, false))
	// Then the stake is moved to cooldown
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.CooldownVET)
	queuedVet, queuedWeight, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), queuedVet.String())
	assert.Equal(t, big.NewInt(0).String(), queuedWeight.String())

	// And the funds should be withdrawable after the next iteration
	_, _, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.WithdrawVET)
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
	assert.Equal(t, stake, aggregation.PendingCooldownVET)

	// When the delegation enables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, true))

	// Then the stake is moved to pending locked
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.PendingLockedVET)

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
	assert.Equal(t, stake, aggregation.CooldownVET)

	// When the delegation enables auto renew
	assert.NoError(t, staker.UpdateDelegationAutoRenew(id, true))

	// Then the stake is moved to pending locked
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.LockedVET)

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
	assert.Equal(t, stake, aggregation.LockedVET)

	// When the validator signals an exit
	assert.NoError(t, staker.UpdateAutoRenew(validator.Endorsor, validator.ID, false))

	// And the next staking period is over
	_, _, err = staker.Housekeep(validator.Period * 2)
	assert.NoError(t, err)
	aggregation, err = staker.storage.GetAggregation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, aggregation.WithdrawVET)

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
	assert.Equal(t, big.NewInt(0), aggregation.PendingLockedVET)

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
