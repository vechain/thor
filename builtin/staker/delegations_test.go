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
	id1, err := staker.AddDelegator(validator.ID, stake, true, 255)
	assert.NoError(t, err)
	assert.False(t, id1.IsZero())
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, delegation.PendingLockedVET, stake)
	delegator, err := staker.GetDelegator(id1)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegator.Stake)
	assert.Equal(t, uint8(255), delegator.Multiplier)
	assert.Equal(t, uint32(1), delegator.FirstIteration)
	assert.Nil(t, delegator.ExitIteration) // auto renew, so exit iteration is nil

	// Auto Renew == false
	validator = validators[1]
	id2, err := staker.AddDelegator(validator.ID, stake, false, 255)
	assert.NoError(t, err)
	delegation2, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, delegation2.PendingCooldownVET, stake)

	delegator, err = staker.GetDelegator(id2)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegator.Stake)
	assert.Equal(t, uint8(255), delegator.Multiplier)
	assert.Equal(t, uint32(1), delegator.FirstIteration)
	expectedExit := uint32(2)
	assert.Equal(t, &expectedExit, delegator.ExitIteration) // auto renew, so we know when it will exit
}

func Test_AddDelegator_StakeRange(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	validator := validators[0]

	// should NOT be able to stake 0 VET
	_, err := staker.AddDelegator(validator.ID, big.NewInt(0), true, 255)
	assert.ErrorContains(t, err, "stake must be greater than 0")

	// should NOT be able to stake greater than max stake
	_, err = staker.AddDelegator(validator.ID, maxStake, true, 255)
	assert.ErrorContains(t, err, "validator's next period stake exceeds max stake")

	// should be able stake 1 VET
	_, err = staker.AddDelegator(validator.ID, big.NewInt(1), true, 255)
	assert.NoError(t, err)

	// should be able stake for all remaining space
	validator = validators[1]
	validation, err := staker.Get(validator.ID)
	assert.NoError(t, err)
	remaining := big.NewInt(0).Sub(maxStake, validation.NextPeriodStakes(newDelegation()))
	_, err = staker.AddDelegator(validator.ID, remaining, true, 255)
	assert.NoError(t, err)
}

func Test_AddDelegator_ValidatorNotFound(t *testing.T) {
	staker, _ := newStaker(t, 75, 101, true)

	_, err := staker.AddDelegator(datagen.RandomHash(), RandomStake(), true, 255)
	assert.ErrorContains(t, err, "validator not found")
}

func Test_AddDelegator_ManyValidators(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	stake := RandomStake()

	for _, validator := range validators {
		_, err := staker.AddDelegator(validator.ID, stake, true, 255)
		assert.NoError(t, err)
		delegation, err := staker.storage.GetDelegation(validator.ID)
		assert.NoError(t, err)
		assert.Equal(t, delegation.PendingLockedVET, stake)
	}
}

func Test_AddDelegator_ZeroMultiplier(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	_, err := staker.AddDelegator(validators[0].ID, RandomStake(), true, 0)
	assert.ErrorContains(t, err, "multiplier cannot be 0")
}

func Test_Delegator_DisableAutoRenew_PendingLocked(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegator(validator.ID, stake, true, 255)
	assert.NoError(t, err)
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.PendingLockedVET)

	// When the delegator disables auto renew
	assert.NoError(t, staker.UpdateDelegatorAutoRenew(id, false))

	// Then the stake is moved to pending cooldown
	delegation, err = staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.PendingCooldownVET)

	// And the funds should be withdrawable after 1 iteration
	// step 1: start the first iteration
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	_, err = staker.DelegatorWithdrawStake(id)
	assert.ErrorContains(t, err, "delegator is not eligible for withdraw")
	// step 2: end the first iteration
	_, err = staker.Housekeep(2 * validator.Period)

	assert.NoError(t, err)
	amount, err := staker.DelegatorWithdrawStake(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)
}

func Test_QueuedDelegator_Withdraw_NonAutoRenew(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew disabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegator(validator.ID, stake, false, 255)
	assert.NoError(t, err)

	// When the delegator withdraws
	amount, err := staker.DelegatorWithdrawStake(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the delegation should be removed
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), delegation.PendingCooldownVET)

	// And the delegator should be removed
	delegator, err := staker.GetDelegator(id)
	assert.NoError(t, err)
	assert.True(t, delegator.IsEmpty())
}

func Test_Delegator_DisableAutoRenew_InAStakingPeriod(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()

	id, err := staker.AddDelegator(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	queuedVet, err := staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, stake, queuedVet)

	// And the first staking period has occurred
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.LockedVET)
	queuedVet, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), queuedVet.String())

	// When the delegator disables auto renew
	assert.NoError(t, staker.UpdateDelegatorAutoRenew(id, false))
	// Then the stake is moved to cooldown
	delegation, err = staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.CooldownVET)
	queuedVet, err = staker.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), queuedVet.String())

	// And the funds should be withdrawable after the next iteration
	_, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	delegation, err = staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.WithdrawVET)
}

func Test_Delegator_EnableAutoRenew_PendingLocked(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew disabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegator(validator.ID, stake, false, 255)
	assert.NoError(t, err)
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.PendingCooldownVET)

	// When the delegator enables auto renew
	assert.NoError(t, staker.UpdateDelegatorAutoRenew(id, true))

	// Then the stake is moved to pending locked
	delegation, err = staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.PendingLockedVET)

	// And the funds should NOT be withdrawable after 1 iteration
	// step 1: start the first iteration
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	_, err = staker.DelegatorWithdrawStake(id)
	assert.ErrorContains(t, err, "delegator is not eligible for withdraw")

	// step 2: end the first iteration
	_, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	_, err = staker.DelegatorWithdrawStake(id)
	assert.ErrorContains(t, err, "delegator is not eligible for withdraw")
}

func Test_Delegator_EnableAutoRenew_InAStakingPeriod(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew disabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegator(validator.ID, stake, false, 255)
	assert.NoError(t, err)

	// And the first staking period has occurred
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.CooldownVET)

	// When the delegator enables auto renew
	assert.NoError(t, staker.UpdateDelegatorAutoRenew(id, true))

	// Then the stake is moved to pending locked
	delegation, err = staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.LockedVET)

	// And the funds should NOT be withdrawable after 1 iteration
	_, err = staker.Housekeep(2 * validator.Period)
	assert.NoError(t, err)
	_, err = staker.DelegatorWithdrawStake(id)
	assert.ErrorContains(t, err, "delegator is not eligible for withdraw")
}

func Test_Delegator_AutoRenew_ValidatorExits(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegator(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	// And the first staking period has occurred
	_, err = staker.Housekeep(validator.Period)
	assert.NoError(t, err)
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.LockedVET)

	// When the validator signals an exit
	assert.NoError(t, staker.UpdateAutoRenew(validator.Endorsor, validator.ID, false, validator.Period+1))

	// And the next staking period is over
	_, err = staker.Housekeep(validator.Period * 2)
	assert.NoError(t, err)
	delegation, err = staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, stake, delegation.WithdrawVET)

	// Then the funds should be withdrawable
	amount, err := staker.DelegatorWithdrawStake(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)
}

func Test_Delegator_WithdrawWhilePending(t *testing.T) {
	// Given the staker contract is setup
	staker, validators := newDelegationStaker(t)

	// And a delegator is added with auto renew enabled
	validator := validators[0]
	stake := RandomStake()
	id, err := staker.AddDelegator(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	// When the delegator withdraws
	amount, err := staker.DelegatorWithdrawStake(id)
	assert.NoError(t, err)
	assert.Equal(t, stake, amount)

	// Then the delegation should be removed
	delegation, err := staker.storage.GetDelegation(validator.ID)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), delegation.PendingLockedVET)

	// And the delegator should be removed
	delegator, err := staker.GetDelegator(id)
	assert.NoError(t, err)
	assert.True(t, delegator.IsEmpty())
}

func Test_Delegator_ID_ShouldBeIncremental(t *testing.T) {
	staker, validators := newDelegationStaker(t)

	// Given the staker contract is setup
	validator := validators[0]
	stake := big.NewInt(100)

	id, err := staker.AddDelegator(validator.ID, stake, true, 255)
	assert.NoError(t, err)

	for range 100 {
		nextID, err := staker.AddDelegator(validator.ID, stake, true, 255)
		assert.NoError(t, err)
		prev := big.NewInt(0).SetBytes(id.Bytes())
		next := big.NewInt(0).SetBytes(nextID.Bytes())
		assert.Equal(t, prev.Add(prev, big.NewInt(1)), next)
		id = nextID
	}
}
