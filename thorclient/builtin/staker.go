// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/vechain/thor/v2/api"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

type StakerStatus uint8

const (
	StakerStatusUnknown StakerStatus = iota
	StakerStatusQueued
	StakerStatusActive
	StakerStatusExited
)

func MinStake() *big.Int {
	eth := big.NewInt(1e18)                        // 1 ETH
	return new(big.Int).Mul(eth, big.NewInt(25e6)) // 25 million VET
}

type Staker struct {
	contract *bind.Contract
	revision string
}

func NewStaker(client *thorclient.Client) (*Staker, error) {
	contract, err := bind.NewContract(client, builtin.Staker.RawABI(), &builtin.Staker.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to create staker contract: %w", err)
	}
	return &Staker{
		contract: contract,
	}, nil
}

// Revision creates a new Staker instance with the specified revision.
func (s *Staker) Revision(rev string) *Staker {
	return &Staker{
		contract: s.contract,
		revision: rev,
	}
}

// FirstActive returns the first active validator
func (s *Staker) FirstActive() (*Validation, thor.Address, error) {
	out := new(common.Address)
	if err := s.contract.Method("firstActive").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, thor.Address{}, err
	}
	validator := thor.Address(*out)
	if validator.IsZero() {
		return nil, thor.Address{}, errors.New("no active validator")
	}
	v, err := s.GetValidation(validator)
	return v, validator, err
}

func (s *Staker) Raw() *bind.Contract {
	return s.contract
}

// FirstQueued returns the first queued validator
func (s *Staker) FirstQueued() (*Validation, thor.Address, error) {
	out := new(common.Address)
	if err := s.contract.Method("firstQueued").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, thor.Address{}, err
	}
	validator := thor.Address(*out)
	if validator.IsZero() {
		return nil, thor.Address{}, errors.New("no queued validator")
	}
	v, err := s.GetValidation(validator)
	return v, validator, err
}

// Next returns the next validator
func (s *Staker) Next(validator thor.Address) (*Validation, thor.Address, error) {
	out := new(common.Address)
	if err := s.contract.Method("next", validator).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, thor.Address{}, err
	}
	next := thor.Address(*out)
	if next.IsZero() {
		return nil, thor.Address{}, errors.New("no next validator")
	}
	v, err := s.GetValidation(next)
	return v, next, err
}

func (s *Staker) TotalStake() (*big.Int, *big.Int, error) {
	out := [2]any{}
	out[0] = new(*big.Int)
	out[1] = new(*big.Int)
	if err := s.contract.Method("totalStake").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, nil, err
	}
	return *(out[0].(**big.Int)), *(out[1].(**big.Int)), nil
}

func (s *Staker) QueuedStake() (*big.Int, error) {
	out := new(*big.Int)
	if err := s.contract.Method("queuedStake").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return *out, nil
}

type Validation struct {
	Address      thor.Address
	Endorser     thor.Address
	Stake        *big.Int
	Weight       *big.Int
	QueuedStake  *big.Int
	Status       StakerStatus
	OfflineBlock uint32
}

type ValidatorPeriodDetails struct {
	Address          thor.Address
	Period           uint32
	StartBlock       uint32
	ExitBlock        uint32
	CompletedPeriods uint32
}

func (v *Validation) Exists() bool {
	return !v.Endorser.IsZero() && v.Status != 0
}

func (v *Validation) IsOnline() bool {
	return v.OfflineBlock == math.MaxUint32
}

func (s *Staker) GetValidation(node thor.Address) (*Validation, error) {
	out := [7]any{}
	out[0] = new(common.Address)
	out[1] = new(*big.Int)
	out[2] = new(*big.Int)
	out[3] = new(*big.Int)
	out[4] = new(uint8)
	out[5] = new(uint32)
	if err := s.contract.Method("getValidation", node).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	validatorStake := &Validation{
		Address:      node,
		Endorser:     thor.Address(out[0].(*common.Address)[:]),
		Stake:        *(out[1].(**big.Int)),
		Weight:       *(out[2].(**big.Int)),
		QueuedStake:  *(out[3].(**big.Int)),
		Status:       StakerStatus(*(out[4].(*uint8))),
		OfflineBlock: *(out[5].(*uint32)),
	}

	return validatorStake, nil
}

func (s *Staker) GetValidationPeriodDetails(node thor.Address) (*ValidatorPeriodDetails, error) {
	out := [4]any{}
	out[0] = new(uint32)
	out[1] = new(uint32)
	out[2] = new(uint32)
	out[3] = new(uint32)
	if err := s.contract.Method("getValidationPeriodDetails", node).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	validatorPeriodDetails := &ValidatorPeriodDetails{
		Address:          node,
		Period:           *(out[0].(*uint32)),
		StartBlock:       *(out[1].(*uint32)),
		ExitBlock:        *(out[2].(*uint32)),
		CompletedPeriods: *(out[3].(*uint32)),
	}

	return validatorPeriodDetails, nil
}

func (s *Staker) AddValidation(validator thor.Address, stake *big.Int, period uint32) *bind.MethodBuilder {
	return s.contract.Method("addValidation", validator, period).WithValue(stake)
}

func (s *Staker) SignalExit(validator thor.Address) *bind.MethodBuilder {
	return s.contract.Method("signalExit", validator)
}

func (s *Staker) GetWithdrawable(validator thor.Address) (*big.Int, error) {
	out := new(big.Int)
	if err := s.contract.Method("getWithdrawable", validator).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Staker) WithdrawStake(validator thor.Address) *bind.MethodBuilder {
	return s.contract.Method("withdrawStake", validator)
}

func (s *Staker) DecreaseStake(validator thor.Address, amount *big.Int) *bind.MethodBuilder {
	return s.contract.Method("decreaseStake", validator, amount)
}

func (s *Staker) IncreaseStake(validator thor.Address, amount *big.Int) *bind.MethodBuilder {
	return s.contract.Method("increaseStake", validator).WithValue(amount)
}

func (s *Staker) SetBeneficiary(validator, beneficiary thor.Address) *bind.MethodBuilder {
	return s.contract.Method("setBeneficiary", validator, beneficiary)
}

func (s *Staker) AddDelegation(validator thor.Address, stake *big.Int, multiplier uint8) *bind.MethodBuilder {
	return s.contract.Method("addDelegation", validator, multiplier).WithValue(stake)
}

func (s *Staker) SignalDelegationExit(delegationID *big.Int) *bind.MethodBuilder {
	return s.contract.Method("signalDelegationExit", delegationID)
}

func (s *Staker) WithdrawDelegation(delegationID *big.Int) *bind.MethodBuilder {
	return s.contract.Method("withdrawDelegation", delegationID)
}

func (s *Staker) GetDelegatorsRewards(validatorID thor.Address, period uint32) (*big.Int, error) {
	out := new(big.Int)
	if err := s.contract.Method("getDelegatorsRewards", validatorID, period).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return out, nil
}

type Delegation struct {
	Validator  thor.Address
	Stake      *big.Int
	Multiplier uint8
	Locked     bool
}

type DelegationPeriodDetails struct {
	StartPeriod uint32
	EndPeriod   uint32
}

func (s *Staker) GetDelegation(delegationID *big.Int) (*Delegation, error) {
	out := make([]any, 4)
	out[0] = new(common.Address)
	out[1] = new(*big.Int)
	out[2] = new(uint8)
	out[3] = new(bool)
	if err := s.contract.Method("getDelegation", delegationID).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	delegatorStake := &Delegation{
		Validator:  thor.Address(out[0].(*common.Address)[:]),
		Stake:      *(out[1].(**big.Int)),
		Multiplier: *(out[2].(*uint8)),
		Locked:     *(out[3].(*bool)),
	}

	return delegatorStake, nil
}

func (s *Staker) GetDelegationPeriodDetails(delegationID *big.Int) (*DelegationPeriodDetails, error) {
	out := make([]any, 2)
	out[0] = new(uint32)
	out[1] = new(uint32)
	if err := s.contract.Method("getDelegationPeriodDetails", delegationID).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	delegatorPeriodDetails := &DelegationPeriodDetails{
		StartPeriod: *(out[0].(*uint32)),
		EndPeriod:   *(out[1].(*uint32)),
	}

	return delegatorPeriodDetails, nil
}

type ValidationTotals struct {
	TotalLockedStake  *big.Int
	TotalLockedWeight *big.Int
	TotalQueuedStake  *big.Int
	TotalExitingStake *big.Int
	NextPeriodWeight  *big.Int
}

func (s *Staker) GetValidationTotals(node thor.Address) (*ValidationTotals, error) {
	out := make([]any, 5)
	out[0] = new(*big.Int)
	out[1] = new(*big.Int)
	out[2] = new(*big.Int)
	out[3] = new(*big.Int)
	out[4] = new(*big.Int)
	if err := s.contract.Method("getValidationTotals", node).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	validationTotals := &ValidationTotals{
		TotalLockedStake:  *(out[0].(**big.Int)),
		TotalLockedWeight: *(out[1].(**big.Int)),
		TotalQueuedStake:  *(out[2].(**big.Int)),
		TotalExitingStake: *(out[3].(**big.Int)),
		NextPeriodWeight:  *(out[4].(**big.Int)),
	}

	return validationTotals, nil
}

func (s *Staker) GetValidationsNum() (uint64, uint64, error) {
	out := make([]any, 4)
	out[0] = new(uint64)
	out[1] = new(uint64)
	if err := s.contract.Method("getValidationsNum").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return 0, 0, err
	}

	return *(out[0].(*uint64)), *(out[1].(*uint64)), nil
}

func (s *Staker) Issuance(revision string) (*big.Int, error) {
	out := new(big.Int)
	if err := s.contract.Method("issuance").Call().AtRevision(revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return out, nil
}

type ValidationQueuedEvent struct {
	Node     thor.Address
	Endorser thor.Address
	Period   uint32
	Stake    *big.Int
	Log      api.FilteredEvent
}

type ValidatorQueuedEvent struct {
	Endorser     thor.Address
	Master       thor.Address
	ValidationID thor.Address
	Stake        *big.Int
	Period       uint32
	Log          api.FilteredEvent
}

func (s *Staker) FilterValidatorQueued(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ValidationQueuedEvent, error) {
	event, ok := s.contract.ABI().Events["ValidationQueued"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent(event.Name).WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]ValidationQueuedEvent, len(raw))
	for i, log := range raw {
		node := thor.BytesToAddress(log.Topics[1][:])     // indexed
		endorser := thor.BytesToAddress(log.Topics[2][:]) // indexed

		// non-indexed
		data := make([]any, 2)
		data[0] = new(uint32)
		data[1] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = ValidationQueuedEvent{
			Node:     node,
			Endorser: endorser,
			Period:   *(data[0].(*uint32)),
			Stake:    *(data[1].(**big.Int)),
			Log:      log,
		}
	}

	return out, nil
}

type ValidationSignaledExitEvent struct {
	Node thor.Address
	Log  api.FilteredEvent
}

func (s *Staker) FilterValidationSignaledExit(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ValidationSignaledExitEvent, error) {
	raw, err := s.contract.FilterEvent("ValidationSignaledExit").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]ValidationSignaledExitEvent, len(raw))
	for i, log := range raw {
		node := thor.BytesToAddress(log.Topics[1][:]) // indexed

		out[i] = ValidationSignaledExitEvent{
			Node: node,
			Log:  log,
		}
	}

	return out, nil
}

type DelegationAddedEvent struct {
	Validator    thor.Address
	DelegationID *big.Int
	Stake        *big.Int
	Multiplier   uint8
	Log          api.FilteredEvent
}

func (s *Staker) FilterDelegationAdded(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]DelegationAddedEvent, error) {
	event, ok := s.contract.ABI().Events["DelegationAdded"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent(event.Name).WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]DelegationAddedEvent, len(raw))
	for i, log := range raw {
		validator := thor.BytesToAddress(log.Topics[1][:])      // indexed
		delegationID := new(big.Int).SetBytes(log.Topics[2][:]) // indexed

		// non-indexed
		data := make([]any, 2)
		data[0] = new(*big.Int)
		data[1] = new(uint8)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = DelegationAddedEvent{
			Validator:    validator,
			DelegationID: delegationID,
			Stake:        *(data[0].(**big.Int)),
			Multiplier:   *(data[1].(*uint8)),
			Log:          log,
		}
	}

	return out, nil
}

type DelegationSignaledExitEvent struct {
	DelegationID *big.Int
	Log          api.FilteredEvent
}

func (s *Staker) FilterDelegationSignaledExit(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]DelegationSignaledExitEvent, error) {
	raw, err := s.contract.FilterEvent("DelegationSignaledExit").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]DelegationSignaledExitEvent, len(raw))
	for i, log := range raw {
		delegationID := new(big.Int).SetBytes(log.Topics[1][:]) // indexed
		out[i] = DelegationSignaledExitEvent{
			DelegationID: delegationID,
			Log:          log,
		}
	}

	return out, nil
}

type DelegationWithdrawnEvent struct {
	DelegationID *big.Int
	Stake        *big.Int
	Log          api.FilteredEvent
}

func (s *Staker) FilterDelegationWithdrawn(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]DelegationWithdrawnEvent, error) {
	event, ok := s.contract.ABI().Events["DelegationWithdrawn"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("DelegationWithdrawn").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]DelegationWithdrawnEvent, len(raw))
	for i, log := range raw {
		delegationID := new(big.Int).SetBytes(log.Topics[1][:]) // indexed

		// non-indexed
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = DelegationWithdrawnEvent{
			DelegationID: delegationID,
			Stake:        *(data[0].(**big.Int)),
			Log:          log,
		}
	}

	return out, nil
}

type StakeIncreasedEvent struct {
	Validator thor.Address
	Added     *big.Int
	Log       api.FilteredEvent
}

func (s *Staker) FilterStakeIncreased(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]StakeIncreasedEvent, error) {
	event, ok := s.contract.ABI().Events["StakeIncreased"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent(event.Name).WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]StakeIncreasedEvent, len(raw))
	for i, log := range raw {
		node := thor.BytesToAddress(log.Topics[1][:]) // indexed

		// non-indexed
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = StakeIncreasedEvent{
			Validator: node,
			Added:     *(data[0].(**big.Int)),
			Log:       log,
		}
	}

	return out, nil
}

type StakeDecreasedEvent struct {
	Validator thor.Address
	Removed   *big.Int
	Log       api.FilteredEvent
}

func (s *Staker) FilterStakeDecreased(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]StakeDecreasedEvent, error) {
	event, ok := s.contract.ABI().Events["StakeDecreased"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent(event.Name).WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]StakeDecreasedEvent, len(raw))
	for i, log := range raw {
		node := thor.BytesToAddress(log.Topics[1][:]) // indexed

		// non-indexed
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = StakeDecreasedEvent{
			Validator: node,
			Removed:   *(data[0].(**big.Int)),
			Log:       log,
		}
	}

	return out, nil
}

type BeneficiarySetEvent struct {
	Validator   thor.Address
	Beneficiary thor.Address
	Log         api.FilteredEvent
}

func (s *Staker) FilterBeneficiarySet(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]BeneficiarySetEvent, error) {
	event, ok := s.contract.ABI().Events["BeneficiarySet"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent(event.Name).WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]BeneficiarySetEvent, len(raw))
	for i, log := range raw {
		validator := thor.BytesToAddress(log.Topics[1][:]) // indexed

		// non-indexed
		data := make([]any, 1)
		data[0] = new(common.Address)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = BeneficiarySetEvent{
			Validator:   validator,
			Beneficiary: thor.Address(*data[0].(*common.Address)),
			Log:         log,
		}
	}

	return out, nil
}

type ValidationWithdrawnEvent struct {
	Validator thor.Address
	Stake     *big.Int
	Log       api.FilteredEvent
}

func (s *Staker) FilterValidationWithdrawn(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ValidationWithdrawnEvent, error) {
	event, ok := s.contract.ABI().Events["ValidationWithdrawn"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent(event.Name).WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]ValidationWithdrawnEvent, len(raw))
	for i, log := range raw {
		node := thor.BytesToAddress(log.Topics[1][:]) // indexed

		// non-indexed
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = ValidationWithdrawnEvent{
			Validator: node,
			Stake:     *(data[0].(**big.Int)),
			Log:       log,
		}
	}

	return out, nil
}
