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

func GetNewInterpreter(jumpTable *JumpTable) *Interpreter {
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

func TestInterpreterMcopy_Run(t *testing.T) {
	jt := NewDencunInstructionSet()
	interpreter := GetNewInterpreter(jt)

	// EIP-5656 test: MCOPY 0 32 32
	// Stores 0x000102...1e1f at memory[32..63], then copies to memory[0..31], returns memory[0..31].
	byteCode := []byte{
		// PUSH32 0x000102...1e1f
		byte(PUSH32),
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		byte(PUSH1), 0x20, // dst offset 32
		byte(MSTORE),
		byte(PUSH1), 0x20, // length = 32
		byte(PUSH1), 0x20, // src = 32
		byte(PUSH1), 0x00, // dst = 0
		byte(MCOPY),
		byte(PUSH1), 0x20, // return size = 32
		byte(PUSH1), 0x00, // return offset = 0
		byte(RETURN),
	}

	contract := GetNewContractFromBytecode(byteCode)
	ret, err := interpreter.Run(contract, nil)

	assert.NoError(t, err)
	expected := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	}
	assert.Equal(t, expected, ret)
}

func TestInterpreterMcopyRunWithOverflow(t *testing.T) {
	jt := NewDencunInstructionSet()
	interpreter := GetNewInterpreter(jt)

	testCases := []struct {
		name          string
		typeCode      []byte
		expectedError string
	}{
		{
			name:          "no error",
			typeCode:      []byte{byte(PUSH1), 0x00, byte(PUSH1), 0x00, byte(PUSH1), 0x00, byte(MCOPY), byte(STOP)},
			expectedError: "",
		},
		{
			name:          "length overflow",
			typeCode:      []byte{byte(PUSH1), 0x01, byte(PUSH1), 0x00, byte(PUSH8), 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, byte(MCOPY), byte(STOP)},
			expectedError: ErrGasUintOverflow.Error(),
		},
	}

	for _, tt := range testCases {
		contract := GetNewContractFromBytecode(tt.typeCode)
		ret, err := interpreter.Run(contract, nil)
		if tt.expectedError != "" {
			assert.ErrorIs(t, err, ErrGasUintOverflow)
		} else {
			assert.NoError(t, err)
			assert.Nil(t, ret)
		}
	}
}
