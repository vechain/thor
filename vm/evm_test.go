// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

var _ Logger = (*noopTracer)(nil)

type noopTracer struct{}

func (t *noopTracer) CaptureStart(_ *EVM, _ common.Address, _ common.Address, _ bool, _ []byte, _ uint64, _ *big.Int) {
}
func (t *noopTracer) CaptureEnd(_ []byte, _ uint64, _ error) {
}
func (t *noopTracer) CaptureState(_ uint64, _ OpCode, _, _ uint64, _ *Memory, _ *Stack, _ *Contract, _ []byte, _ int, _ error) {
}
func (t *noopTracer) CaptureFault(_ uint64, _ OpCode, _, _ uint64, _ *Memory, _ *Stack, _ *Contract, _ int, _ error) {
}
func (t *noopTracer) CaptureEnter(_ OpCode, _ common.Address, _ common.Address, _ []byte, _ uint64, _ *big.Int) {
}
func (t *noopTracer) CaptureExit(_ []byte, _ uint64, _ error) {
}
func (*noopTracer) CaptureClauseStart(_ uint64) {}
func (*noopTracer) CaptureClauseEnd(_ uint64)   {}

func setupEvmTestContract(codeAddr *common.Address) (*EVM, *Contract) {
	statedb := NoopStateDB{}

	evmConfig := Config{
		Tracer: &noopTracer{},
	}

	evm := NewEVM(
		Context{
			BlockNumber:        big.NewInt(1),
			GasPrice:           big.NewInt(1),
			CanTransfer:        NoopCanTransfer,
			Transfer:           NoopTransfer,
			NewContractAddress: newContractAddress,
		},
		statedb,
		&ChainConfig{ChainConfig: *params.TestChainConfig}, evmConfig)

	contract := &Contract{
		CallerAddress: common.HexToAddress("0x01"),
		Code:          []byte{0x60, 0x02, 0x5b, 0x00},
		CodeHash:      common.HexToHash("somehash"),
		CodeAddr:      codeAddr,
		Gas:           500000,
		DelegateCall:  true,
	}

	contractCode := []byte{0x60, 0x00}
	contract.SetCode(common.BytesToHash(contractCode), contractCode)

	return evm, contract
}

func TestCall(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	caller := AccountRef(common.HexToAddress("0x01"))
	contractAddr := common.HexToAddress("0x1")
	input := []byte{}

	ret, leftOverGas, err := evm.Call(caller, contractAddr, input, 1000000, big.NewInt(100000))

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestCallCode(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	caller := AccountRef(common.HexToAddress("0x01"))
	contractAddr := common.HexToAddress("0x1")
	input := []byte{}

	ret, leftOverGas, err := evm.CallCode(caller, contractAddr, input, 1000000, big.NewInt(100000))

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestDelegateCall(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01")
	objectAddress := common.HexToAddress("0x03")
	input := []byte{}

	parentContract := NewContract(AccountRef(parentCallerAddress), AccountRef(parentCallerAddress), big.NewInt(2000), 5000)
	childContract := NewContract(parentContract, AccountRef(objectAddress), big.NewInt(2000), 5000)

	ret, leftOverGas, err := evm.DelegateCall(childContract, parentContract.CallerAddress, input, 1000000)

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestStaticCall(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01")
	objectAddress := common.HexToAddress("0x03")
	input := []byte{}

	parentContract := NewContract(AccountRef(parentCallerAddress), AccountRef(parentCallerAddress), big.NewInt(2000), 5000)
	childContract := NewContract(parentContract, AccountRef(objectAddress), big.NewInt(2000), 5000)

	ret, leftOverGas, err := evm.StaticCall(childContract, parentContract.CallerAddress, input, 1000000)

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestCreate(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01234567A")
	input := []byte{}

	ret, addr, leftOverGas, err := evm.Create(AccountRef(parentCallerAddress), input, 1000000, big.NewInt(2000))

	assert.Nil(t, err)
	assert.NotNil(t, addr)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestMaxCodeSize(t *testing.T) {
	parentCallerAddress := common.HexToAddress("0x01234567A")

	type testCase struct {
		name        string
		input       []byte
		expectedErr error
	}

	testCases := []testCase{
		{
			name:        "Empty code",
			input:       []byte{},
			expectedErr: nil,
		},
		{
			name:        "Code size below max code size",
			input:       bytecodeBelowLimit,
			expectedErr: nil,
		},
		{
			name:        "Code size equal to max code size",
			input:       bytecodeSameLimit,
			expectedErr: nil,
		},
		{
			name:        "Code size greater than max code size",
			input:       bytecodeLimitPlusOne,
			expectedErr: ErrMaxCodeSizeExceeded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			codeAddr := common.BytesToAddress([]byte{1})
			evm, _ := setupEvmTestContract(&codeAddr)

			_, _, _, err := evm.Create(AccountRef(parentCallerAddress), tc.input, 10000000, big.NewInt(0))

			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestCreate2(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01234567A")
	input := []byte{}

	ret, addr, leftOverGas, err := evm.Create2(AccountRef(parentCallerAddress), input, 10000, big.NewInt(2000), uint256.NewInt(10000))

	assert.Nil(t, err)
	assert.NotNil(t, addr)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}
