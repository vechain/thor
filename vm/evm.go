// Copyright 2014 The go-ethereum Authors
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

package vm

import (
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/vechain/thor/v2/thor"
)

// emptyCodeHash is used by create to ensure deployment is disallowed to already
// deployed contract addresses (relevant after the account abstraction).
var emptyCodeHash = common.Hash(thor.Keccak256(nil))

type (
	CanTransferFunc func(StateDB, common.Address, *big.Int) bool
	TransferFunc    func(StateDB, common.Address, common.Address, *big.Int)
	// GetHashFunc returns the nth block hash in the blockchain
	// and is used by the BLOCKHASH EVM op code.
	GetHashFunc func(uint64) common.Hash

	// NewContractAddressFunc create a new contract according to current evm context and creation counter.
	NewContractAddressFunc func(evm *EVM, counter uint32) common.Address
	// InterceptContractCallFunc intercept contract call.
	InterceptContractCallFunc func(evm *EVM, contract *Contract, readonly bool) ([]byte, error, bool)

	// OnCreateContractFunc callback when creating contract.
	OnCreateContractFunc func(evm *EVM, contractAddr common.Address, caller common.Address)

	// OnSuicideContractFunc callback when suicide contract.
	OnSuicideContractFunc func(evm *EVM, contractAddr common.Address, tokenReceiver common.Address)
)

func (evm *EVM) precompile(addr common.Address) (PrecompiledContract, bool) {
	var precompiles map[common.Address]PrecompiledContract
	switch {
	case evm.chainRules.IsShanghai:
		precompiles = PrecompiledContractsShanghai
	case evm.chainRules.IsIstanbul:
		precompiles = PrecompiledContractsIstanbul
	case evm.chainRules.IsByzantium:
		precompiles = PrecompiledContractsByzantium
	default:
		precompiles = PrecompiledContractsHomestead
	}
	p, ok := precompiles[addr]
	return p, ok
}

// Context provides the EVM with auxiliary information. Once provided
// it shouldn't be modified.
type Context struct {
	// CanTransfer returns whether the account contains
	// sufficient ether to transfer the value
	CanTransfer CanTransferFunc
	// Transfer transfers ether from one account to the other
	Transfer TransferFunc
	// GetHash returns the hash corresponding to n
	GetHash GetHashFunc

	NewContractAddress    NewContractAddressFunc
	InterceptContractCall InterceptContractCallFunc
	OnCreateContract      OnCreateContractFunc
	OnSuicideContract     OnSuicideContractFunc

	// Message information
	Origin   common.Address // Provides information for ORIGIN
	GasPrice *big.Int       // Provides information for GASPRICE

	// Block information
	Coinbase    common.Address // Provides information for COINBASE
	GasLimit    uint64         // Provides information for GASLIMIT
	BlockNumber *big.Int       // Provides information for NUMBER
	Time        *big.Int       // Provides information for TIME
	Difficulty  *big.Int       // Provides information for DIFFICULTY
	BaseFee     *big.Int       // Provides information for BASEFEE (0 if vm runs with NoBaseFee flag and 0 gas price)
}

// EVM is the Ethereum Virtual Machine base object and provides
// the necessary tools to run a contract on the given state with
// the provided context. It should be noted that any error
// generated through any of the calls should be considered a
// revert-state-and-consume-all-gas operation, no checks on
// specific errors should ever be performed. The interpreter makes
// sure that any errors generated are to be considered faulty code.
//
// The EVM should never be reused and is not thread safe.
type EVM struct {
	// Context provides auxiliary blockchain related information
	Context
	// StateDB gives access to the underlying state
	StateDB StateDB
	// Depth is the current call stack
	depth int

	// chainConfig contains information about the current chain
	chainConfig *ChainConfig
	// chain rules contains the chain rules for the current epoch
	chainRules Rules
	// virtual machine configuration options used to initialise the
	// evm.
	vmConfig Config
	// global (to this context) ethereum virtual machine
	// used throughout the execution of the tx.
	interpreter *Interpreter
	// abort is used to abort the EVM calling operations
	// NOTE: must be set atomically
	abort int32
	// callGasTemp holds the gas available for the current call. This is needed because the
	// available gas is calculated in gasCall* according to the 63/64 rule and later
	// applied in opCall*.
	callGasTemp uint64

	// contract created during execution.
	// this value is important for generating contract address.
	contractCreationCount uint32
}

// NewEVM returns a new EVM. The returned EVM is not thread safe and should
// only ever be used *once*.
func NewEVM(ctx Context, statedb StateDB, chainConfig *ChainConfig, vmConfig Config) *EVM {
	evm := &EVM{
		Context:     ctx,
		StateDB:     statedb,
		vmConfig:    vmConfig,
		chainConfig: chainConfig,
		chainRules:  chainConfig.Rules(ctx.BlockNumber),
	}

	evm.interpreter = NewInterpreter(evm, vmConfig)
	return evm
}

// Cancel cancels any running EVM operation. This may be called concurrently and
// it's safe to be called multiple times.
func (evm *EVM) Cancel() {
	atomic.StoreInt32(&evm.abort, 1)
}

// Depth returns call stack depth.
func (evm *EVM) Depth() int {
	return evm.depth
}

// Call executes the contract associated with the addr with the given input as
// parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.Tracer != nil {
		// Capture the tracer start/end events in debug mode
		if evm.depth == 0 {
			evm.vmConfig.Tracer.CaptureStart(evm, caller.Address(), addr, false, input, gas, value)

			defer func() { // Lazy evaluation of the parameters
				evm.vmConfig.Tracer.CaptureEnd(ret, gas-leftOverGas, err)
			}()
		} else {
			// Handle tracer events for entering and exiting a call frame
			evm.vmConfig.Tracer.CaptureEnter(CALL, caller.Address(), addr, input, gas, value)
			defer func() {
				evm.vmConfig.Tracer.CaptureExit(ret, gas-leftOverGas, err)
			}()
		}
	}

	return evm.call(caller, addr, input, gas, value)
}

func (evm *EVM) call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}

	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// Fail if we're trying to transfer more than the available balance
	if value.Sign() != 0 && !evm.Context.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, gas, ErrInsufficientBalance
	}

	var (
		to              = AccountRef(addr)
		snapshot        = evm.StateDB.Snapshot()
		p, isPrecompile = evm.precompile(addr)
	)
	if !evm.StateDB.Exist(addr) {
		if !isPrecompile && evm.chainRules.IsEIP158 && value.Sign() == 0 {
			return nil, gas, nil
		}
		evm.StateDB.CreateAccount(addr)
	}
	evm.Transfer(evm.StateDB, caller.Address(), to.Address(), value)

	// Initialise a new contract and set the code that is to be used by the EVM.
	// The contract is a scoped environment for this execution context only.
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	if isPrecompile {
		ret, err = RunPrecompiledContract(p, input, contract)
	} else {
		ret, err = evm.interpreter.Run(contract, input)
	}

	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

// CallCode executes the contract associated with the addr with the given input
// as parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
//
// CallCode differs from Call in the sense that it executes the given address'
// code with the caller as context.
func (evm *EVM) CallCode(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}

	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// Fail if we're trying to transfer more than the available balance
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, gas, ErrInsufficientBalance
	}

	// Invoke tracer hooks that signal entering/exiting a call frame
	if evm.vmConfig.Tracer != nil {
		evm.vmConfig.Tracer.CaptureEnter(CALLCODE, caller.Address(), addr, input, gas, value)
		defer func() {
			evm.vmConfig.Tracer.CaptureExit(ret, gas-leftOverGas, err)
		}()
	}

	var (
		snapshot        = evm.StateDB.Snapshot()
		to              = AccountRef(caller.Address())
		p, isPrecompile = evm.precompile(addr)
	)
	// initialise a new contract and set the code that is to be used by the
	// EVM. The contract is a scoped environment for this execution context
	// only.
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	if isPrecompile {
		ret, err = RunPrecompiledContract(p, input, contract)
	} else {
		ret, err = evm.interpreter.Run(contract, input)
	}

	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

// DelegateCall executes the contract associated with the addr with the given input
// as parameters. It reverses the state in case of an execution error.
//
// DelegateCall differs from CallCode in the sense that it executes the given address'
// code with the caller as context and the caller is set to the caller of the caller.
func (evm *EVM) DelegateCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}
	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}

	// Invoke tracer hooks that signal entering/exiting a call frame
	if evm.vmConfig.Tracer != nil {
		evm.vmConfig.Tracer.CaptureEnter(DELEGATECALL, caller.Address(), addr, input, gas, nil)
		defer func() {
			evm.vmConfig.Tracer.CaptureExit(ret, gas-leftOverGas, err)
		}()
	}

	var (
		snapshot        = evm.StateDB.Snapshot()
		to              = AccountRef(caller.Address())
		p, isPrecompile = evm.precompile(addr)
	)

	// Initialise a new contract and make initialise the delegate values
	contract := NewContract(caller, to, nil, gas).AsDelegate()
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	if isPrecompile {
		ret, err = RunPrecompiledContract(p, input, contract)
	} else {
		ret, err = evm.interpreter.Run(contract, input)
	}
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

// StaticCall executes the contract associated with the addr with the given input
// as parameters while disallowing any modifications to the state during the call.
// Opcodes that attempt to perform such modifications will result in exceptions
// instead of performing the modifications.
func (evm *EVM) StaticCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}
	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// Make sure the readonly is only set if we aren't in readonly yet
	// this makes also sure that the readonly flag isn't removed for
	// child calls.
	if !evm.interpreter.readOnly {
		evm.interpreter.readOnly = true
		defer func() { evm.interpreter.readOnly = false }()
	}

	// Invoke tracer hooks that signal entering/exiting a call frame
	if evm.vmConfig.Tracer != nil {
		evm.vmConfig.Tracer.CaptureEnter(STATICCALL, caller.Address(), addr, input, gas, nil)
		defer func() {
			evm.vmConfig.Tracer.CaptureExit(ret, gas-leftOverGas, err)
		}()
	}

	var (
		to              = AccountRef(addr)
		snapshot        = evm.StateDB.Snapshot()
		p, isPrecompile = evm.precompile(addr)
	)
	// Initialise a new contract and set the code that is to be used by the
	// EVM. The contract is a scoped environment for this execution context
	// only.
	contract := NewContract(caller, to, new(big.Int), gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	if isPrecompile {
		ret, err = RunPrecompiledContract(p, input, contract)
	} else {
		ret, err = evm.interpreter.Run(contract, input)
	}

	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in Homestead this also counts for code storage gas errors.
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

// Create creates a new contract using code as deployment code.
func (evm *EVM) Create(caller ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	contractAddr = evm.NewContractAddress(evm, evm.contractCreationCount)

	if evm.vmConfig.Tracer != nil {
		// Capture the tracer start/end events in debug mode
		if evm.depth == 0 {
			evm.vmConfig.Tracer.CaptureStart(evm, caller.Address(), contractAddr, true, code, gas, value)

			defer func() { // Lazy evaluation of the parameters
				evm.vmConfig.Tracer.CaptureEnd(ret, gas-leftOverGas, err)
			}()
		} else {
			// Handle tracer events for entering and exiting a call frame
			evm.vmConfig.Tracer.CaptureEnter(CREATE, caller.Address(), contractAddr, code, gas, value)
			defer func() {
				evm.vmConfig.Tracer.CaptureExit(ret, gas-leftOverGas, err)
			}()
		}
	}

	return evm.create(caller, code, gas, value, contractAddr)
}

// Create2 creates a new contract using code as deployment code.
//
// The different between Create2 with Create is Create2 uses sha3(0xff ++ msg.sender ++ salt ++ sha3(init_code))[12:]
// instead of the usual sender-and-nonce-hash as the address where the contract is initialized at.
func (evm *EVM) Create2(caller ContractRef, code []byte, gas uint64, endowment *big.Int, salt *uint256.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	// Cannot use crypto.CreateAddress2 function.
	// v1.8.14 -> v1.8.27 dependency issue. See patch.go file.
	contractAddr = CreateAddress2(caller.Address(), salt.Bytes32(), thor.Keccak256(code).Bytes())

	// Capture the tracer start/end events in debug mode
	if evm.vmConfig.Tracer != nil {
		// Handle tracer events for entering and exiting a call frame
		evm.vmConfig.Tracer.CaptureEnter(CREATE2, caller.Address(), contractAddr, code, gas, endowment)
		defer func() {
			evm.vmConfig.Tracer.CaptureExit(ret, gas-leftOverGas, err)
		}()
	}

	return evm.create(caller, code, gas, endowment, contractAddr) // endowment is value.
}

// create creates a new contract using code as deployment code.
func (evm *EVM) create(caller ContractRef, code []byte, gas uint64, value *big.Int, contractAddr common.Address) ([]byte, common.Address, uint64, error) {
	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	if evm.depth > int(params.CallCreateDepth) {
		return nil, common.Address{}, gas, ErrDepth
	}
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, common.Address{}, gas, ErrInsufficientBalance
	}
	nonce := evm.StateDB.GetNonce(caller.Address())
	evm.StateDB.SetNonce(caller.Address(), nonce+1)

	// Increase counter, same behavior as Create()
	// We already have address, just need to increase the counter.
	evm.contractCreationCount++

	// Ensure there's no existing contract already at the designated address
	contractHash := evm.StateDB.GetCodeHash(contractAddr)
	if evm.StateDB.GetNonce(contractAddr) != 0 || (contractHash != (common.Hash{}) && contractHash != emptyCodeHash) {
		return nil, common.Address{}, 0, ErrContractAddressCollision
	}
	// Create a new account on the state
	snapshot := evm.StateDB.Snapshot()
	evm.StateDB.CreateAccount(contractAddr)
	if evm.chainRules.IsEIP158 {
		evm.StateDB.SetNonce(contractAddr, 1)
	}

	if evm.OnCreateContract != nil {
		// should callback after snapshot
		evm.OnCreateContract(evm, contractAddr, caller.Address())
	}

	evm.Transfer(evm.StateDB, caller.Address(), contractAddr, value)

	// initialise a new contract and set the code that is to be used by the
	// EVM. The contract is a scoped environment for this execution context
	// only.
	contract := NewContract(caller, AccountRef(contractAddr), value, gas)
	contract.SetCallCode(&contractAddr, common.Hash(thor.Keccak256(code)), code)

	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, contractAddr, gas, nil
	}

	ret, err := evm.interpreter.Run(contract, nil)

	// Check whether the max code size has been exceeded, assign err if the case.
	if err == nil && evm.chainRules.IsEIP158 && len(ret) > params.MaxCodeSize {
		err = ErrMaxCodeSizeExceeded
	}

	// Reject code starting with 0xEF if EIP-3541 is enabled.
	if err == nil && len(ret) >= 1 && ret[0] == 0xEF && evm.chainRules.IsShanghai {
		err = ErrInvalidCode
	}

	// if the contract creation ran successfully and no errors were returned
	// calculate the gas required to store the code. If the code could not
	// be stored due to not enough gas set an error and let it be handled
	// by the error checking condition below.
	if err == nil {
		createDataGas := uint64(len(ret)) * params.CreateDataGas
		if contract.UseGas(createDataGas) {
			evm.StateDB.SetCode(contractAddr, ret)
		} else {
			err = ErrCodeStoreOutOfGas
		}
	}

	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if err != nil && (evm.chainRules.IsHomestead || err != ErrCodeStoreOutOfGas) {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}

	return ret, contractAddr, contract.Gas, err
}

// ChainConfig returns the environment's chain configuration
func (evm *EVM) ChainConfig() *ChainConfig { return evm.chainConfig }

// Interpreter returns the EVM interpreter
func (evm *EVM) Interpreter() *Interpreter { return evm.interpreter }
