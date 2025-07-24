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

// delegations is a struct that manages the delegations for the staker contract.
type delegations struct {
	storage   *storage
	idCounter *solidity.Uint256
}

func newDelegations(storage *storage) *delegations {
	return &delegations{
		storage:   storage,
		idCounter: solidity.NewUint256(storage.context, slotDelegationsCounter),
	}
}

func (d *delegations) Add(
	validationID thor.Address,
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
	validation, err := d.storage.GetValidation(validationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if validation.IsEmpty() {
		return thor.Bytes32{}, errors.New("validation not found")
	}
	if validation.Status != StatusQueued && validation.Status != StatusActive {
		return thor.Bytes32{}, errors.New("validation is not queued or active")
	}

	aggregated, err := d.storage.GetAggregation(validationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if aggregated.IsEmpty() {
		aggregated = newAggregation()
	}
	nextPeriodTVL := big.NewInt(0).Add(validation.NextPeriodTVL(), aggregated.NextPeriodTVL())
	nextPeriodTVL = nextPeriodTVL.Add(nextPeriodTVL, stake)
	if nextPeriodTVL.Cmp(MaxStake) > 0 {
		return thor.Bytes32{}, errors.New("validation's next period stake exceeds max stake")
	}

	id, err := d.idCounter.Get()
	if err != nil {
		return thor.Bytes32{}, err
	}
	id = id.Add(id, big.NewInt(1))
	if err := d.idCounter.Set(id); err != nil {
		return thor.Bytes32{}, errors.Wrap(err, "failed to increment delegation ID counter")
	}

	delegationID := thor.BytesToBytes32(id.Bytes())

	delegation := &Delegation{
		Multiplier:     multiplier,
		Stake:          stake,
		AutoRenew:      autoRenew,
		ValidationID:   validationID,
		FirstIteration: validation.CurrentIteration() + 1,
	}
	weight := delegation.Weight()
	if delegation.AutoRenew {
		aggregated.PendingRecurringVET = big.NewInt(0).Add(aggregated.PendingRecurringVET, delegation.Stake)
		aggregated.PendingRecurringWeight = big.NewInt(0).Add(aggregated.PendingRecurringWeight, weight)
	} else {
		aggregated.PendingOneTimeVET = big.NewInt(0).Add(aggregated.PendingOneTimeVET, delegation.Stake)
		aggregated.PendingOneTimeWeight = big.NewInt(0).Add(aggregated.PendingOneTimeWeight, weight)
		last := validation.CurrentIteration() + 1
		delegation.LastIteration = &last
	}

	if err := d.storage.queuedVET.Add(stake); err != nil {
		return thor.Bytes32{}, err
	}
	if err := d.storage.queuedWeight.Add(weight); err != nil {
		return thor.Bytes32{}, err
	}
	if err := d.storage.SetAggregation(validationID, aggregated, false); err != nil {
		return thor.Bytes32{}, err
	}

	return delegationID, d.storage.SetDelegation(delegationID, delegation, true)
}

func (d *delegations) DisableAutoRenew(delegationID thor.Bytes32) error {
	delegation, validation, aggregation, err := d.storage.GetDelegationBundle(delegationID)
	if err != nil {
		return err
	}
	if !delegation.AutoRenew {
		return errors.New("delegation is not autoRenew")
	}
	if delegation.Stake.Sign() == 0 {
		return errors.New("delegation is not active")
	}
	if delegation.Ended(validation) {
		return errors.New("delegation is not active")
	}

	weight := delegation.Weight()

	// the delegation's funds have already been locked, so we need to move them to non-recurring, but still locked
	if delegation.Started(validation) {
		// move the delegation's portion of locked to non-recurring.
		// this will make the funds available at the end of the current iteration
		aggregation.CurrentRecurringVET = big.NewInt(0).Sub(aggregation.CurrentRecurringVET, delegation.Stake)
		aggregation.CurrentRecurringWeight = big.NewInt(0).Sub(aggregation.CurrentRecurringWeight, weight)

		aggregation.CurrentOneTimeVET = big.NewInt(0).Add(aggregation.CurrentOneTimeVET, delegation.Stake)
		aggregation.CurrentOneTimeWeight = big.NewInt(0).Add(aggregation.CurrentOneTimeWeight, weight)
	} else {
		// the delegation's stake is pending a lock
		// this moves the delegation's portion of pending locked to pending non-recurring
		// pending non-recurring means the funds will be available after completing one staking period
		aggregation.PendingOneTimeVET = big.NewInt(0).Add(aggregation.PendingOneTimeVET, delegation.Stake)
		aggregation.PendingOneTimeWeight = big.NewInt(0).Add(aggregation.PendingOneTimeWeight, weight)

		aggregation.PendingRecurringVET = big.NewInt(0).Sub(aggregation.PendingRecurringVET, delegation.Stake)
		aggregation.PendingRecurringWeight = big.NewInt(0).Sub(aggregation.PendingRecurringWeight, weight)
	}

	// TODO: In a future PR this won't be possible, so it will be removed. This is backwards compatible according to the unit tests.
	// - In future: delegations auto added as auto-renew, then will have to signal an exit to withdraw in the next staking period.
	// set the delegation's exit iteration
	var lastIteration uint32
	if delegation.Started(validation) {
		lastIteration = validation.CurrentIteration()
	} else {
		lastIteration = delegation.FirstIteration
	}
	delegation.LastIteration = &lastIteration
	delegation.AutoRenew = false

	if err := d.storage.SetDelegation(delegationID, delegation, false); err != nil {
		return err
	}

	return d.storage.SetAggregation(delegation.ValidationID, aggregation, false)
}

func (d *delegations) EnableAutoRenew(delegationID thor.Bytes32) error {
	delegation, validation, aggregation, err := d.storage.GetDelegationBundle(delegationID)
	if err != nil {
		return err
	}
	if delegation.AutoRenew {
		return errors.New("delegation is already autoRenew")
	}
	if delegation.Ended(validation) {
		return errors.New("delegation is not active")
	}
	weight := delegation.Weight()

	// validate that the enablement does not exceed the max stake considering next staking period changes
	nextPeriodTVL := big.NewInt(0).Add(validation.NextPeriodTVL(), aggregation.NextPeriodTVL())
	nextPeriodTVL.Add(nextPeriodTVL, delegation.Stake)
	if nextPeriodTVL.Cmp(MaxStake) > 0 {
		return errors.New("validation's next period stake exceeds max stake")
	}

	if delegation.Started(validation) {
		// move the delegation's portion of non-recurring to locked.
		// this means the funds will not be available until the validator is inactive, or the delegation signals an exit
		// and completes the current staking period
		aggregation.CurrentOneTimeVET = big.NewInt(0).Sub(aggregation.CurrentOneTimeVET, delegation.Stake)
		aggregation.CurrentOneTimeWeight = big.NewInt(0).Sub(aggregation.CurrentOneTimeWeight, weight)

		aggregation.CurrentRecurringVET = big.NewInt(0).Add(aggregation.CurrentRecurringVET, delegation.Stake)
		aggregation.CurrentRecurringWeight = big.NewInt(0).Add(aggregation.CurrentRecurringWeight, weight)
	} else {
		// the delegation's stake is moved from pending locked to pending non-recurring, ie 1 staking period
		aggregation.PendingRecurringVET = big.NewInt(0).Add(aggregation.PendingRecurringVET, delegation.Stake)
		aggregation.PendingRecurringWeight = big.NewInt(0).Add(aggregation.PendingRecurringWeight, weight)

		aggregation.PendingOneTimeVET = big.NewInt(0).Sub(aggregation.PendingOneTimeVET, delegation.Stake)
		aggregation.PendingOneTimeWeight = big.NewInt(0).Sub(aggregation.PendingOneTimeWeight, weight)
	}

	delegation.LastIteration = nil
	delegation.AutoRenew = true
	if err := d.storage.SetDelegation(delegationID, delegation, false); err != nil {
		return err
	}

	return d.storage.SetAggregation(delegation.ValidationID, aggregation, false)
}

func (d *delegations) Withdraw(delegationID thor.Bytes32) (*big.Int, error) {
	delegation, validation, aggregation, err := d.storage.GetDelegationBundle(delegationID)
	if err != nil {
		return nil, err
	}
	started := delegation.Started(validation)
	finished := delegation.Ended(validation)
	if started && !finished {
		return nil, errors.New("delegation is not eligible for withdraw")
	}
	weight := delegation.Weight()

	if !started {
		if delegation.AutoRenew { // delegation's stake is pending locked
			if aggregation.PendingRecurringVET.Cmp(delegation.Stake) < 0 {
				return nil, errors.New("not enough pending locked VET")
			}
			aggregation.PendingRecurringVET = big.NewInt(0).Sub(aggregation.PendingRecurringVET, delegation.Stake)
			aggregation.PendingRecurringWeight = big.NewInt(0).Sub(aggregation.PendingRecurringWeight, weight)
		} else { // delegation's stake is pending 1 staking period only, i.e., pending non-recurring
			if aggregation.PendingOneTimeVET.Cmp(delegation.Stake) < 0 {
				return nil, errors.New("not enough pending non-recurring VET")
			}
			aggregation.PendingOneTimeVET = big.NewInt(0).Sub(aggregation.PendingOneTimeVET, delegation.Stake)
			aggregation.PendingOneTimeWeight = big.NewInt(0).Sub(aggregation.PendingOneTimeWeight, weight)
		}
		if err := d.storage.queuedVET.Sub(delegation.Stake); err != nil {
			return nil, err
		}
		if err := d.storage.queuedWeight.Sub(weight); err != nil {
			return nil, err
		}
	}

	if finished {
		// the stake has moved to withdrawable since we checked if the validation is locked above
		if aggregation.WithdrawableVET.Cmp(delegation.Stake) < 0 {
			return nil, errors.New("not enough withdraw VET")
		}
		aggregation.WithdrawableVET = big.NewInt(0).Sub(aggregation.WithdrawableVET, delegation.Stake)
	}

	stake := delegation.Stake
	delegation.Stake = big.NewInt(0)
	// remove the delegation from the mapping after the withdraw
	if err := d.storage.SetDelegation(delegationID, delegation, false); err != nil {
		return nil, err
	}
	if err = d.storage.SetAggregation(delegation.ValidationID, aggregation, false); err != nil {
		return nil, err
	}

	return stake, nil
}
