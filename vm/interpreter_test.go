package vm_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/tracers"
	"github.com/vechain/thor/v2/vm"
)

func GetNewInterpreter(jumpTable vm.JumpTable) *vm.Interpreter {
	statedb := vm.NoopStateDB{}

	tracer, err := tracers.DefaultDirectory.New("noopTracer", json.RawMessage(`{}`), false)
	if err != nil {
		panic("failed to create noopTracer: " + err.Error())
	}

	evmLogger, ok := tracer.(vm.Logger)
	if !ok {
		panic("noopTracer does not implement vm.Logger")
	}

	evmConfig := vm.Config{
		Tracer:    evmLogger,
		JumpTable: jumpTable,
	}

	evm := vm.NewEVM(vm.Context{
		BlockNumber:        big.NewInt(1),
		GasPrice:           big.NewInt(1),
		CanTransfer:        vm.NoopCanTransfer,
		Transfer:           vm.NoopTransfer,
		NewContractAddress: newContractAddress,
	},
		statedb,
		&vm.ChainConfig{ChainConfig: *params.TestChainConfig}, evmConfig)

	interpreter := vm.NewInterpreter(evm, evmConfig)

	return interpreter
}

func GetNewContractFromBytecode(byteCode []byte) *vm.Contract {
	caller := vm.AccountRef(common.BytesToAddress([]byte{1}))
	object := vm.AccountRef(common.BytesToAddress([]byte{2}))

	value := big.NewInt(0)        // Value being sent to the contract
	var gasLimit uint64 = 3000000 // Gas limit

	contract := vm.NewContract(caller, object, value, gasLimit)

	contract.SetCode(common.BytesToHash([]byte{0x1}), byteCode)

	return contract
}

type account struct{}

func (account) SubBalance(amount *big.Int)                          {}
func (account) AddBalance(amount *big.Int)                          {}
func (account) SetAddress(common.Address)                           {}
func (account) Value() *big.Int                                     { return nil }
func (account) SetBalance(*big.Int)                                 {}
func (account) SetNonce(uint64)                                     {}
func (account) Balance() *big.Int                                   { return nil }
func (account) Address() common.Address                             { return common.Address{} }
func (account) SetCode(common.Hash, []byte)                         {}
func (account) ForEachStorage(cb func(key, value common.Hash) bool) {}

func TestNewInterpreter(t *testing.T) {

	interpreter := GetNewInterpreter(nil)
	assert.NotNil(t, interpreter)
}

func TestInterpreter_Run(t *testing.T) {

	interpreter := GetNewInterpreter(nil)

	// Some valid byteCode
	byteCode := []byte{0x60}

	contract := GetNewContractFromBytecode(byteCode)

	interpreter.Run(contract, []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f})
}

func TestInterpreterInvalidStack_Run(t *testing.T) {

	interpreter := GetNewInterpreter(nil)

	// Some valid byteCode
	byteCode := []byte{0xF0, 0x10, 0x60, 0x00, 0x56}

	contract := GetNewContractFromBytecode(byteCode)

	interpreter.Run(contract, []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f})
}

func TestInterpreterInvalidOpcode_Run(t *testing.T) {

	interpreter := GetNewInterpreter(nil)

	// Some invalid byteCode
	byteCode := []byte{0xfe}

	contract := GetNewContractFromBytecode(byteCode)

	interpreter.Run(contract, []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f})

}
