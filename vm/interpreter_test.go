// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"
)

func GetNewInterpreter(jumpTable JumpTable) *Interpreter {
	statedb := NoopStateDB{}

	evmConfig := Config{
		Tracer:    &noopTracer{},
		JumpTable: jumpTable,
	}

	evm := NewEVM(Context{
		BlockNumber:        big.NewInt(1),
		GasPrice:           big.NewInt(1),
		CanTransfer:        NoopCanTransfer,
		Transfer:           NoopTransfer,
		NewContractAddress: newContractAddress,
	},
		statedb,
		&ChainConfig{ChainConfig: *params.TestChainConfig}, evmConfig)

	interpreter := NewInterpreter(evm, evmConfig)

	return interpreter
}

func GetNewContractFromBytecode(byteCode []byte) *Contract {
	caller := AccountRef(common.BytesToAddress([]byte{1}))
	object := AccountRef(common.BytesToAddress([]byte{2}))

	value := big.NewInt(0)        // Value being sent to the contract
	var gasLimit uint64 = 3000000 // Gas limit

	contract := NewContract(caller, object, value, gasLimit)

	contract.SetCode(common.BytesToHash([]byte{0x1}), byteCode)

	return contract
}

func TestNewInterpreter(t *testing.T) {
	interpreter := GetNewInterpreter(nil)
	assert.NotNil(t, interpreter)
}

func TestInterpreter_Run(t *testing.T) {
	interpreter := GetNewInterpreter(nil)

	// Some valid byteCode
	byteCode := []byte{0x60}

	contract := GetNewContractFromBytecode(byteCode)

	ret, err := interpreter.Run(contract, []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f})

	assert.Nil(t, ret)
	assert.Nil(t, err)
}

func TestInterpreterInvalidStack_Run(t *testing.T) {
	interpreter := GetNewInterpreter(nil)

	// Some valid byteCode
	byteCode := []byte{0xF0, 0x10, 0x60, 0x00, 0x56}

	contract := GetNewContractFromBytecode(byteCode)

	ret, err := interpreter.Run(contract, []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f})

	assert.Nil(t, ret)
	assert.NotNil(t, err)
}

func TestInterpreterInvalidOpcode_Run(t *testing.T) {
	interpreter := GetNewInterpreter(nil)

	// Some invalid byteCode
	byteCode := []byte{0xfe}

	contract := GetNewContractFromBytecode(byteCode)

	ret, err := interpreter.Run(contract, []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f})

	assert.Nil(t, ret)
	assert.NotNil(t, err)
}
