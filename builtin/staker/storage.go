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
	slotLockedVET          = nameToSlot("total-stake")
	slotLockedWeight       = nameToSlot("total-weight")
	slotQueuedVET          = nameToSlot("queued-stake")
	slotQueuedWeight       = nameToSlot("queued-weight")
	slotValidations        = nameToSlot("validations")
	slotValidationLookups  = nameToSlot("validator-lookups")
	slotAggregations       = nameToSlot("aggregated-delegations")
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
	// exit epoch mapping
	slotExitEpochs = nameToSlot("exit-epochs")
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
	rewards      *solidity.Mapping[thor.Bytes32, *big.Int]     // stores rewards per validator staking period
	exits        *solidity.Mapping[*big.Int, thor.Bytes32]     // exit block -> validator ID
	charger      *gascharger.Charger                           // track storage access costs
}

// newStorage creates a new instance of storage.
func newStorage(addr thor.Address, state *state.State, charger *gascharger.Charger) *storage {
	return &storage{
		charger:      charger,
		state:        state,
		address:      addr,
		validations:  solidity.NewMapping[thor.Bytes32, *Validation](addr, state, slotValidations),
		aggregations: solidity.NewMapping[thor.Bytes32, *Aggregation](addr, state, slotAggregations),
		delegations:  solidity.NewMapping[thor.Bytes32, *Delegation](addr, state, slotDelegations),
		lookups:      solidity.NewMapping[thor.Address, thor.Bytes32](addr, state, slotValidationLookups),
		rewards:      solidity.NewMapping[thor.Bytes32, *big.Int](addr, state, slotRewards),
		exits:        solidity.NewMapping[*big.Int, thor.Bytes32](addr, state, slotExitEpochs),
	}
}

func (s *storage) chargeGas(cost uint64) {
	if s.charger != nil {
		s.charger.Charge(cost)
	}
}

func (s *storage) Address() thor.Address {
	return s.address
}

func (s *storage) State() *state.State {
	return s.state
}

func (s *storage) GetValidation(id thor.Bytes32) (*Validation, error) {
	s.chargeGas(thor.SloadGas)

	v, err := s.validations.Get(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return v, nil
}

func (s *storage) SetValidation(id thor.Bytes32, entry *Validation) error {
	s.chargeGas(thor.SstoreResetGas)

	if err := s.validations.Set(id, entry); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (s *storage) GetAggregation(validationID thor.Bytes32) (*Aggregation, error) {
	s.chargeGas(thor.SloadGas)

	d, err := s.aggregations.Get(validationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator aggregation")
	}
	return d, nil
}

func (s *storage) SetAggregation(validationID thor.Bytes32, entry *Aggregation) error {
	s.chargeGas(thor.SstoreResetGas)

	if err := s.aggregations.Set(validationID, entry); err != nil {
		return errors.Wrap(err, "failed to set validator aggregation")
	}
	return nil
}

func (s *storage) GetDelegation(delegationID thor.Bytes32) (*Delegation, error) {
	s.chargeGas(thor.SloadGas)

	d, err := s.delegations.Get(delegationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get delegation")
	}
	return d, nil
}

func (s *storage) SetDelegation(delegationID thor.Bytes32, entry *Delegation) error {
	s.chargeGas(thor.SloadGas)

	if err := s.delegations.Set(delegationID, entry); err != nil {
		return errors.Wrap(err, "failed to set delegation")
	}
	return nil
}

// GetDelegationBundle retrieves the delegation, validation, and aggregation for a given delegation ID.
// It returns an error if any of the components are not found or are empty.
func (s *storage) GetDelegationBundle(delegationID thor.Bytes32) (*Delegation, *Validation, *Aggregation, error) {
	delegation, err := s.GetDelegation(delegationID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get delegation")
	}
	if delegation.IsEmpty() {
		return nil, nil, nil, errors.New("delegation is empty")
	}

	validation, err := s.GetValidation(delegation.ValidationID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get validation")
	}
	if validation.IsEmpty() {
		return nil, nil, nil, errors.New("validation is empty")
	}

	aggregation, err := s.GetAggregation(delegation.ValidationID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get aggregation")
	}
	if aggregation.IsEmpty() {
		return nil, nil, nil, errors.New("aggregation is empty")
	}
	return delegation, validation, aggregation, nil
}

func (s *storage) GetLookup(address thor.Address) (thor.Bytes32, error) {
	s.chargeGas(thor.SloadGas)

	l, err := s.lookups.Get(address)
	if err != nil {
		return thor.Bytes32{}, errors.Wrap(err, "failed to get lookup")
	}
	return l, err
}

func (s *storage) SetLookup(address thor.Address, id thor.Bytes32) error {
	s.chargeGas(thor.SstoreResetGas)

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
	val, err := s.GetValidation(id)
	if err != nil {
		return nil, thor.Bytes32{}, err
	}
	return val, id, nil
}

func (s *storage) GetExitEpoch(block uint32) (thor.Bytes32, error) {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	s.chargeGas(thor.SloadGas)
	id, err := s.exits.Get(bigBlock)
	if err != nil {
		return thor.Bytes32{}, errors.Wrap(err, "failed to get exit epoch")
	}
	return id, nil
}

func (s *storage) SetExitEpoch(block uint32, id thor.Bytes32) error {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	s.chargeGas(thor.SstoreResetGas)
	if err := s.exits.Set(bigBlock, id); err != nil {
		return errors.Wrap(err, "failed to set exit epoch")
	}
	return nil
}

func (s *storage) GetRewards(validationID thor.Bytes32, stakingPeriod uint32) (*big.Int, error) {
	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, stakingPeriod)
	key := thor.Blake2b([]byte("rewards"), validationID.Bytes(), periodBytes)

	s.chargeGas(thor.SloadGas)
	return s.rewards.Get(key)
}

func (s *storage) GetCompletedPeriods(validationID thor.Bytes32) (uint32, error) {
	v, err := s.GetValidation(validationID)
	if err != nil {
		return uint32(0), err
	}
	return v.CompleteIterations, nil
}

func (s *storage) IncreaseReward(master thor.Address, reward big.Int) error {
	id, err := s.GetLookup(master)
	if err != nil {
		return err
	}
	if id.IsZero() {
		return nil
	}
	val, err := s.GetValidation(id)
	if err != nil {
		return err
	}

	periodBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(periodBytes, val.CurrentIteration())
	key := thor.Blake2b([]byte("rewards"), id.Bytes(), periodBytes)

	s.chargeGas(thor.SloadGas)

	rewards, err := s.rewards.Get(key)
	if err != nil {
		return err
	}
	s.chargeGas(thor.SstoreResetGas)

	return s.rewards.Set(key, big.NewInt(0).Add(rewards, &reward))
}

func (s *storage) debugOverride(ptr *uint32, bytes32 thor.Bytes32) {
	if num, err := solidity.NewUint256(s.Address(), s.State(), bytes32).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			o := uint32(numUint64)
			logger.Warn("overrode state value", "variable", bytes32.String(), "value", o)
			*ptr = o
		}
	}
}
