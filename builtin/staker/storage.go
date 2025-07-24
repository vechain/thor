// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"encoding/binary"
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotValidations        = nameToSlot("validations")
	slotDelegations        = nameToSlot("delegations")
	slotDelegationsCounter = nameToSlot("delegations-counter")
	slotRewards            = nameToSlot("period-rewards")
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
	slotCooldownPeriod      = nameToSlot("cooldown-period")
	slotEpochLength         = nameToSlot("epoch-length")
	// exit epoch mapping
	slotExitEpochs = nameToSlot("exit-epochs")
)

func nameToSlot(name string) thor.Bytes32 {
	return thor.BytesToBytes32([]byte(name))
}

// storage represents the root storage for the Staker contract.
type storage struct {
	context     *solidity.Context
	validations *solidity.Mapping[thor.Address, *Validation]
	delegations *solidity.Mapping[thor.Bytes32, *Delegation]
	rewards     *solidity.Mapping[thor.Bytes32, *big.Int] // stores rewards per validator staking period
	exits       *solidity.Mapping[*big.Int, thor.Address] // exit block -> validator ID
}

// newStorage creates a new instance of storage.
func newStorage(addr thor.Address, state *state.State, charger *gascharger.Charger) *storage {
	context := solidity.NewContext(addr, state, charger)
	return &storage{
		context:     context,
		validations: solidity.NewMapping[thor.Address, *Validation](context, slotValidations),
		delegations: solidity.NewMapping[thor.Bytes32, *Delegation](context, slotDelegations),
		rewards:     solidity.NewMapping[thor.Bytes32, *big.Int](context, slotRewards),
		exits:       solidity.NewMapping[*big.Int, thor.Address](context, slotExitEpochs),
	}
}

func (s *storage) GetValidation(id thor.Address) (*Validation, error) {
	v, err := s.validations.Get(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return v, nil
}

func (s *storage) SetValidation(id thor.Address, entry *Validation, isNew bool) error {
	if err := s.validations.Set(id, entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set validator")
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

func (s *storage) SetDelegation(delegationID thor.Bytes32, entry *Delegation, isNew bool) error {
	if err := s.delegations.Set(delegationID, entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set delegation")
	}
	return nil
}

// GetDelegationBundle retrieves the delegation, validation, and aggregation for a given delegation ID.
// It returns an error if any of the components are not found or are empty.
func (s *storage) GetDelegationBundle(delegationID thor.Bytes32) (*Delegation, *Validation, error) {
	delegation, err := s.GetDelegation(delegationID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get delegation")
	}
	if delegation.IsEmpty() {
		return nil, nil, errors.New("delegation is empty")
	}

	validation, err := s.GetValidation(delegation.ValidationID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get validation")
	}
	if validation.IsEmpty() {
		return nil, nil, errors.New("validation is empty")
	}

	return delegation, validation, nil
}

func (s *storage) GetExitEpoch(block uint32) (thor.Address, error) {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	id, err := s.exits.Get(bigBlock)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get exit epoch")
	}
	return id, nil
}

func (s *storage) SetExitEpoch(block uint32, id thor.Address) error {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	if err := s.exits.Set(bigBlock, id, true); err != nil {
		return errors.Wrap(err, "failed to set exit epoch")
	}
	return nil
}

func (s *storage) GetRewards(validationID thor.Address, stakingPeriod uint32) (*big.Int, error) {
	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, stakingPeriod)
	key := thor.Blake2b([]byte("rewards"), validationID.Bytes(), periodBytes)

	return s.rewards.Get(key)
}

func (s *storage) GetCompletedPeriods(validationID thor.Address) (uint32, error) {
	v, err := s.GetValidation(validationID)
	if err != nil {
		return uint32(0), err
	}
	return v.CompleteIterations, nil
}

func (s *storage) IncreaseReward(node thor.Address, reward big.Int) error {
	val, err := s.GetValidation(node)
	if err != nil {
		return err
	}

	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, val.CurrentIteration())
	key := thor.Blake2b([]byte("rewards"), node.Bytes(), periodBytes)

	rewards, err := s.rewards.Get(key)
	if err != nil {
		return err
	}

	return s.rewards.Set(key, big.NewInt(0).Add(rewards, &reward), false)
}

func (s *storage) debugOverride(ptr *uint32, bytes32 thor.Bytes32) {
	if num, err := solidity.NewUint256(s.context, bytes32).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			o := uint32(numUint64)
			logger.Warn("overrode state value", "variable", bytes32.String(), "value", o)
			*ptr = o
		}
	}
}
