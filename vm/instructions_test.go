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
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime/statedb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

type contractRef struct {
	addr common.Address
}

func (c contractRef) Address() common.Address {
	return c.addr
}

type twoOperandTest struct {
	x        string
	y        string
	expected string
}

func testTwoOperandOp(t *testing.T, tests []twoOperandTest, opFn func(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error)) {
	var (
		env   = NewEVM(Context{}, nil, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		stack = newstack()
		pc    = uint64(0)
	)
	for i, test := range tests {
		x := new(uint256.Int).SetBytes(common.FromHex(test.x))
		shift := new(uint256.Int).SetBytes(common.FromHex(test.y))
		expected := new(uint256.Int).SetBytes(common.FromHex(test.expected))
		stack.push(x)
		stack.push(shift)
		opFn(&pc, env, nil, nil, stack)
		actual := stack.pop()
		if actual.Cmp(expected) != 0 {
			t.Errorf("Testcase %d, expected  %v, got %v", i, expected, actual)
		}
	}
}

func TestByteOp(t *testing.T) {
	var (
		env   = NewEVM(Context{}, nil, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		stack = newstack()
	)
	tests := []struct {
		v        string
		th       uint64
		expected *uint256.Int
	}{
		{"ABCDEF0908070605040302010000000000000000000000000000000000000000", 0, uint256.NewInt(0xAB)},
		{"ABCDEF0908070605040302010000000000000000000000000000000000000000", 1, uint256.NewInt(0xCD)},
		{"00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff", 0, uint256.NewInt(0x00)},
		{"00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff", 1, uint256.NewInt(0xCD)},
		{"0000000000000000000000000000000000000000000000000000000000102030", 31, uint256.NewInt(0x30)},
		{"0000000000000000000000000000000000000000000000000000000000102030", 30, uint256.NewInt(0x20)},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 32, uint256.NewInt(0x0)},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 0xFFFFFFFFFFFFFFFF, uint256.NewInt(0x0)},
	}
	pc := uint64(0)
	for _, test := range tests {
		val := new(uint256.Int).SetBytes(common.FromHex(test.v))
		th := uint256.NewInt(test.th)
		stack.push(val)
		stack.push(th)
		opByte(&pc, env, nil, nil, stack)
		actual := stack.pop()
		if actual.Cmp(test.expected) != 0 {
			t.Fatalf("Expected  [%v] %v:th byte to be %v, was %v.", test.v, test.th, test.expected, actual)
		}
	}
}

func TestSHL(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#shl-shift-left
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000002"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ff", "8000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "0101", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "8000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"},
	}
	testTwoOperandOp(t, tests, opSHL)
}

func TestSHR(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#shr-logical-shift-right
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "01", "4000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "ff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0101", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opSHR)
}

func TestSAR(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#sar-arithmetic-shift-right
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "01", "c000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "ff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0100", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0101", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"4000000000000000000000000000000000000000000000000000000000000000", "fe", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "f8", "000000000000000000000000000000000000000000000000000000000000007f"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "fe", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
	}

	testTwoOperandOp(t, tests, opSAR)
}

func TestSGT(t *testing.T) {
	tests := []twoOperandTest{
		{
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"0000000000000000000000000000000000000000000000000000000000000001",
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"0000000000000000000000000000000000000000000000000000000000000001",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"8000000000000000000000000000000000000000000000000000000000000001",
			"8000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"8000000000000000000000000000000000000000000000000000000000000001",
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"8000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb",
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd",
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
	testTwoOperandOp(t, tests, opSgt)
}

func TestSLT(t *testing.T) {
	tests := []twoOperandTest{
		{
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"0000000000000000000000000000000000000000000000000000000000000001",
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"0000000000000000000000000000000000000000000000000000000000000001",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"8000000000000000000000000000000000000000000000000000000000000001",
			"8000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"8000000000000000000000000000000000000000000000000000000000000001",
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"8000000000000000000000000000000000000000000000000000000000000001",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb",
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd",
			"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},
	}
	testTwoOperandOp(t, tests, opSlt)
}

func opBenchmark(bench *testing.B, op func(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error), args ...string) {
	var (
		env   = NewEVM(Context{}, nil, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		stack = newstack()
	)
	// convert args
	byteArgs := make([][]byte, len(args))
	for i, arg := range args {
		byteArgs[i] = common.Hex2Bytes(arg)
	}
	pc := uint64(0)

	for bench.Loop() {
		for _, arg := range byteArgs {
			a := new(uint256.Int).SetBytes(arg)
			stack.push(a)
		}
		op(&pc, env, nil, nil, stack)
		stack.pop()
	}
}

func BenchmarkOpAdd64(b *testing.B) {
	x := "ffffffff"
	y := "fd37f3e2bba2c4f"

	opBenchmark(b, opAdd, x, y)
}

func BenchmarkOpAdd128(b *testing.B) {
	x := "ffffffffffffffff"
	y := "f5470b43c6549b016288e9a65629687"

	opBenchmark(b, opAdd, x, y)
}

func BenchmarkOpAdd256(b *testing.B) {
	x := "0802431afcbce1fc194c9eaa417b2fb67dc75a95db0bc7ec6b1c8af11df6a1da9"
	y := "a1f5aac137876480252e5dcac62c354ec0d42b76b0642b6181ed099849ea1d57"

	opBenchmark(b, opAdd, x, y)
}

func BenchmarkOpSub64(b *testing.B) {
	x := "51022b6317003a9d"
	y := "a20456c62e00753a"

	opBenchmark(b, opSub, x, y)
}

func BenchmarkOpSub128(b *testing.B) {
	x := "4dde30faaacdc14d00327aac314e915d"
	y := "9bbc61f5559b829a0064f558629d22ba"

	opBenchmark(b, opSub, x, y)
}

func BenchmarkOpSub256(b *testing.B) {
	x := "4bfcd8bb2ac462735b48a17580690283980aa2d679f091c64364594df113ea37"
	y := "97f9b1765588c4e6b69142eb00d20507301545acf3e1238c86c8b29be227d46e"

	opBenchmark(b, opSub, x, y)
}

func BenchmarkOpMul(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opMul, x, y)
}

func BenchmarkOpDiv256(b *testing.B) {
	x := "ff3f9014f20db29ae04af2c2d265de17"
	y := "fe7fb0d1f59dfe9492ffbf73683fd1e870eec79504c60144cc7f5fc2bad1e611"
	opBenchmark(b, opDiv, x, y)
}

func BenchmarkOpDiv128(b *testing.B) {
	x := "fdedc7f10142ff97"
	y := "fbdfda0e2ce356173d1993d5f70a2b11"
	opBenchmark(b, opDiv, x, y)
}

func BenchmarkOpDiv64(b *testing.B) {
	x := "fcb34eb3"
	y := "f97180878e839129"
	opBenchmark(b, opDiv, x, y)
}

func BenchmarkOpSdiv(b *testing.B) {
	x := "ff3f9014f20db29ae04af2c2d265de17"
	y := "fe7fb0d1f59dfe9492ffbf73683fd1e870eec79504c60144cc7f5fc2bad1e611"

	opBenchmark(b, opSdiv, x, y)
}

func BenchmarkOpMod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opMod, x, y)
}

func BenchmarkOpSmod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSmod, x, y)
}

func BenchmarkOpExp(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opExp, x, y)
}

func BenchmarkOpSignExtend(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSignExtend, x, y)
}

func BenchmarkOpLt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opLt, x, y)
}

func BenchmarkOpGt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opGt, x, y)
}

func BenchmarkOpSlt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSlt, x, y)
}

func BenchmarkOpSgt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSgt, x, y)
}

func BenchmarkOpEq(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opEq, x, y)
}

func BenchmarkOpEq2(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201fffffffe"
	opBenchmark(b, opEq, x, y)
}

func BenchmarkOpAnd(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opAnd, x, y)
}

func BenchmarkOpOr(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opOr, x, y)
}

func BenchmarkOpXor(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opXor, x, y)
}

func BenchmarkOpByte(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opByte, x, y)
}

func BenchmarkOpAddmod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	z := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opAddmod, x, y, z)
}

func BenchmarkOpMulmod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	z := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opMulmod, x, y, z)
}

func BenchmarkOpSHL(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "ff"

	opBenchmark(b, opSHL, x, y)
}

func BenchmarkOpSHR(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "ff"

	opBenchmark(b, opSHR, x, y)
}

func BenchmarkOpSAR(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "ff"

	opBenchmark(b, opSAR, x, y)
}

func BenchmarkOpIsZero(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	opBenchmark(b, opIszero, x)
}

func TestOpMstore(t *testing.T) {
	var (
		env   = NewEVM(Context{}, nil, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		stack = newstack()
		mem   = NewMemory()
	)

	mem.Resize(64)
	pc := uint64(0)
	v := "abcdef00000000000000abba000000000deaf000000c0de00100000000133700"
	stack.push(new(uint256.Int).SetBytes(common.Hex2Bytes(v)))
	stack.push(new(uint256.Int))
	opMstore(&pc, env, nil, mem, stack)
	if got := common.Bytes2Hex(mem.GetCopy(0, 32)); got != v {
		t.Fatalf("Mstore fail, got %v, expected %v", got, v)
	}
	stack.push(new(uint256.Int).SetUint64(0x1))
	stack.push(new(uint256.Int))
	opMstore(&pc, env, nil, mem, stack)
	if common.Bytes2Hex(mem.GetCopy(0, 32)) != "0000000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("Mstore failed to overwrite previous value")
	}
}

func BenchmarkOpMstore(bench *testing.B) {
	var (
		env   = NewEVM(Context{}, nil, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		stack = newstack()
		mem   = NewMemory()
	)

	mem.Resize(64)
	pc := uint64(0)
	memStart := new(uint256.Int)
	value := new(uint256.Int).SetUint64(0x1337)

	for bench.Loop() {
		stack.push(value)
		stack.push(memStart)
		opMstore(&pc, env, nil, mem, stack)
	}
}

func TestOpTstore(t *testing.T) {
	var (
		db          = muxdb.NewMem()
		state       = state.New(db, trie.Root{Hash: thor.Bytes32{}})
		stateDB     = statedb.New(state)
		env         = NewEVM(Context{}, stateDB, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		stack       = newstack()
		mem         = NewMemory()
		caller      = common.Address{0}
		to          = common.Address{1}
		contractRef = contractRef{caller}
		contract    = NewContract(contractRef, AccountRef(to), big.NewInt(0), 0)
		value       = common.Hex2Bytes("abcdef00000000000000abba000000000deaf000000c0de00100000000133700")
	)

	// Add a stateObject for the caller and the contract being called
	stateDB.CreateAccount(caller)
	stateDB.CreateAccount(to)

	pc := uint64(0)
	// push the value to the stack
	stack.push(new(uint256.Int).SetBytes(value))
	// push the location to the stack
	stack.push(new(uint256.Int))
	opTstore(&pc, env, contract, mem, stack)
	// there should be no elements on the stack after TSTORE
	if stack.len() != 0 {
		t.Fatal("stack wrong size")
	}
	// push the location to the stack
	stack.push(new(uint256.Int))
	opTload(&pc, env, contract, mem, stack) // there should be one element on the stack after TLOAD
	if stack.len() != 1 {
		t.Fatal("stack wrong size")
	}
	val := stack.peek()
	if !bytes.Equal(val.Bytes(), value) {
		t.Fatal("incorrect element read from transient storage")
	}
}

func TestCreate2Addreses(t *testing.T) {
	type testcase struct {
		origin   string
		salt     string
		code     string
		expected string
	}

	for i, tt := range []testcase{
		{
			origin:   "0x0000000000000000000000000000000000000000",
			salt:     "0x0000000000000000000000000000000000000000",
			code:     "0x00",
			expected: "0x4d1a2e2bb4f88f0250f26ffff098b0b30b26bf38",
		},
		{
			origin:   "0xdeadbeef00000000000000000000000000000000",
			salt:     "0x0000000000000000000000000000000000000000",
			code:     "0x00",
			expected: "0xB928f69Bb1D91Cd65274e3c79d8986362984fDA3",
		},
		{
			origin:   "0xdeadbeef00000000000000000000000000000000",
			salt:     "0xfeed000000000000000000000000000000000000",
			code:     "0x00",
			expected: "0xD04116cDd17beBE565EB2422F2497E06cC1C9833",
		},
		{
			origin:   "0x0000000000000000000000000000000000000000",
			salt:     "0x0000000000000000000000000000000000000000",
			code:     "0xdeadbeef",
			expected: "0x70f2b2914A2a4b783FaEFb75f459A580616Fcb5e",
		},
		{
			origin:   "0x00000000000000000000000000000000deadbeef",
			salt:     "0xcafebabe",
			code:     "0xdeadbeef",
			expected: "0x60f3f640a8508fC6a86d45DF051962668E1e8AC7",
		},
		{
			origin:   "0x00000000000000000000000000000000deadbeef",
			salt:     "0xcafebabe",
			code:     "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			expected: "0x1d8bfDC5D46DC4f61D6b6115972536eBE6A8854C",
		},
		{
			origin:   "0x0000000000000000000000000000000000000000",
			salt:     "0x0000000000000000000000000000000000000000",
			code:     "0x",
			expected: "0xE33C0C7F7df4809055C3ebA6c09CFe4BaF1BD9e0",
		},
	} {
		origin := common.BytesToAddress(common.FromHex(tt.origin))
		salt := common.BytesToHash(common.FromHex(tt.salt))
		code := common.FromHex(tt.code)
		codeHash := thor.Keccak256(code).Bytes()
		// THOR: Cannot use crypto.CreateAddress2 function.
		// v1.8.14 -> v1.8.27 dependency issue. See patch.go file.
		address := CreateAddress2(origin, salt, codeHash)
		/*
			stack          := newstack()
			// salt, but we don't need that for this test
			stack.push(big.NewInt(int64(len(code)))) //size
			stack.push(big.NewInt(0)) // memstart
			stack.push(big.NewInt(0)) // value
			gas, _ := gasCreate2(params.GasTable{}, nil, nil, stack, nil, 0)
			fmt.Printf("Example %d\n* address `0x%x`\n* salt `0x%x`\n* init_code `0x%x`\n* gas (assuming no mem expansion): `%v`\n* result: `%s`\n\n", i,origin, salt, code, gas, address.String())
		*/
		expected := common.BytesToAddress(common.FromHex(tt.expected))
		if !bytes.Equal(expected.Bytes(), address.Bytes()) {
			t.Errorf("test %d: expected %s, got %s", i, expected.String(), address.String())
		}
	}
}

func TestOpSuicide6780(t *testing.T) {
	masterAddress := common.HexToAddress("0x01")
	contractAddr := common.HexToAddress("0x02")
	tokenReceiver := common.HexToAddress("0x03")
	energyABI, _ := abi.New(
		[]byte(
			`[{"anonymous":false,"inputs":[{"indexed":true,"name":"_from","type":"address"},{"indexed":true,"name":"_to","type":"address"},{"indexed":false,"name":"_value","type":"uint256"}],"name":"Transfer","type":"event"}]`,
		),
	)
	energyTransferEvent, _ := energyABI.EventByName("Transfer")

	type testcase struct {
		name     string
		initFunc func() (evm *EVM, state *state.State, stack *Stack)
		testFunc func(evm *EVM, state *state.State, t *testing.T)
	}

	tests := []testcase{}

	newEVMInstance := func(state *state.State) *EVM {
		stateDB := statedb.New(state)
		evm := NewEVM(Context{
			BlockNumber: big.NewInt(1),
			GasPrice:    big.NewInt(1),

			// NOTE: THIS IS CLOUSER FUNCTION.
			// IF YOU WANT TO CHANGE THIS TEST CASE, PLEASE MAKE SURE THE LOGIC IS CORRECT.
			// AND CHANGE THE FUNCTION IN runtime/runtime.go ACCORDINGLY.
			OnSuicideContract: func(evm *EVM, contract common.Address, receiver common.Address) {
				// it's IMPORTANT to process energy before token
				energy, err := state.GetEnergy(thor.Address(contract), 1, 1)
				if err != nil {
					panic(err)
				}
				bal := stateDB.GetBalance(contract)

				if bal.Sign() != 0 || energy.Sign() != 0 {
					receiverEnergy, err := state.GetEnergy(thor.Address(receiver), 1, 1)
					if err != nil {
						panic(err)
					}

					// touch the receiver's energy
					// after EIP6780, MUST to clear contract's energy, vm delete contarct operation is optional.
					// if token receiver is same as contract itself, skip no-op transfer when self-destructing to self.
					if contract.String() != receiver.String() {
						if err := state.SetEnergy(
							thor.Address(receiver),
							new(big.Int).Add(receiverEnergy, energy),
							1); err != nil {
							panic(err)
						}

						if err = state.SetEnergy(
							thor.Address(contract),
							big.NewInt(0),
							1); err != nil {
							panic(err)
						}
					}

					// emit event if there is energy in the account
					if energy.Sign() != 0 {
						// see ERC20's Transfer event
						topics := []common.Hash{
							common.Hash(energyTransferEvent.ID()),
							common.BytesToHash(contract[:]),
							common.BytesToHash(receiver[:]),
						}

						data, err := energyTransferEvent.Encode(energy)
						if err != nil {
							panic(err)
						}

						stateDB.AddLog(&types.Log{
							Address: common.Address(thor.BytesToAddress([]byte("0x0000000000000000000000000000456E65726779"))),
							Topics:  topics,
							Data:    data,
						})
					}
				}

				if bal.Sign() != 0 {
					// after EIP6780, MUST to clear contract's VET, vm delete contarct operation is optional.
					// if token receiver is same as contract itself, skip no-op transfer when self-destructing to self.
					if contract.String() != receiver.String() {
						stateDB.AddBalance(receiver, bal)
						stateDB.SubBalance(contract, bal)
					}

					stateDB.AddTransfer(&tx.Transfer{
						Sender:    thor.Address(contract),
						Recipient: thor.Address(receiver),
						Amount:    bal,
					})
				}
			},
		}, stateDB, &ChainConfig{ChainConfig: *params.TestChainConfig}, Config{})
		return evm
	}

	case1 := testcase{
		name: "Different Clause,different receiver",
		initFunc: func() (*EVM, *state.State, *Stack) {
			var (
				db    = muxdb.NewMem()
				state = state.New(db, trie.Root{})
				stack = newstack()
			)

			evm := newEVMInstance(state)

			stack.push(new(uint256.Int).SetBytes(tokenReceiver.Bytes()))

			// simulate the contract create in the other clause
			state.SetStorage(thor.Address(contractAddr), thor.BytesToBytes32([]byte("key1")), thor.BytesToBytes32([]byte("value1")))
			state.SetBalance(thor.Address(contractAddr), big.NewInt(100))
			state.SetEnergy(thor.Address(contractAddr), big.NewInt(100), 1)
			state.SetMaster(thor.Address(contractAddr), thor.Address(masterAddress))
			state.SetCode(thor.Address(contractAddr), []byte("code"))

			return evm, state, stack
		},
		testFunc: func(evm *EVM, state *state.State, t *testing.T) {
			if evm.StateDB.GetBalance(contractAddr).Sign() != 0 || evm.StateDB.GetBalance(tokenReceiver).Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("expected contract balance to be transfer all to receiver, got %v", evm.StateDB.GetBalance(contractAddr))
			}

			contractEnergy, err := state.GetEnergy(thor.Address(contractAddr), 1, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if contractEnergy.Sign() != 0 {
				t.Fatalf("expected contract energy to be transfer all to receiver, got %v", contractEnergy)
			}

			receiverEnergy, err := state.GetEnergy(thor.Address(tokenReceiver), 1, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if receiverEnergy.Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("expected receiver energy to be transfer all from contract, got %v", receiverEnergy)
			}

			if evm.StateDB.Empty(contractAddr) {
				t.Fatalf("expected contract still in stateDB, but it still have been deleted")
			}
		},
	}

	case2 := testcase{
		name: "Same Clause, different receiver",
		initFunc: func() (*EVM, *state.State, *Stack) {
			var (
				db    = muxdb.NewMem()
				state = state.New(db, trie.Root{})
				stack = newstack()
			)

			evm := newEVMInstance(state)

			stack.push(new(uint256.Int).SetBytes(tokenReceiver.Bytes()))

			// simulate the contract create in the same clause
			evm.StateDB.CreateContract(contractAddr)

			state.SetStorage(thor.Address(contractAddr), thor.BytesToBytes32([]byte("key1")), thor.BytesToBytes32([]byte("value1")))
			state.SetBalance(thor.Address(contractAddr), big.NewInt(100))
			state.SetEnergy(thor.Address(contractAddr), big.NewInt(100), 1)
			state.SetMaster(thor.Address(contractAddr), thor.Address(masterAddress))
			state.SetCode(thor.Address(contractAddr), []byte("code"))

			return evm, state, stack
		},
		testFunc: func(evm *EVM, state *state.State, t *testing.T) {
			if evm.StateDB.GetBalance(contractAddr).Sign() != 0 || evm.StateDB.GetBalance(tokenReceiver).Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("expected contract balance to be transfer all to receiver, got %v", evm.StateDB.GetBalance(contractAddr))
			}

			contractEnergy, err := state.GetEnergy(thor.Address(contractAddr), 1, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if contractEnergy.Sign() != 0 {
				t.Fatalf("expected contract energy to be transfer all to receiver, got %v", contractEnergy)
			}

			receiverEnergy, err := state.GetEnergy(thor.Address(tokenReceiver), 1, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if receiverEnergy.Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("expected receiver energy to be transfer all from contract, got %v", receiverEnergy)
			}

			if !evm.StateDB.Empty(contractAddr) {
				t.Fatalf("expected contract will be deleted, but it is not")
			}
		},
	}

	case3 := testcase{
		name: "Different Clause,same receiver",
		initFunc: func() (*EVM, *state.State, *Stack) {
			var (
				db    = muxdb.NewMem()
				state = state.New(db, trie.Root{})
				stack = newstack()
			)

			evm := newEVMInstance(state)

			stack.push(new(uint256.Int).SetBytes(contractAddr.Bytes()))

			// simulate the contract create in the other clause
			state.SetStorage(thor.Address(contractAddr), thor.BytesToBytes32([]byte("key1")), thor.BytesToBytes32([]byte("value1")))
			state.SetBalance(thor.Address(contractAddr), big.NewInt(100))
			state.SetEnergy(thor.Address(contractAddr), big.NewInt(100), 1)
			state.SetMaster(thor.Address(contractAddr), thor.Address(masterAddress))
			state.SetCode(thor.Address(contractAddr), []byte("code"))

			return evm, state, stack
		},
		testFunc: func(evm *EVM, state *state.State, t *testing.T) {
			if evm.StateDB.GetBalance(contractAddr).Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("expected contract balance to be transfer all to receiver, got %v", evm.StateDB.GetBalance(contractAddr))
			}

			contractEnergy, err := state.GetEnergy(thor.Address(contractAddr), 1, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if contractEnergy.Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("expected contract energy to be transfer all to receiver, got %v", contractEnergy)
			}

			if evm.StateDB.Empty(contractAddr) {
				t.Fatalf("expected contract will be deleted, but it is not")
			}
		},
	}

	case4 := testcase{
		name: "Same Clause,same receiver",
		initFunc: func() (*EVM, *state.State, *Stack) {
			var (
				db    = muxdb.NewMem()
				state = state.New(db, trie.Root{})
				stack = newstack()
			)

			evm := newEVMInstance(state)

			stack.push(new(uint256.Int).SetBytes(contractAddr.Bytes()))

			// simulate the contract create in the same clause
			evm.StateDB.CreateContract(contractAddr)

			state.SetStorage(thor.Address(contractAddr), thor.BytesToBytes32([]byte("key1")), thor.BytesToBytes32([]byte("value1")))
			state.SetBalance(thor.Address(contractAddr), big.NewInt(100))
			state.SetEnergy(thor.Address(contractAddr), big.NewInt(100), 1)
			state.SetMaster(thor.Address(contractAddr), thor.Address(masterAddress))
			state.SetCode(thor.Address(contractAddr), []byte("code"))

			return evm, state, stack
		},
		testFunc: func(evm *EVM, state *state.State, t *testing.T) {
			if evm.StateDB.GetBalance(contractAddr).Sign() != 0 {
				t.Fatalf("expected contract balance to be burnt, got %v", evm.StateDB.GetBalance(contractAddr))
			}

			contractEnergy, err := state.GetEnergy(thor.Address(contractAddr), 1, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if contractEnergy.Sign() != 0 {
				t.Fatalf("expected contract energy to be burnt, got %v", contractEnergy)
			}

			if !evm.StateDB.Empty(contractAddr) {
				t.Fatalf("expected contract will be deleted, but it is not")
			}
		},
	}

	tests = append(tests, case1, case2, case3, case4)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evm, state, stack := tc.initFunc()
			_, err := opSuicide6780(nil, evm, NewContract(AccountRef(masterAddress), AccountRef(contractAddr), big.NewInt(0), 0), nil, stack)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.testFunc(evm, state, t)
		})
	}
}
