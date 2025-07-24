// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	_ "embed"
	"errors"
	"fmt"
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
func (s *Staker) FirstActive() (*Validator, thor.Address, error) {
	out := new(common.Address)
	if err := s.contract.Method("firstActive").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, thor.Address{}, err
	}
	res := *out
	id := thor.Address(res[:])
	if id.IsZero() {
		return nil, thor.Address{}, errors.New("no active validator")
	}
	v, err := s.Get(id)
	return v, id, err
}

func (s *Staker) Raw() *bind.Contract {
	return s.contract
}

// FirstQueued returns the first queued validator
func (s *Staker) FirstQueued() (*Validator, thor.Address, error) {
	out := new(common.Address)
	if err := s.contract.Method("firstQueued").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, thor.Address{}, err
	}
	res := *out
	id := thor.Address(res[:])
	if id.IsZero() {
		return nil, thor.Address{}, errors.New("no queued validator")
	}
	v, err := s.Get(id)
	return v, id, err
}

// Next returns the next validator
func (s *Staker) Next(id thor.Address) (*Validator, thor.Address, error) {
	out := new(common.Address)
	if err := s.contract.Method("next", id).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, thor.Address{}, err
	}
	res := *out
	next := thor.Address(res[:])
	if next.IsZero() {
		return nil, thor.Address{}, errors.New("no next validator")
	}
	v, err := s.Get(id)
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

func (s *Staker) QueuedStake() (*big.Int, *big.Int, error) {
	out := [2]any{}
	out[0] = new(*big.Int)
	out[1] = new(*big.Int)
	if err := s.contract.Method("queuedStake").Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, nil, err
	}
	return *(out[0].(**big.Int)), *(out[1].(**big.Int)), nil
}

type Validator struct {
	Master     *thor.Address
	Endorsor   *thor.Address
	Stake      *big.Int
	Weight     *big.Int
	Status     StakerStatus
	Online     bool
	Period     uint32
	StartBlock uint32
	ExitBlock  uint32
}

func (v *Validator) Exists() bool {
	return v.Endorsor != nil && !v.Endorsor.IsZero() && v.Status != 0
}

func (s *Staker) Get(id thor.Address) (*Validator, error) {
	out := [9]any{}
	out[0] = new(common.Address)
	out[1] = new(common.Address)
	out[2] = new(*big.Int)
	out[3] = new(*big.Int)
	out[4] = new(uint8)
	out[5] = new(bool)
	out[6] = new(uint32)
	out[7] = new(uint32)
	out[8] = new(uint32)
	if err := s.contract.Method("get", id).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	validator := &Validator{
		Master:     (*thor.Address)(out[0].(*common.Address)),
		Endorsor:   (*thor.Address)(out[1].(*common.Address)),
		Stake:      *(out[2].(**big.Int)),
		Weight:     *(out[3].(**big.Int)),
		Status:     StakerStatus(*(out[4].(*uint8))),
		Online:     *(out[5].(*bool)),
		Period:     *(out[6].(*uint32)),
		StartBlock: *(out[7].(*uint32)),
		ExitBlock:  *(out[8].(*uint32)),
	}

	return validator, nil
}

func (s *Staker) AddValidator(master thor.Address, stake *big.Int, period uint32) *bind.MethodBuilder {
	return s.contract.Method("addValidator", master, period).WithValue(stake)
}

func (s *Staker) AddDelegation(validationID thor.Address, stake *big.Int, autoRenew bool, multiplier uint8) *bind.MethodBuilder {
	return s.contract.Method("addDelegation", validationID, autoRenew, multiplier).WithValue(stake)
}

func (s *Staker) UpdateDelegationAutoRenew(delegationID thor.Bytes32, autoRenew bool) *bind.MethodBuilder {
	return s.contract.Method("updateDelegationAutoRenew", delegationID, autoRenew)
}

func (s *Staker) SignalExit(validationID thor.Address) *bind.MethodBuilder {
	return s.contract.Method("signalExit", validationID)
}

func (s *Staker) WithdrawDelegation(delegationID thor.Bytes32) *bind.MethodBuilder {
	return s.contract.Method("withdrawDelegation", delegationID)
}

func (s *Staker) WithdrawStake(validationID thor.Address) *bind.MethodBuilder {
	return s.contract.Method("withdrawStake", validationID)
}

func (s *Staker) DecreaseStake(validationID thor.Address, amount *big.Int) *bind.MethodBuilder {
	return s.contract.Method("decreaseStake", validationID, amount)
}

func (s *Staker) IncreaseStake(validationID thor.Address, amount *big.Int) *bind.MethodBuilder {
	return s.contract.Method("increaseStake", validationID).WithValue(amount)
}

func (s *Staker) GetWithdrawable(validationID thor.Address) (*big.Int, error) {
	out := new(big.Int)
	if err := s.contract.Method("getWithdrawable", validationID).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Staker) GetRewards(validatorID thor.Address, period uint32) (*big.Int, error) {
	out := new(big.Int)
	if err := s.contract.Method("getRewards", validatorID, period).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Staker) GetCompletedPeriods(validatorID thor.Address) (*uint32, error) {
	out := uint32(0)
	if err := s.contract.Method("getCompletedPeriods", validatorID).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

type Delegation struct {
	ValidationID thor.Address
	Stake        *big.Int
	StartPeriod  uint32
	EndPeriod    uint32
	Multiplier   uint8
	AutoRenew    bool
	Locked       bool
}

func (s *Staker) GetDelegation(delegationID thor.Bytes32) (*Delegation, error) {
	out := make([]any, 7)
	out[0] = new(common.Address)
	out[1] = new(*big.Int)
	out[2] = new(uint32)
	out[3] = new(uint32)
	out[4] = new(uint8)
	out[5] = new(bool)
	out[6] = new(bool)
	if err := s.contract.Method("getDelegation", delegationID).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	delegatorInfo := &Delegation{
		ValidationID: thor.Address(out[0].(*common.Address)[:]),
		Stake:        *(out[1].(**big.Int)),
		StartPeriod:  *(out[2].(*uint32)),
		EndPeriod:    *(out[3].(*uint32)),
		Multiplier:   *(out[4].(*uint8)),
		AutoRenew:    *(out[5].(*bool)),
		Locked:       *(out[6].(*bool)),
	}

	return delegatorInfo, nil
}

type ValidationTotals struct {
	TotalLockedStake        *big.Int
	TotalLockedWeight       *big.Int
	DelegationsLockedStake  *big.Int
	DelegationsLockedWeight *big.Int
}

func (s *Staker) GetValidatorsTotals(validationID thor.Address) (*ValidationTotals, error) {
	out := make([]any, 4)
	out[0] = new(*big.Int)
	out[1] = new(*big.Int)
	out[2] = new(*big.Int)
	out[3] = new(*big.Int)
	if err := s.contract.Method("getValidatorTotals", validationID).Call().AtRevision(s.revision).ExecuteInto(&out); err != nil {
		return nil, err
	}
	validationTotals := &ValidationTotals{
		TotalLockedStake:        *(out[0].(**big.Int)),
		TotalLockedWeight:       *(out[1].(**big.Int)),
		DelegationsLockedStake:  *(out[2].(**big.Int)),
		DelegationsLockedWeight: *(out[3].(**big.Int)),
	}

	return validationTotals, nil
}

type ValidatorQueuedEvent struct {
	Endorsor     thor.Address
	Master       thor.Address
	ValidationID thor.Address
	Stake        *big.Int
	Period       uint32
	AutoRenew    bool
	Log          api.FilteredEvent
}

func (s *Staker) FilterValidatorQueued(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ValidatorQueuedEvent, error) {
	event, ok := s.contract.ABI().Events["ValidatorQueued"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("ValidatorQueued").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]ValidatorQueuedEvent, len(raw))
	for i, log := range raw {
		endorsor := thor.BytesToAddress(log.Topics[1][:]) // indexed
		master := thor.BytesToAddress(log.Topics[2][:])   // indexed

		// non-indexed
		data := make([]any, 3)
		data[0] = new(uint32)
		data[1] = new(*big.Int)
		data[2] = new(bool)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = ValidatorQueuedEvent{
			Endorsor:     endorsor,
			Master:       master,
			ValidationID: master,
			Period:       *(data[0].(*uint32)),
			Stake:        *(data[1].(**big.Int)),
			AutoRenew:    *(data[2].(*bool)),
			Log:          log,
		}
	}

	return out, nil
}

type ValidatorSignaledExitEvent struct {
	Endorsor     thor.Address
	ValidationID thor.Bytes32
	Log          api.FilteredEvent
}

func (s *Staker) FilterValidatorSignaledExit(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ValidatorSignaledExitEvent, error) {
	raw, err := s.contract.FilterEvent("ValidatorSignaledExit").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]ValidatorSignaledExitEvent, len(raw))
	for i, log := range raw {
		endorsor := thor.BytesToAddress(log.Topics[1][:]) // indexed
		validationID := thor.Bytes32(log.Topics[2][:])    // indexed

		out[i] = ValidatorSignaledExitEvent{
			Endorsor:     endorsor,
			ValidationID: validationID,
			Log:          log,
		}
	}

	return out, nil
}

type DelegationAddedEvent struct {
	ValidationID thor.Bytes32
	DelegationID thor.Bytes32
	Stake        *big.Int
	AutoRenew    bool
	Multiplier   uint8
	Log          api.FilteredEvent
}

func (s *Staker) FilterDelegationAdded(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]DelegationAddedEvent, error) {
	event, ok := s.contract.ABI().Events["DelegationAdded"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("DelegationAdded").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]DelegationAddedEvent, len(raw))
	for i, log := range raw {
		validationID := thor.Bytes32(log.Topics[1][:]) // indexed
		delegationID := thor.Bytes32(log.Topics[2][:]) // indexed

		// non-indexed
		data := make([]any, 4)
		data[0] = new(*big.Int)
		data[1] = new(bool)
		data[2] = new(uint8)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = DelegationAddedEvent{
			ValidationID: validationID,
			DelegationID: delegationID,
			Stake:        *(data[0].(**big.Int)),
			AutoRenew:    *(data[1].(*bool)),
			Multiplier:   *(data[2].(*uint8)),
			Log:          log,
		}
	}

	return out, nil
}

type DelegationUpdatedAutoRenewEvent struct {
	DelegationID thor.Bytes32
	AutoRenew    bool
	Log          api.FilteredEvent
}

func (s *Staker) FilterDelegationUpdatedAutoRenew(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]DelegationUpdatedAutoRenewEvent, error) {
	event, ok := s.contract.ABI().Events["DelegationUpdatedAutoRenew"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("DelegationUpdatedAutoRenew").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]DelegationUpdatedAutoRenewEvent, len(raw))
	for i, log := range raw {
		delegationID := thor.Bytes32(log.Topics[1][:])

		// non-indexed
		data := make([]any, 1)
		data[0] = new(bool)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = DelegationUpdatedAutoRenewEvent{
			DelegationID: delegationID,
			AutoRenew:    *(data[0].(*bool)),
			Log:          log,
		}
	}

	return out, nil
}

type DelegationWithdrawnEvent struct {
	DelegationID thor.Bytes32
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
		delegationID := thor.Bytes32(log.Topics[1][:]) // indexed

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
	Endorsor     thor.Address
	ValidationID thor.Address
	Added        *big.Int
	Log          api.FilteredEvent
}

func (s *Staker) FilterStakeIncreased(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]StakeIncreasedEvent, error) {
	event, ok := s.contract.ABI().Events["StakeIncreased"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("StakeIncreased").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]StakeIncreasedEvent, len(raw))
	for i, log := range raw {
		endorsor := thor.BytesToAddress(log.Topics[1][:])     // indexed
		validationID := thor.BytesToAddress(log.Topics[2][:]) // indexed

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
			Endorsor:     endorsor,
			ValidationID: validationID,
			Added:        *(data[0].(**big.Int)),
			Log:          log,
		}
	}

	return out, nil
}

type StakeDecreasedEvent struct {
	Endorsor     thor.Address
	ValidationID thor.Address
	Removed      *big.Int
	Log          api.FilteredEvent
}

func (s *Staker) FilterStakeDecreased(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]StakeDecreasedEvent, error) {
	event, ok := s.contract.ABI().Events["StakeDecreased"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("StakeDecreased").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]StakeDecreasedEvent, len(raw))
	for i, log := range raw {
		endorsor := thor.BytesToAddress(log.Topics[1][:])     // indexed
		validationID := thor.BytesToAddress(log.Topics[2][:]) // indexed

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
			Endorsor:     endorsor,
			ValidationID: validationID,
			Removed:      *(data[0].(**big.Int)),
			Log:          log,
		}
	}

	return out, nil
}

type ValidatorWithdrawnEvent struct {
	Endorsor     thor.Address
	ValidationID thor.Bytes32
	Stake        *big.Int
	Log          api.FilteredEvent
}

func (s *Staker) FilterValidatorWithdrawn(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ValidatorWithdrawnEvent, error) {
	event, ok := s.contract.ABI().Events["ValidatorWithdrawn"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := s.contract.FilterEvent("ValidatorWithdrawn").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]ValidatorWithdrawnEvent, len(raw))
	for i, log := range raw {
		endorsor := thor.BytesToAddress(log.Topics[1][:]) // indexed
		validationID := thor.Bytes32(log.Topics[2][:])    // indexed

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

		out[i] = ValidatorWithdrawnEvent{
			Endorsor:     endorsor,
			ValidationID: validationID,
			Stake:        *(data[0].(**big.Int)),
			Log:          log,
		}
	}

	return out, nil
}
