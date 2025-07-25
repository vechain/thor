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

	return delegationID, d.storage.SetDelegation(delegationID, delegation, true)
}

func (d *delegations) SignalExit(delegationID thor.Bytes32) error {
	delegation, validation, err := d.storage.GetDelegationBundle(delegationID)
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

	last := validation.CurrentIteration()
	delegation.LastIteration = &last

	return d.storage.SetDelegation(delegationID, delegation, false)
}

func (d *delegations) Withdraw(delegationID thor.Bytes32) (bool, bool, *big.Int, *big.Int, error) {
	delegation, validation, err := d.storage.GetDelegationBundle(delegationID)
	if err != nil {
		return false, false, nil, nil, err
	}
	started := delegation.Started(validation)
	finished := delegation.Ended(validation)
	if started && !finished {
		return false, false, nil, nil, errors.New("delegation is not eligible for withdraw")
	}
	// ensure the pointers are copied, not referenced
	withdrawableStake := new(big.Int).Set(delegation.Stake)
	withdrawableStakeWeight := delegation.CalcWeight()

	delegation.Stake = big.NewInt(0)
	if err = d.storage.SetDelegation(delegationID, delegation, false); err != nil {
		return false, false, nil, nil, err
	}

	return started, finished, withdrawableStake, withdrawableStakeWeight, nil
}
