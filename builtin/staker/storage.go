// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotLockedVET          = nameToSlot("total-stake")
	slotQueuedVET          = nameToSlot("queued-stake")
	slotValidations        = nameToSlot("validations")
	slotValidationLookups  = nameToSlot("validator-lookups")
	slotAggregations       = nameToSlot("aggregated-delegations")
	slotDelegations        = nameToSlot("delegations")
	slotDelegationsCounter = nameToSlot("delegations-counter")
	// active validations linked list
	slotActiveTail      = nameToSlot("validations-active-tail")
	slotActiveHead      = nameToSlot("validations-active-head")
	slotActiveGroupSize = nameToSlot("validations-active-group-size")
	// queued validations linked list
	slotQueuedHead      = nameToSlot("validations-queued-head")
	slotQueuedTail      = nameToSlot("validations-queued-tail")
	slotQueuedGroupSize = nameToSlot("validations-queued-group-size")
	// init params
	slotLowStakingPeriod    = nameToSlot("staker-low-staking-period")
	slotMediumStakingPeriod = nameToSlot("staker-medium-staking-period")
	slotHighStakingPeriod   = nameToSlot("staker-high-staking-period")
)

func nameToSlot(name string) thor.Bytes32 {
	return thor.BytesToBytes32([]byte(name))
}

// storage represents the root storage for the Staker contract.
type storage struct {
	state        *state.State
	address      thor.Address
	validations  *solidity.Mapping[thor.Bytes32, *Validation]
	aggregations *solidity.Mapping[thor.Bytes32, *Aggregation]
	delegations  *solidity.Mapping[thor.Bytes32, *Delegation]
	lookups      *solidity.Mapping[thor.Address, thor.Bytes32] // allows lookup of Validation by node master address
}

// newStorage creates a new instance of storage.
func newStorage(addr thor.Address, state *state.State) *storage {
	return &storage{
		state:        state,
		address:      addr,
		validations:  solidity.NewMapping[thor.Bytes32, *Validation](addr, state, slotValidations),
		aggregations: solidity.NewMapping[thor.Bytes32, *Aggregation](addr, state, slotAggregations),
		delegations:  solidity.NewMapping[thor.Bytes32, *Delegation](addr, state, slotDelegations),
		lookups:      solidity.NewMapping[thor.Address, thor.Bytes32](addr, state, slotValidationLookups),
	}
}

func (s *storage) Address() thor.Address {
	return s.address
}

func (s *storage) State() *state.State {
	return s.state
}

func (s *storage) GetValidator(id thor.Bytes32) (*Validation, error) {
	v, err := s.validations.Get(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return v, nil
}

func (s *storage) SetValidator(id thor.Bytes32, entry *Validation) error {
	if err := s.validations.Set(id, entry); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (s *storage) GetAggregation(validationID thor.Bytes32) (*Aggregation, error) {
	d, err := s.aggregations.Get(validationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator aggregation")
	}
	return d, nil
}

func (s *storage) SetAggregation(validationID thor.Bytes32, entry *Aggregation) error {
	if err := s.aggregations.Set(validationID, entry); err != nil {
		return errors.Wrap(err, "failed to set validator aggregation")
	}
	return nil
}

func (s *storage) GetDelegation(delegationID thor.Bytes32) (*Delegation, error) {
	d, err := s.delegations.Get(delegationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get delegation")
	}
	return d, nil
}

func (s *storage) SetDelegation(delegationID thor.Bytes32, entry *Delegation) error {
	if err := s.delegations.Set(delegationID, entry); err != nil {
		return errors.Wrap(err, "failed to set delegation")
	}
	return nil
}

func (s *storage) GetLookup(address thor.Address) (thor.Bytes32, error) {
	l, err := s.lookups.Get(address)
	if err != nil {
		return thor.Bytes32{}, errors.Wrap(err, "failed to get lookup")
	}
	return l, err
}

func (s *storage) SetLookup(address thor.Address, id thor.Bytes32) error {
	if err := s.lookups.Set(address, id); err != nil {
		return errors.Wrap(err, "failed to set lookup")
	}
	return nil
}

func (s *storage) LookupMaster(master thor.Address) (*Validation, thor.Bytes32, error) {
	id, err := s.GetLookup(master)
	if err != nil {
		return nil, thor.Bytes32{}, err
	}
	if id.IsZero() {
		return &Validation{}, thor.Bytes32{}, nil
	}
	val, err := s.GetValidator(id)
	if err != nil {
		return nil, thor.Bytes32{}, err
	}
	return val, id, nil
}
