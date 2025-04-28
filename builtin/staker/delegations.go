// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

// delegations represent a 1-to-1 mapping of a validator to the sum of its delegator totals.
type delegations struct {
	storage   *storage
	queuedVET *solidity.Uint256
}

func newDelegations(storage *storage) *delegations {
	return &delegations{
		storage:   storage,
		queuedVET: solidity.NewUint256(storage.Address(), storage.State(), slotQueuedVET),
	}
}

func (d *delegations) Add(
	validationID thor.Bytes32,
	delegatorAddr thor.Address,
	stake *big.Int,
	autoRenew bool,
	multiplier uint8,
) error {
	if multiplier == 0 {
		return errors.New("multiplier cannot be 0")
	}
	if stake.Cmp(big.NewInt(0)) <= 0 {
		return errors.New("stake must be greater than 0")
	}
	delegator, err := d.storage.GetDelegator(validationID, delegatorAddr)
	if err != nil {
		return err
	}
	if !delegator.IsEmpty() {
		return errors.New("delegator already exists")
	}
	validator, err := d.storage.GetValidator(validationID)
	if err != nil {
		return err
	}
	if validator.IsEmpty() {
		return errors.New("validator not found")
	}

	delegation, err := d.storage.GetDelegation(validationID)
	if err != nil {
		return err
	}
	if delegation.IsEmpty() {
		delegation = newDelegation()
	}
	nextPeriodStake := validator.NextPeriodStakes(delegation)
	nextPeriodStake = nextPeriodStake.Add(nextPeriodStake, stake)
	if nextPeriodStake.Cmp(maxStake) > 0 {
		return errors.New("validator's next period stake exceeds max stake")
	}

	delegator.Multiplier = multiplier
	delegator.Stake = stake
	delegator.AutoRenew = autoRenew
	delegator.ValidatorID = validationID
	delegator.FirstIteration = validator.CompleteIterations + 1

	weight := delegator.Weight()

	if !autoRenew {
		exitIteration := validator.CompleteIterations + 1
		if validator.Status == StatusActive {
			exitIteration += 1 // validator is currently active, so this delegation needs to wait for this iteration and the next
		}
		delegator.ExitIteration = &exitIteration
	}

	if err := d.queuedVET.Add(stake); err != nil {
		return err
	}

	if delegator.AutoRenew {
		delegation.PendingLockedVET = big.NewInt(0).Add(delegation.PendingLockedVET, delegator.Stake)
		delegation.PendingLockedWeight = big.NewInt(0).Add(delegation.PendingLockedWeight, weight)
	} else {
		delegation.PendingCooldownVET = big.NewInt(0).Add(delegation.PendingCooldownVET, delegator.Stake)
		delegation.PendingCooldownWeight = big.NewInt(0).Add(delegation.PendingCooldownWeight, weight)
	}

	if err := d.storage.SetDelegation(validationID, delegation); err != nil {
		return err
	}

	return d.storage.SetDelegator(validationID, delegatorAddr, delegator)
}

func (d *delegations) DisableAutoRenew(validationID thor.Bytes32, delegatorAddr thor.Address) error {
	delegator, err := d.storage.GetDelegator(validationID, delegatorAddr)
	if err != nil {
		return err
	}
	if delegator.IsEmpty() {
		return errors.New("delegator is empty")
	}
	if !delegator.AutoRenew {
		return errors.New("delegator is not autoRenew")
	}
	delegation, err := d.storage.GetDelegation(delegator.ValidatorID)
	if err != nil {
		return err
	}
	if delegation.IsEmpty() {
		return errors.New("delegation not found")
	}
	validator, err := d.storage.GetValidator(validationID)
	if err != nil {
		return err
	}

	weight := delegator.Weight()

	// update the delegator
	exitIteration := validator.CompleteIterations + 1
	if validator.Status == StatusActive && validator.CompleteIterations < delegator.FirstIteration {
		exitIteration += 1 // delegator's first staking period has not begun, it must wait for the current and then its own period
	}
	delegator.ExitIteration = &exitIteration
	delegator.AutoRenew = false

	if err := d.storage.SetDelegator(validationID, delegatorAddr, delegator); err != nil {
		return err
	}

	// the delegator's funds have already been locked, so we need to move them to cooldown
	if validator.CompleteIterations >= delegator.FirstIteration {
		// move the delegator's portion of locked to cooldown.
		// this will make the funds available at the end of the current iteration
		delegation.LockedVET = big.NewInt(0).Sub(delegation.LockedVET, delegator.Stake)
		delegation.LockedWeight = big.NewInt(0).Sub(delegation.LockedWeight, weight)

		delegation.CooldownVET = big.NewInt(0).Add(delegation.CooldownVET, delegator.Stake)
		delegation.CooldownWeight = big.NewInt(0).Add(delegation.CooldownWeight, weight)
	} else {
		// the delegator's stake is pending a lock
		// this moves the delegator's portion of pending locked to pending cooldown
		// pending cooldown means the funds will be available at the end of the next iteration
		delegation.PendingCooldownVET = big.NewInt(0).Add(delegation.PendingCooldownVET, delegator.Stake)
		delegation.PendingCooldownWeight = big.NewInt(0).Add(delegation.PendingCooldownWeight, weight)

		delegation.PendingLockedVET = big.NewInt(0).Sub(delegation.PendingLockedVET, delegator.Stake)
		delegation.PendingLockedWeight = big.NewInt(0).Sub(delegation.PendingLockedWeight, weight)
	}

	return d.storage.SetDelegation(delegator.ValidatorID, delegation)
}

func (d *delegations) EnableAutoRenew(validationID thor.Bytes32, delegatorAddr thor.Address) error {
	validator, err := d.storage.GetValidator(validationID)
	if err != nil {
		return err
	}
	delegator, err := d.storage.GetDelegator(validationID, delegatorAddr)
	if err != nil {
		return err
	}
	if delegator.IsEmpty() {
		return errors.New("delegator is empty")
	}
	if delegator.AutoRenew {
		return errors.New("delegator is already autoRenew")
	}
	delegation, err := d.storage.GetDelegation(delegator.ValidatorID)
	if err != nil {
		return err
	}
	if delegation.IsEmpty() {
		return errors.New("delegation not found")
	}

	weight := delegator.Weight()

	delegator.ExitIteration = nil
	delegator.AutoRenew = true
	if err := d.storage.SetDelegator(validationID, delegatorAddr, delegator); err != nil {
		return err
	}

	if validator.CompleteIterations >= delegator.FirstIteration {
		// move the delegator's portion of cooldown to locked.
		// this means the funds will not be available until the validator is inactive, or the delegator signals an exit
		// and completes the current staking period
		delegation.CooldownVET = big.NewInt(0).Sub(delegation.CooldownVET, delegator.Stake)
		delegation.CooldownWeight = big.NewInt(0).Sub(delegation.CooldownWeight, weight)

		delegation.LockedVET = big.NewInt(0).Add(delegation.LockedVET, delegator.Stake)
		delegation.LockedWeight = big.NewInt(0).Add(delegation.LockedWeight, weight)
	} else {
		// the delegator's stake is from pending locked, to pending cooldown, ie 1 staking period
		delegation.PendingLockedVET = big.NewInt(0).Add(delegation.PendingLockedVET, delegator.Stake)
		delegation.PendingLockedWeight = big.NewInt(0).Add(delegation.PendingLockedWeight, weight)

		delegation.PendingCooldownVET = big.NewInt(0).Sub(delegation.PendingCooldownVET, delegator.Stake)
		delegation.PendingCooldownWeight = big.NewInt(0).Sub(delegation.PendingCooldownWeight, weight)
	}

	return d.storage.SetDelegation(delegator.ValidatorID, delegation)
}

func (d *delegations) Withdraw(validationID thor.Bytes32, delegatorAddr thor.Address) (*big.Int, error) {
	delegator, err := d.storage.GetDelegator(validationID, delegatorAddr)
	if err != nil {
		return nil, err
	}
	if delegator.IsEmpty() {
		return nil, errors.New("delegator is empty")
	}
	delegation, err := d.storage.GetDelegation(delegator.ValidatorID)
	if err != nil {
		return nil, err
	}
	validator, err := d.storage.GetValidator(validationID)
	if err != nil {
		return nil, err
	}
	if delegator.IsLocked(validator) {
		return nil, errors.New("delegator is not eligible for withdraw")
	}

	delegationStarted := delegator.FirstIteration <= validator.CompleteIterations
	if delegationStarted {
		// the stake has moved to withdrawable since we checked if the validator is locked above
		if delegation.WithdrawVET.Cmp(delegator.Stake) < 0 {
			return nil, errors.New("not enough withdraw VET")
		}
		delegation.WithdrawVET = delegation.WithdrawVET.Sub(delegation.WithdrawVET, delegator.Stake)
	} else {
		if delegator.AutoRenew { // delegator's stake is pending locked
			if delegation.PendingLockedVET.Cmp(delegator.Stake) < 0 {
				return nil, errors.New("not enough pending locked VET")
			}
			delegation.PendingLockedVET = delegation.PendingLockedVET.Sub(delegation.PendingLockedVET, delegator.Stake)
		} else { // delegator's stake is pending 1 staking period only, i.e., pending cooldown
			if delegation.PendingCooldownVET.Cmp(delegator.Stake) < 0 {
				return nil, errors.New("not enough pending cooldown VET")
			}
			delegation.PendingCooldownVET = delegation.PendingCooldownVET.Sub(delegation.PendingCooldownVET, delegator.Stake)
		}
	}

	// remove the delegator from the mapping after the withdraw
	if err := d.storage.SetDelegator(validationID, delegatorAddr, &Delegator{}); err != nil {
		return nil, err
	}

	if err = d.storage.SetDelegation(delegator.ValidatorID, delegation); err != nil {
		return nil, err
	}

	return delegator.Stake, nil
}
