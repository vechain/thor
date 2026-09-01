// Copyright 2017 The go-ethereum Authors
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
	"reflect"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func newContractAddress(_ *EVM, _ uint32) common.Address {
	return common.HexToAddress("0x012345657ABC")
}

func GetFunctionArguments() (*EVM, *Stack) {
	statedb := NoopStateDB{}

	evm := NewEVM(Context{
		BlockNumber:        big.NewInt(1),
		GasPrice:           big.NewInt(1),
		CanTransfer:        NoopCanTransfer,
		Transfer:           NoopTransfer,
		NewContractAddress: newContractAddress,
	},
		statedb,
		&ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})

	stack := &Stack{}
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))

	return evm, stack
}

func TestMemoryGasCost(t *testing.T) {
	size := uint64(0xffffffffe0)
	v, err := memoryGasCost(&Memory{}, size)
	if err != nil {
		t.Error("didn't expect error:", err)
	}
	if v != 36028899963961341 {
		t.Errorf("Expected: 36028899963961341, got %d", v)
	}

	_, err = memoryGasCost(&Memory{}, size+1)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGasFunctions(t *testing.T) {
	evm, stack := GetFunctionArguments()

	// Define the function signature
	type gasFuncType func(params.GasTable, *EVM, *Contract, *Stack, *Memory, uint64) (uint64, error)

	// Create a struct to hold a function reference and its expected result
	type testItem struct {
		function   gasFuncType
		memerySize uint64
		expected   uint64
	}

	// Create a list of functions to test
	tests := []testItem{
		{gasCallDataCopy, 0, uint64(0x1800000000000003)},
		{gasReturnDataCopy, 0, uint64(0x1800000000000003)},
		{gasSha3, 0, uint64(0x300000000000001e)},
		{gasCodeCopy, 0, uint64(0x1800000000000003)},
		{gasExtCodeCopy, 0, uint64(0x1800000000000000)},
		{gasExtCodeHash, 0, uint64(0x0)},
		{gasMLoad, 0, uint64(0x3)},
		{gasMStore8, 0, uint64(0x3)},
		{gasMStore, 0, uint64(0x3)},
		{gasCreate, 0, uint64(0x7d00)},
		{gasCreate2, 0, uint64(0x3000000000007d00)},
		{gasBalance, 0, uint64(0x0)},
		{gasExtCodeSize, 0, uint64(0x0)},
		{gasSLoad, 0, uint64(0x0)},
		{gasExp, 0, uint64(0xa)},
		{gasReturn, 0, uint64(0x0)},
		{gasRevert, 0, uint64(0x0)},
		{gasDelegateCall, 0, uint64(0xffffffffffffffff)},
		{gasStaticCall, 0, uint64(0xffffffffffffffff)},
		{gasPush, 0, uint64(0x3)},
		{gasSwap, 0, uint64(0x3)},
		{gasDup, 0, uint64(0x3)},
	}

	for _, test := range tests {
		result, err := test.function(params.GasTable{}, evm, &Contract{}, stack, &Memory{}, test.memerySize)
		if err != nil {
			t.Errorf("Function %v returned an error: %v", runtime.FuncForPC(reflect.ValueOf(test.function).Pointer()).Name(), err)
		}
		assert.Equal(
			t,
			result,
			test.expected,
			"Mismatch in gas calculation for function %v",
			runtime.FuncForPC(reflect.ValueOf(test.function).Pointer()).Name(),
		)
	}
}

func TestGasCall(t *testing.T) {
	evm, stack := GetFunctionArguments()
	gas, _ := gasCall(params.GasTable{}, evm, &Contract{}, stack, &Memory{}, 0)

	assert.Equal(t, gas, uint64(0x0))
}

func TestGasCallCode(t *testing.T) {
	evm, stack := GetFunctionArguments()
	gas, _ := gasCallCode(params.GasTable{}, evm, &Contract{}, stack, &Memory{}, 0)

	assert.Equal(t, gas, uint64(0x0))
}

func TestGasLog(t *testing.T) {
	evm, stack := GetFunctionArguments()
	gasFunc := makeGasLog(0)

	gas, _ := gasFunc(params.GasTable{}, evm, &Contract{}, stack, &Memory{}, 0)
	assert.Equal(t, gas, uint64(0x0))
}

func TestGasMcopyOverflow(t *testing.T) {
	t.Run("length overflows uint64", func(t *testing.T) {
		stack := newstack()
		// length that doesn't fit in uint64 → Uint64WithOverflow returns true
		hugeLength := new(uint256.Int).Lsh(uint256.NewInt(1), 64)
		stack.push(hugeLength)        // length (stack.Back(2) after dst/src pushed)
		stack.push(uint256.NewInt(0)) // src
		stack.push(uint256.NewInt(0)) // dst

		_, err := gasMcopy(params.GasTable{}, nil, nil, stack, &Memory{}, 0)
		assert.ErrorIs(t, err, ErrGasUintOverflow)
		returnStack(stack)
	})

	t.Run("memorySize overflows gas calculation", func(t *testing.T) {
		stack := newstack()
		stack.push(uint256.NewInt(32)) // length
		stack.push(uint256.NewInt(0))  // src
		stack.push(uint256.NewInt(0))  // dst

		// memorySize > 0xffffffffe0 triggers overflow in memoryGasCost
		_, err := gasMcopy(params.GasTable{}, nil, nil, stack, &Memory{}, 0xffffffffe1)
		assert.ErrorIs(t, err, ErrGasUintOverflow)
		returnStack(stack)
	})
}

func TestGasMcopy(t *testing.T) {
	tests := []struct {
		name       string
		dst        uint64
		src        uint64
		length     uint64
		memorySize uint64
		memLen     int
		expected   uint64
	}{
		{
			name: "zero length copy",
			dst:  0, src: 0, length: 0,
			memorySize: 0,
			memLen:     0,
			// Gverylow(3) + 0 words * CopyGas(3) = 3
			expected: 3,
		},
		{
			name: "32 bytes pre-expanded memory",
			dst:  0, src: 32, length: 32,
			memorySize: 64,
			memLen:     64,
			// Gverylow(3) + 1 word * CopyGas(3) + 0 expansion = 6
			expected: 6,
		},
		{
			name: "32 bytes needs expansion",
			dst:  0, src: 32, length: 32,
			memorySize: 64,
			memLen:     0,
			// expansion: 2 words → linCoef=6, quadCoef=0, fee=6
			// Gverylow(3) + 1 word * CopyGas(3) + 6 expansion = 12
			expected: 12,
		},
		{
			name: "8 bytes overlapping copy pre-expanded",
			dst:  0, src: 1, length: 8,
			memorySize: 32,
			memLen:     32,
			// Gverylow(3) + 1 word * CopyGas(3) + 0 expansion = 6
			expected: 6,
		},
		{
			name: "33 bytes pre-expanded memory",
			dst:  0, src: 32, length: 33,
			memorySize: 96,
			memLen:     96,
			// Gverylow(3) + 2 words * CopyGas(3) + 0 expansion = 9
			expected: 9,
		},
		{
			name: "33 bytes needs expansion",
			dst:  0, src: 32, length: 33,
			memorySize: 96,
			memLen:     0,
			// expansion: 3 words → linCoef=9, quadCoef=0, fee=9
			// Gverylow(3) + 2 words * CopyGas(3) + 9 expansion = 18
			expected: 18,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := newstack()
			stack.push(uint256.NewInt(tt.length))
			stack.push(uint256.NewInt(tt.src))
			stack.push(uint256.NewInt(tt.dst))

			mem := NewMemory()
			if tt.memLen > 0 {
				mem.Resize(uint64(tt.memLen))
				// Pre-charge lastGasCost to simulate prior expansion
				memoryGasCost(mem, uint64(tt.memLen))
			}

			gas, err := gasMcopy(params.GasTable{}, nil, nil, stack, mem, tt.memorySize)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, gas)

			returnStack(stack)
		})
	}
}

// refundStateDB tracks refunds for gas-function tests.
type refundStateDB struct {
	NoopStateDB
	refund uint64
}

func (s *refundStateDB) AddRefund(g uint64) { s.refund += g }
func (s *refundStateDB) GetRefund() uint64  { return s.refund }

func newEVMWithStateDB(statedb StateDB) *EVM {
	return NewEVM(Context{
		BlockNumber:        big.NewInt(1),
		GasPrice:           big.NewInt(1),
		CanTransfer:        NoopCanTransfer,
		Transfer:           NoopTransfer,
		NewContractAddress: newContractAddress,
	}, statedb, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
}

func selfdestructTestContract() *Contract {
	addr := common.HexToAddress("0x0badc0de")
	return NewContract(AccountRef(addr), AccountRef(addr), big.NewInt(0), 0)
}

func selfdestructTestStack() *Stack {
	s := &Stack{}
	s.push(uint256.NewInt(0)) // beneficiary
	return s
}

// gasSuicide (legacy/pre-fork) grants the refund.
func TestGasSuicideGrantsRefund(t *testing.T) {
	sdb := &refundStateDB{}
	evm := newEVMWithStateDB(sdb)
	gas, err := gasSuicide(params.GasTable{}, evm, selfdestructTestContract(), selfdestructTestStack(), &Memory{}, 0)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), gas) // gt.Suicide == 0 for empty GasTable
	assert.Equal(t, params.SuicideRefundGas, sdb.GetRefund())
}

// gasSuicide3529 (Cancun) grants no refund.
func TestGasSuicide3529NoRefund(t *testing.T) {
	sdb := &refundStateDB{}
	evm := newEVMWithStateDB(sdb)
	gas, err := gasSuicide3529(params.GasTable{}, evm, selfdestructTestContract(), selfdestructTestStack(), &Memory{}, 0)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), gas)
	assert.Equal(t, uint64(0), sdb.GetRefund())
}

// repeated SELFDESTRUCT on a surviving contract accumulates zero refund (exploit closed).
func TestGasSuicide3529RepeatedNoRefund(t *testing.T) {
	sdb := &refundStateDB{}
	evm := newEVMWithStateDB(sdb)
	for range 10 {
		_, err := gasSuicide3529(params.GasTable{}, evm, selfdestructTestContract(), selfdestructTestStack(), &Memory{}, 0)
		assert.NoError(t, err)
	}
	assert.Equal(t, uint64(0), sdb.GetRefund())
}

// the Cancun instruction set is wired to the no-refund gas function.
func TestCancunSelfdestructWiredNoRefund(t *testing.T) {
	sdb := &refundStateDB{}
	evm := newEVMWithStateDB(sdb)
	op := NewCancunInstructionSet()[SELFDESTRUCT]
	_, err := op.gasCost(params.GasTable{}, evm, selfdestructTestContract(), selfdestructTestStack(), &Memory{}, 0)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), sdb.GetRefund())
}

// gas cost (excluding refund) is identical between legacy and EIP-3529.
func TestSelfdestructGasCostUnchanged(t *testing.T) {
	gt := params.GasTable{Suicide: 5000, CreateBySuicide: 25000}
	g1, err1 := gasSuicide(gt, newEVMWithStateDB(&refundStateDB{}), selfdestructTestContract(), selfdestructTestStack(), &Memory{}, 0)
	g2, err2 := gasSuicide3529(gt, newEVMWithStateDB(&refundStateDB{}), selfdestructTestContract(), selfdestructTestStack(), &Memory{}, 0)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, g1, g2)
}
