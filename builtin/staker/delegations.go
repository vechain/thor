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
		ValidationID:   validationID,
		FirstIteration: validation.CurrentIteration() + 1,
	}
	aggregated.PendingVET = big.NewInt(0).Add(aggregated.PendingVET, stake)
	aggregated.PendingWeight = big.NewInt(0).Add(aggregated.PendingWeight, delegation.Weight())

	if err := d.storage.queuedVET.Add(stake); err != nil {
		return thor.Bytes32{}, err
	}
	if err := d.storage.queuedWeight.Add(delegation.Weight()); err != nil {
		return thor.Bytes32{}, err
	}
	if err := d.storage.SetAggregation(validationID, aggregated, false); err != nil {
		return thor.Bytes32{}, err
	}

	return delegationID, d.storage.SetDelegation(delegationID, delegation, true)
}

func (d *delegations) SignalExit(delegationID thor.Bytes32) error {
	delegation, validation, aggregation, err := d.storage.GetDelegationBundle(delegationID)
	if err != nil {
		return err
	}
	if delegation.LastIteration != nil {
		return errors.New("delegation is already disabled for auto-renew")
	}
	if delegation.Stake.Sign() == 0 {
		return errors.New("delegation is not active")
	}
	if !delegation.Started(validation) {
		return errors.New("delegation has not started yet, funds can be withdrawn")
	}
	if delegation.Ended(validation) {
		return errors.New("delegation has ended, funds can be withdrawn")
	}
	aggregation.ExitingVET = big.NewInt(0).Add(aggregation.ExitingVET, delegation.Stake)
	aggregation.ExitingWeight = big.NewInt(0).Add(aggregation.ExitingWeight, delegation.Weight())

	last := validation.CurrentIteration()
	delegation.LastIteration = &last

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

	if !started { // delegation's funds are still pending
		aggregation.PendingVET = big.NewInt(0).Sub(aggregation.PendingVET, delegation.Stake)
		aggregation.PendingWeight = big.NewInt(0).Sub(aggregation.PendingWeight, weight)

		if err := d.storage.queuedVET.Sub(delegation.Stake); err != nil {
			return nil, err
		}
		if err := d.storage.queuedWeight.Sub(weight); err != nil {
			return nil, err
		}
	}

	if finished { // delegation's funds have move to withdrawable
		if aggregation.WithdrawableVET.Cmp(delegation.Stake) < 0 {
			return nil, errors.New("not enough withdraw VET")
		}
		aggregation.WithdrawableVET = big.NewInt(0).Sub(aggregation.WithdrawableVET, delegation.Stake)
	}

	stake := delegation.Stake
	delegation.Stake = big.NewInt(0)
	if err := d.storage.SetDelegation(delegationID, delegation, false); err != nil {
		return nil, err
	}
	if err = d.storage.SetAggregation(delegation.ValidationID, aggregation, false); err != nil {
		return nil, err
	}

	return stake, nil
}
