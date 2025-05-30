// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"log/slog"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

// delegations is a struct that manages the delegations for the staker contract.
type delegations struct {
	storage   *storage
	queuedVET *solidity.Uint256
	idCounter *solidity.Uint256
}

func newDelegations(storage *storage) *delegations {
	return &delegations{
		storage:   storage,
		queuedVET: solidity.NewUint256(storage.Address(), storage.State(), slotQueuedVET),
		idCounter: solidity.NewUint256(storage.Address(), storage.State(), slotDelegationsCounter),
	}
}

func (d *delegations) Add(
	validationID thor.Bytes32,
	stake *big.Int,
	autoRenew bool,
	multiplier uint8,
) (thor.Bytes32, error) {
	if multiplier == 0 {
		return thor.Bytes32{}, errors.New("multiplier cannot be 0")
	}
	if stake.Cmp(big.NewInt(0)) <= 0 {
		return thor.Bytes32{}, errors.New("stake must be greater than 0")
	}
	validator, err := d.storage.GetValidator(validationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if validator.IsEmpty() {
		return thor.Bytes32{}, errors.New("validator not found")
	}
	if validator.Status != StatusQueued && validator.Status != StatusActive {
		return thor.Bytes32{}, errors.New("validator is not queued or active")
	}

	aggregated, err := d.storage.GetAggregation(validationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if aggregated.IsEmpty() {
		aggregated = newAggregation()
	}
	nextPeriodStake := validator.NextPeriodStakes(aggregated)
	nextPeriodStake = nextPeriodStake.Add(nextPeriodStake, stake)
	if nextPeriodStake.Cmp(maxStake) > 0 {
		return thor.Bytes32{}, errors.New("validator's next period stake exceeds max stake")
	}

	id, err := d.idCounter.Get()
	if err != nil {
		return thor.Bytes32{}, err
	}
	id = id.Add(id, big.NewInt(1))
	d.idCounter.Set(id)

	delegationID := thor.BytesToBytes32(id.Bytes())

	delegation := &Delegation{
		Multiplier:     multiplier,
		Stake:          stake,
		AutoRenew:      autoRenew,
		ValidatorID:    validationID,
		FirstIteration: validator.CurrentIteration() + 1,
	}

	weight := delegation.Weight()

	if !autoRenew {
		last := validator.CurrentIteration() + 1
		delegation.LastIteration = &last
	}

	if err := d.queuedVET.Add(stake); err != nil {
		return thor.Bytes32{}, err
	}

	if delegation.AutoRenew {
		aggregated.PendingLockedVET = big.NewInt(0).Add(aggregated.PendingLockedVET, delegation.Stake)
		aggregated.PendingLockedWeight = big.NewInt(0).Add(aggregated.PendingLockedWeight, weight)
	} else {
		aggregated.PendingCooldownVET = big.NewInt(0).Add(aggregated.PendingCooldownVET, delegation.Stake)
		aggregated.PendingCooldownWeight = big.NewInt(0).Add(aggregated.PendingCooldownWeight, weight)
	}

	if err := d.storage.SetAggregation(validationID, aggregated); err != nil {
		return thor.Bytes32{}, err
	}

	return delegationID, d.storage.SetDelegation(delegationID, delegation)
}

func (d *delegations) DisableAutoRenew(delegationID thor.Bytes32) error {
	delegation, err := d.storage.GetDelegation(delegationID)
	if err != nil {
		return err
	}
	if delegation.IsEmpty() {
		return errors.New("delegation is empty")
	}
	if !delegation.AutoRenew {
		return errors.New("delegation is not autoRenew")
	}
	aggregation, err := d.storage.GetAggregation(delegation.ValidatorID)
	if err != nil {
		return err
	}
	if aggregation.IsEmpty() {
		return errors.New("aggregation not found")
	}
	validator, err := d.storage.GetValidator(delegation.ValidatorID)
	if err != nil {
		return err
	}

	weight := delegation.Weight()

	slog.Info("Disabling auto-renew for delegation",
		"pending-cooldown-vet", aggregation.PendingCooldownVET,
		"pending-cooldown-weight", aggregation.PendingCooldownWeight,
		"pending-locked-vet", aggregation.PendingLockedVET,
		"pending-locked-weight", aggregation.PendingLockedWeight)

	// the delegation's funds have already been locked, so we need to move them to cooldown
	if delegation.IsLocked(validator) {
		// move the delegation's portion of locked to cooldown.
		// this will make the funds available at the end of the current iteration
		aggregation.LockedVET = big.NewInt(0).Sub(aggregation.LockedVET, delegation.Stake)
		aggregation.LockedWeight = big.NewInt(0).Sub(aggregation.LockedWeight, weight)

		aggregation.CooldownVET = big.NewInt(0).Add(aggregation.CooldownVET, delegation.Stake)
		aggregation.CooldownWeight = big.NewInt(0).Add(aggregation.CooldownWeight, weight)
	} else {
		// the delegation's stake is pending a lock
		// this moves the delegation's portion of pending locked to pending cooldown
		// pending cooldown means the funds will be available at the end of the next iteration
		aggregation.PendingCooldownVET = big.NewInt(0).Add(aggregation.PendingCooldownVET, delegation.Stake)
		aggregation.PendingCooldownWeight = big.NewInt(0).Add(aggregation.PendingCooldownWeight, weight)

		aggregation.PendingLockedVET = big.NewInt(0).Sub(aggregation.PendingLockedVET, delegation.Stake)
		aggregation.PendingLockedWeight = big.NewInt(0).Sub(aggregation.PendingLockedWeight, weight)
	}

	// set the delegation's exit iteration
	lastIteration := validator.CurrentIteration() + 1
	delegation.LastIteration = &lastIteration
	delegation.AutoRenew = false

	slog.Info("Disabling auto-renew for delegation",
		"pending-cooldown-vet", aggregation.PendingCooldownVET,
		"pending-cooldown-weight", aggregation.PendingCooldownWeight,
		"pending-locked-vet", aggregation.PendingLockedVET,
		"pending-locked-weight", aggregation.PendingLockedWeight)

	if err := d.storage.SetDelegation(delegationID, delegation); err != nil {
		return err
	}

	return d.storage.SetAggregation(delegation.ValidatorID, aggregation)
}

func (d *delegations) EnableAutoRenew(delegationID thor.Bytes32) error {
	delegation, err := d.storage.GetDelegation(delegationID)
	if err != nil {
		return err
	}
	validator, err := d.storage.GetValidator(delegation.ValidatorID)
	if err != nil {
		return err
	}
	if delegation.IsEmpty() {
		return errors.New("delegation is empty")
	}
	if delegation.AutoRenew {
		return errors.New("delegation is already autoRenew")
	}
	aggregation, err := d.storage.GetAggregation(delegation.ValidatorID)
	if err != nil {
		return err
	}
	if aggregation.IsEmpty() {
		return errors.New("aggregation not found")
	}

	weight := delegation.Weight()

	if delegation.IsLocked(validator) {
		// move the delegation's portion of cooldown to locked.
		// this means the funds will not be available until the validator is inactive, or the delegation signals an exit
		// and completes the current staking period
		aggregation.CooldownVET = big.NewInt(0).Sub(aggregation.CooldownVET, delegation.Stake)
		aggregation.CooldownWeight = big.NewInt(0).Sub(aggregation.CooldownWeight, weight)

		aggregation.LockedVET = big.NewInt(0).Add(aggregation.LockedVET, delegation.Stake)
		aggregation.LockedWeight = big.NewInt(0).Add(aggregation.LockedWeight, weight)
	} else {
		// the delegation's stake is moved from pending locked to pending cooldown, ie 1 staking period
		aggregation.PendingLockedVET = big.NewInt(0).Add(aggregation.PendingLockedVET, delegation.Stake)
		aggregation.PendingLockedWeight = big.NewInt(0).Add(aggregation.PendingLockedWeight, weight)

		aggregation.PendingCooldownVET = big.NewInt(0).Sub(aggregation.PendingCooldownVET, delegation.Stake)
		aggregation.PendingCooldownWeight = big.NewInt(0).Sub(aggregation.PendingCooldownWeight, weight)
	}

	delegation.LastIteration = nil
	delegation.AutoRenew = true
	if err := d.storage.SetDelegation(delegationID, delegation); err != nil {
		return err
	}

	return d.storage.SetAggregation(delegation.ValidatorID, aggregation)
}

func (d *delegations) Withdraw(delegationID thor.Bytes32) (*big.Int, error) {
	delegation, err := d.storage.GetDelegation(delegationID)
	if err != nil {
		return nil, err
	}
	if delegation.IsEmpty() {
		return nil, errors.New("delegation is empty")
	}
	aggregation, err := d.storage.GetAggregation(delegation.ValidatorID)
	if err != nil {
		return nil, err
	}
	validator, err := d.storage.GetValidator(delegation.ValidatorID)
	if err != nil {
		return nil, err
	}
	if delegation.Stake.Sign() == 0 {
		return nil, errors.New("delegation already withdrawn")
	}
	if delegation.IsLocked(validator) {
		return nil, errors.New("delegation is not eligible for withdraw")
	}

	delegationStarted := delegation.FirstIteration <= validator.CompleteIterations
	if delegationStarted && validator.Status != StatusQueued {
		// the stake has moved to withdrawable since we checked if the validator is locked above
		if aggregation.WithdrawVET.Cmp(delegation.Stake) < 0 {
			return nil, errors.New("not enough withdraw VET")
		}
		aggregation.WithdrawVET = aggregation.WithdrawVET.Sub(aggregation.WithdrawVET, delegation.Stake)
	} else {
		if delegation.AutoRenew { // delegation's stake is pending locked
			if aggregation.PendingLockedVET.Cmp(delegation.Stake) < 0 {
				return nil, errors.New("not enough pending locked VET")
			}
			aggregation.PendingLockedVET = aggregation.PendingLockedVET.Sub(aggregation.PendingLockedVET, delegation.Stake)
		} else { // delegation's stake is pending 1 staking period only, i.e., pending cooldown
			if aggregation.PendingCooldownVET.Cmp(delegation.Stake) < 0 {
				return nil, errors.New("not enough pending cooldown VET")
			}
			aggregation.PendingCooldownVET = aggregation.PendingCooldownVET.Sub(aggregation.PendingCooldownVET, delegation.Stake)
		}
	}

	amount := delegation.Stake
	delegation.Stake = big.NewInt(0)

	// remove the delegation from the mapping after the withdraw
	if err := d.storage.SetDelegation(delegationID, delegation); err != nil {
		return nil, err
	}

	if err = d.storage.SetAggregation(delegation.ValidatorID, aggregation); err != nil {
		return nil, err
	}

	return amount, nil
}
