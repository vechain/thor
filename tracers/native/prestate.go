// Copyright 2022 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package native

import (
	"bytes"
	"encoding/json"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/vm"
)

//go:generate go run github.com/fjl/gencodec -type account -field-override accountMarshaling -out gen_account_json.go

func init() {
	tracers.DefaultDirectory.Register("prestateTracer", newPrestateTracer, false)
}

type state = map[common.Address]*account

type account struct {
	Balance *big.Int                    `json:"balance,omitempty"`
	Energy  *big.Int                    `json:"energy,omitempty"`
	Code    []byte                      `json:"code,omitempty"`
	Storage map[common.Hash]common.Hash `json:"storage,omitempty"`
}

func (a *account) exists() bool {
	return len(a.Code) > 0 || len(a.Storage) > 0 || (a.Balance != nil && a.Balance.Sign() != 0) || (a.Energy != nil && a.Energy.Sign() != 0)
}

type accountMarshaling struct {
	Balance *hexutil.Big
	Code    hexutil.Bytes
}

type prestateTracer struct {
	noopTracer
	env                   *vm.EVM
	ctx                   *tracers.Context
	pre                   state
	post                  state
	create                bool
	to                    common.Address
	gasLimit              uint64 // Amount of gas bought for the whole tx
	config                prestateTracerConfig
	interrupt             atomic.Value // Atomic flag to signal execution interruption
	reason                error        // Textual reason for the interruption
	contractCreationCount uint32
	created               map[common.Address]bool
	deleted               map[common.Address]bool
}

type prestateTracerConfig struct {
	DiffMode bool `json:"diffMode"` // If true, this tracer will return state modifications
}

func newPrestateTracer(cfg json.RawMessage) (tracers.Tracer, error) {
	var config prestateTracerConfig
	if cfg != nil {
		if err := json.Unmarshal(cfg, &config); err != nil {
			return nil, err
		}
	}
	return &prestateTracer{
		pre:     state{},
		post:    state{},
		config:  config,
		created: make(map[common.Address]bool),
		deleted: make(map[common.Address]bool),
	}, nil
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *prestateTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.env = env
	t.create = create
	t.to = to

	t.lookupAccount(from)
	t.lookupAccount(to)
	t.lookupAccount(env.Context.Coinbase)
	// tracer hooks run before value transfer, no need to touch balance
	if create {
		t.contractCreationCount++
		if t.config.DiffMode {
			t.created[to] = true
		}
	}
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *prestateTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if t.config.DiffMode {
		return
	}

	if t.create {
		// Keep existing account prior to contract creation at that address
		if s := t.pre[t.to]; s != nil && !s.exists() {
			// Exclude newly created contract.
			delete(t.pre, t.to)
		}
	}
}

func (t *prestateTracer) CaptureClauseStart(gasLimit uint64) {
	t.gasLimit = gasLimit
}

func (t *prestateTracer) CaptureClauseEnd(restGas uint64) {
	if !t.config.DiffMode {
		return
	}

	for addr, state := range t.pre {
		// The deleted account's state is pruned from `post` but kept in `pre`
		if _, ok := t.deleted[addr]; ok {
			continue
		}
		modified := false
		postAccount := &account{Storage: make(map[common.Hash]common.Hash)}
		newBalance := t.env.StateDB.GetBalance(addr)
		newCode := t.env.StateDB.GetCode(addr)

		energy, err := t.ctx.State.GetEnergy(thor.Address(addr), t.ctx.BlockTime)
		if err != nil {
			// panic state errors, will be recovered by runtime
			panic(err)
		}
		newEnergy := energy

		if newBalance.Cmp(t.pre[addr].Balance) != 0 {
			modified = true
			postAccount.Balance = newBalance
		}
		if newEnergy.Cmp(t.pre[addr].Energy) != 0 {
			modified = true
			postAccount.Energy = newEnergy
		}
		if !bytes.Equal(newCode, t.pre[addr].Code) {
			modified = true
			postAccount.Code = newCode
		}

		for key, val := range state.Storage {
			// don't include the empty slot
			if val == (common.Hash{}) {
				delete(t.pre[addr].Storage, key)
			}

			newVal := t.env.StateDB.GetState(addr, key)
			if val == newVal {
				// Omit unchanged slots
				delete(t.pre[addr].Storage, key)
			} else {
				modified = true
				if newVal != (common.Hash{}) {
					postAccount.Storage[key] = newVal
				}
			}
		}

		if modified {
			t.post[addr] = postAccount
		} else {
			// if state is not modified, then no need to include into the pre state
			delete(t.pre, addr)
		}
	}
	// the new created contracts' prestate were empty, so delete them
	for a := range t.created {
		// the created contract maybe exists in statedb before the creating tx
		if s := t.pre[a]; s != nil && !s.exists() {
			delete(t.pre, a)
		}
	}
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *prestateTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, memory *vm.Memory, stack *vm.Stack, contract *vm.Contract, rData []byte, depth int, err error) {
	if err != nil {
		return
	}
	// Skip if tracing was interrupted
	if stop := t.interrupt.Load(); stop != nil && stop.(bool) {
		return
	}
	stackData := stack.Data()
	stackLen := len(stackData)
	caller := contract.Address()
	switch {
	case stackLen >= 1 && (op == vm.SLOAD || op == vm.SSTORE):
		slot := common.Hash(stackData[stackLen-1].Bytes32())
		t.lookupStorage(caller, slot)
	case stackLen >= 1 && (op == vm.EXTCODECOPY || op == vm.EXTCODEHASH || op == vm.EXTCODESIZE || op == vm.BALANCE || op == vm.SELFDESTRUCT):
		addr := common.Address(stackData[stackLen-1].Bytes20())
		t.lookupAccount(addr)
		if op == vm.SELFDESTRUCT {
			t.deleted[caller] = true
		}
	case stackLen >= 5 && (op == vm.DELEGATECALL || op == vm.CALL || op == vm.STATICCALL || op == vm.CALLCODE):
		addr := common.Address(stackData[stackLen-2].Bytes20())
		t.lookupAccount(addr)
	case op == vm.CREATE:
		addr := t.env.NewContractAddress(t.env, t.contractCreationCount)
		t.contractCreationCount++
		t.lookupAccount(addr)
		t.created[addr] = true
	case stackLen >= 4 && op == vm.CREATE2:
		offset := stackData[stackLen-2]
		size := stackData[stackLen-3]
		init, err := tracers.GetMemoryCopyPadded(memory, int64(offset.Uint64()), int64(size.Uint64()))
		if err != nil {
			return
		}
		inithash := crypto.Keccak256(init)
		salt := stackData[stackLen-4]
		addr := vm.CreateAddress2(contract.Address(), salt.Bytes32(), inithash)
		t.lookupAccount(addr)
		t.created[addr] = true
	}
}

// SetContext set the tracer context
func (t *prestateTracer) SetContext(ctx *tracers.Context) {
	t.ctx = ctx
}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *prestateTracer) GetResult() (json.RawMessage, error) {
	var res []byte
	var err error
	if t.config.DiffMode {
		res, err = json.Marshal(struct {
			Post state `json:"post"`
			Pre  state `json:"pre"`
		}{t.post, t.pre})
	} else {
		res, err = json.Marshal(t.pre)
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(res), t.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *prestateTracer) Stop(err error) {
	t.reason = err
	t.interrupt.Store(true)
}

// lookupAccount fetches details of an account and adds it to the prestate
// if it doesn't exist there.
func (t *prestateTracer) lookupAccount(addr common.Address) {
	if _, ok := t.pre[addr]; ok {
		return
	}

	energy, err := t.ctx.State.GetEnergy(thor.Address(addr), t.ctx.BlockTime)
	if err != nil {
		// panic state errors, will be recovered by runtime
		panic(err)
	}
	t.pre[addr] = &account{
		Balance: t.env.StateDB.GetBalance(addr),
		Energy:  energy,
		Code:    t.env.StateDB.GetCode(addr),
		Storage: make(map[common.Hash]common.Hash),
	}
}

// lookupStorage fetches the requested storage slot and adds
// it to the prestate of the given contract. It assumes `lookupAccount`
// has been performed on the contract before.
func (t *prestateTracer) lookupStorage(addr common.Address, key common.Hash) {
	if _, ok := t.pre[addr].Storage[key]; ok {
		return
	}
	t.pre[addr].Storage[key] = t.env.StateDB.GetState(addr, key)
}
