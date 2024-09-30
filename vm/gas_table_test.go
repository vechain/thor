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
		assert.Equal(t, result, test.expected, "Mismatch in gas calculation for function %v", runtime.FuncForPC(reflect.ValueOf(test.function).Pointer()).Name())
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
