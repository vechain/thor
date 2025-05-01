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
	slotLockedVET         = nameToSlot("total-stake")
	slotQueuedVET         = nameToSlot("queued-stake")
	slotValidations       = nameToSlot("validations")
	slotValidationLookups = nameToSlot("validator-lookups")
	slotDelegations       = nameToSlot("delegations")
	slotDelegators        = nameToSlot("delegators")
	slotDelegatorsCounter = nameToSlot("delegators-counter")
	// active validators linked list
	slotActiveTail      = nameToSlot("validators-active-tail")
	slotActiveHead      = nameToSlot("validators-active-head")
	slotActiveGroupSize = nameToSlot("validators-active-group-size")
	// queued validators linked list
	slotQueuedHead      = nameToSlot("validators-queued-head")
	slotQueuedTail      = nameToSlot("validators-queued-tail")
	slotQueuedGroupSize = nameToSlot("validators-queued-group-size")
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
	state       *state.State
	address     thor.Address
	validators  *solidity.Mapping[thor.Bytes32, *Validation]
	delegations *solidity.Mapping[thor.Bytes32, *ValidatorDelegations]
	delegators  *solidity.Mapping[thor.Bytes32, *Delegator]
	lookups     *solidity.Mapping[thor.Address, thor.Bytes32] // allows lookup of Validation by node master address
}

// newStorage creates a new instance of storage.
func newStorage(addr thor.Address, state *state.State) *storage {
	return &storage{
		state:       state,
		address:     addr,
		validators:  solidity.NewMapping[thor.Bytes32, *Validation](addr, state, slotValidations),
		delegations: solidity.NewMapping[thor.Bytes32, *ValidatorDelegations](addr, state, slotDelegations),
		delegators:  solidity.NewMapping[thor.Bytes32, *Delegator](addr, state, slotDelegators),
		lookups:     solidity.NewMapping[thor.Address, thor.Bytes32](addr, state, slotValidationLookups),
	}
}

func (s *storage) Address() thor.Address {
	return s.address
}

func (s *storage) State() *state.State {
	return s.state
}

func (s *storage) GetValidator(id thor.Bytes32) (*Validation, error) {
	v, err := s.validators.Get(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return v, nil
}

func (s *storage) SetValidator(id thor.Bytes32, entry *Validation) error {
	if err := s.validators.Set(id, entry); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (s *storage) GetDelegation(id thor.Bytes32) (*ValidatorDelegations, error) {
	d, err := s.delegations.Get(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get delegation")
	}
	return d, nil
}

func (s *storage) SetDelegation(id thor.Bytes32, entry *ValidatorDelegations) error {
	if err := s.delegations.Set(id, entry); err != nil {
		return errors.Wrap(err, "failed to set delegation")
	}
	return nil
}

func (s *storage) GetDelegator(delegationID thor.Bytes32) (*Delegator, error) {
	d, err := s.delegators.Get(delegationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get delegator")
	}
	return d, nil
}

func (s *storage) SetDelegator(delegationID thor.Bytes32, entry *Delegator) error {
	if err := s.delegators.Set(delegationID, entry); err != nil {
		return errors.Wrap(err, "failed to set delegator")
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
