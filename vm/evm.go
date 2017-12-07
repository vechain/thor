package vm

import (
	"math/big"

	"github.com/vechain/vecore/cry"
	"github.com/vechain/vecore/vm/vmlog"

	"github.com/vechain/vecore/vm/snapshot"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/vm/account"
	"github.com/vechain/vecore/vm/state"
)

// Config is ref to vm.Config.
type Config vm.Config

// EVM is a facade for ethEvm.
type EVM struct {
	ethEvm *vm.EVM
	state  *state.State
}

var chainConfig = &params.ChainConfig{
	ChainId:        big.NewInt(0),
	HomesteadBlock: big.NewInt(0),
	DAOForkBlock:   big.NewInt(0),
	DAOForkSupport: false,
	EIP150Block:    big.NewInt(0),
	EIP150Hash:     common.Hash{},
	EIP155Block:    big.NewInt(0),
	EIP158Block:    big.NewInt(0),
	ByzantiumBlock: big.NewInt(0),
	Ethash:         nil,
	Clique:         nil,
}

// NewEVM retutrns a new EVM . The returned EVM is not thread safe and should
// only ever be used *once*.
func NewEVM(ctx Context, kvReader KVReader, stateReader StateReader, vmConfig Config) *EVM {
	state := state.New(account.NewManager(kvReader, stateReader), snapshot.New(), vmlog.New())
	evm := vm.NewEVM(vm.Context(ctx), state, chainConfig, vm.Config(vmConfig))
	return &EVM{ethEvm: evm, state: state}
}

// Cancel cancels any running EVM operation.
// This may be called concurrently and it's safe to be called multiple times.
func (evm *EVM) Cancel() {
	evm.ethEvm.Cancel()
}

// Call executes the contract associated with the addr with the given input as parameters.
// It also handles any necessary value transfer required and takes the necessary steps to
// create accounts and reverses the state in case of an execution error or failed value transfer.
func (evm *EVM) Call(caller ContractRef, addr acc.Address, input []byte, gas uint64, value *big.Int) ([]byte, uint64, []*account.Account, map[cry.Hash][]byte, error) {
	ret, leftOverGas, err := evm.ethEvm.Call(&vmContractRef{caller}, common.Address(addr), input, gas, value)
	return ret, leftOverGas, evm.state.GetDirtiedAccounts(), evm.state.Preimages(), err
}

// CallCode executes the contract associated with the addr with the given input as parameters.
// It also handles any necessary value transfer required and takes the necessary steps to create
// accounts and reverses the state in case of an execution error or failed value transfer.
//
// CallCode differs from Call in the sense that it executes the given address'
// code with the caller as context.
func (evm *EVM) CallCode(caller ContractRef, addr acc.Address, input []byte, gas uint64, value *big.Int) ([]byte, uint64, []*account.Account, map[cry.Hash][]byte, error) {
	ret, leftOverGas, err := evm.ethEvm.CallCode(&vmContractRef{caller}, common.Address(addr), input, gas, value)
	return ret, leftOverGas, evm.state.GetDirtiedAccounts(), evm.state.Preimages(), err
}

// DelegateCall executes the contract associated with the addr with the given input as parameters.
// It reverses the state in case of an execution error.
//
// DelegateCall differs from CallCode in the sense that it executes the given address' code with
// the caller as context and the caller is set to the caller of the caller.
func (evm *EVM) DelegateCall(caller ContractRef, addr acc.Address, input []byte, gas uint64) ([]byte, uint64, []*account.Account, map[cry.Hash][]byte, error) {
	ret, leftOverGas, err := evm.ethEvm.DelegateCall(&vmContractRef{caller}, common.Address(addr), input, gas)
	return ret, leftOverGas, evm.state.GetDirtiedAccounts(), evm.state.Preimages(), err
}

// StaticCall executes the contract associated with the addr with the given input as parameters
// while disallowing any modifications to the state during the call.
//
// Opcodes that attempt to perform such modifications will result in exceptions instead of performing
// the modifications.
func (evm *EVM) StaticCall(caller ContractRef, addr acc.Address, input []byte, gas uint64) ([]byte, uint64, []*account.Account, map[cry.Hash][]byte, error) {
	ret, leftOverGas, err := evm.ethEvm.StaticCall(&vmContractRef{caller}, common.Address(addr), input, gas)
	return ret, leftOverGas, evm.state.GetDirtiedAccounts(), evm.state.Preimages(), err
}

// Create creates a new contract using code as deployment code.
func (evm *EVM) Create(caller ContractRef, code []byte, gas uint64, value *big.Int) ([]byte, acc.Address, uint64, []*account.Account, map[cry.Hash][]byte, error) {
	ret, contractAddr, leftOverGas, err := evm.ethEvm.Create(&vmContractRef{caller}, code, gas, value)
	return ret, acc.Address(contractAddr), leftOverGas, evm.state.GetDirtiedAccounts(), evm.state.Preimages(), err
}

// ChainConfig returns the evmironment's chain configuration
func (evm *EVM) ChainConfig() *params.ChainConfig {
	return evm.ethEvm.ChainConfig()
}
