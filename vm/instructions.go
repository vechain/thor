// Copyright 2015 The go-ethereum Authors
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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/vechain/thor/v2/thor"
)

func opAdd(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Add(x, y)
	return nil, nil
}

func opSub(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Sub(x, y)
	return nil, nil
}

func opMul(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Mul(x, y)
	return nil, nil
}

func opDiv(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Div(x, y)
	return nil, nil
}

func opSdiv(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.SDiv(x, y)
	return nil, nil
}

func opMod(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Mod(x, y)
	return nil, nil
}

func opSmod(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.SMod(x, y)
	return nil, nil
}

func opExp(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	base, exponent := stack.popptr(), stack.peek()
	exponent.Exp(base, exponent)
	return nil, nil
}

func opSignExtend(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	back, num := stack.popptr(), stack.peek()
	num.ExtendSign(num, back)
	return nil, nil
}

func opNot(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x := stack.peek()
	x.Not(x)
	return nil, nil
}

func opLt(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	if x.Lt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opGt(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	if x.Gt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opSlt(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	if x.Slt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opSgt(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	if x.Sgt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opEq(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	if x.Eq(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opIszero(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x := stack.peek()
	if x.IsZero() {
		x.SetOne()
	} else {
		x.Clear()
	}
	return nil, nil
}

func opAnd(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.And(x, y)
	return nil, nil
}

func opOr(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Or(x, y)
	return nil, nil
}

func opXor(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y := stack.popptr(), stack.peek()
	y.Xor(x, y)
	return nil, nil
}

func opByte(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	th, val := stack.popptr(), stack.peek()
	val.Byte(th)
	return nil, nil
}

func opAddmod(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y, z := stack.popptr(), stack.popptr(), stack.peek()
	if z.IsZero() {
		z.Clear()
	} else {
		z.AddMod(x, y, z)
	}
	return nil, nil
}

func opMulmod(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x, y, z := stack.popptr(), stack.popptr(), stack.peek()
	z.MulMod(x, y, z)
	return nil, nil
}

// opSHL implements Shift Left
// The SHL instruction (shift left) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the left by arg1 number of bits.
func opSHL(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	// Note, second operand is left in the stack; accumulate result into it, and no need to push it afterwards
	shift, value := stack.popptr(), stack.peek()
	if shift.LtUint64(256) {
		value.Lsh(value, uint(shift.Uint64()))
	} else {
		value.Clear()
	}
	return nil, nil
}

// opSHR implements Logical Shift Right
// The SHR instruction (logical shift right) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the right by arg1 number of bits with zero fill.
func opSHR(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	// Note, second operand is left in the stack; accumulate result into it, and no need to push it afterwards
	shift, value := stack.popptr(), stack.peek()
	if shift.LtUint64(256) {
		value.Rsh(value, uint(shift.Uint64()))
	} else {
		value.Clear()
	}
	return nil, nil
}

// opSAR implements Arithmetic Shift Right
// The SAR instruction (arithmetic shift right) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the right by arg1 number of bits with sign extension.
func opSAR(_ *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	// Note, S256 returns (potentially) a new bigint, so we're popping, not peeking this one
	shift, value := stack.popptr(), stack.peek()
	if shift.GtUint64(256) {
		if value.Sign() >= 0 {
			value.Clear()
		} else {
			// Max negative shift: all bits set
			value.SetAllOne()
		}
		return nil, nil
	}
	n := uint(shift.Uint64())
	value.SRsh(value, n)
	return nil, nil
}

func opSha3(_ *uint64, evm *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	offset, size := stack.popptr(), stack.peek()
	data := memory.GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	hash := thor.Keccak256(data)

	if evm.vmConfig.EnablePreimageRecording {
		evm.StateDB.AddPreimage(common.Hash(hash), common.CopyBytes(data))
	}
	size.SetBytes(hash[:])
	return nil, nil
}

func opAddress(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(new(uint256.Int).SetBytes(contract.Address().Bytes()))
	return nil, nil
}

func opBalance(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	slot := stack.peek()
	address := common.Address(slot.Bytes20())
	slot.SetFromBig(evm.StateDB.GetBalance(address))
	return nil, nil
}

func opOrigin(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(new(uint256.Int).SetBytes(evm.Origin.Bytes()))
	return nil, nil
}

func opCaller(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(new(uint256.Int).SetBytes(contract.Caller().Bytes()))
	return nil, nil
}

func opCallValue(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	v, _ := uint256.FromBig(contract.value)
	stack.push(v)
	return nil, nil
}

func opCallDataLoad(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	x := stack.peek()
	if offset, overflow := x.Uint64WithOverflow(); !overflow {
		data := getData(contract.Input, offset, 32)
		x.SetBytes(data)
	} else {
		x.Clear()
	}
	return nil, nil
}

func opCallDataSize(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(uint256.NewInt(uint64(len(contract.Input))))
	return nil, nil
}

func opCallDataCopy(_ *uint64, _ *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	var (
		memOffset  = stack.popptr()
		dataOffset = stack.popptr()
		length     = stack.popptr()
	)
	dataOffset64, overflow := dataOffset.Uint64WithOverflow()
	if overflow {
		dataOffset64 = 0xffffffffffffffff
	}
	// These values are checked for overflow during gas cost calculation
	memOffset64 := memOffset.Uint64()
	length64 := length.Uint64()
	memory.Set(memOffset64, length64, getData(contract.Input, dataOffset64, length64))
	return nil, nil
}

func opReturnDataSize(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(uint256.NewInt(uint64(len(evm.interpreter.returnData))))
	return nil, nil
}

func opReturnDataCopy(_ *uint64, evm *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	var (
		memOffset  = stack.popptr()
		dataOffset = stack.popptr()
		length     = stack.popptr()
	)
	offset64, overflow := dataOffset.Uint64WithOverflow()
	if overflow {
		return nil, ErrReturnDataOutOfBounds
	}
	// we can reuse dataOffset now (aliasing it for clarity)
	var end = dataOffset
	end.Add(dataOffset, length)
	end64, overflow := end.Uint64WithOverflow()
	if overflow || uint64(len(evm.interpreter.returnData)) < end64 {
		return nil, ErrReturnDataOutOfBounds
	}
	memory.Set(memOffset.Uint64(), length.Uint64(), evm.interpreter.returnData[offset64:end64])
	return nil, nil
}

func opExtCodeSize(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	slot := stack.peek()
	slot.SetUint64(uint64(evm.StateDB.GetCodeSize(slot.Bytes20())))
	return nil, nil
}

func opCodeSize(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	l := uint256.NewInt(uint64(len(contract.Code)))
	stack.push(l)
	return nil, nil
}

func opCodeCopy(_ *uint64, _ *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	var (
		memOffset  = stack.popptr()
		codeOffset = stack.popptr()
		length     = stack.popptr()
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = 0xffffffffffffffff
	}
	codeCopy := getData(contract.Code, uint64CodeOffset, length.Uint64())
	memory.Set(memOffset.Uint64(), length.Uint64(), codeCopy)
	return nil, nil
}

func opExtCodeCopy(_ *uint64, evm *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	var (
		a          = stack.popptr()
		memOffset  = stack.popptr()
		codeOffset = stack.popptr()
		length     = stack.popptr()
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = 0xffffffffffffffff
	}
	addr := common.Address(a.Bytes20())
	codeCopy := getData(evm.StateDB.GetCode(addr), uint64CodeOffset, length.Uint64())
	memory.Set(memOffset.Uint64(), length.Uint64(), codeCopy)
	return nil, nil
}

// opExtCodeHash returns the code hash of a specified account.
// There are several cases when the function is called, while we can relay everything
// to `state.GetCodeHash` function to ensure the correctness.
//
//	(1) Caller tries to get the code hash of a normal contract account, state
//
// should return the relative code hash and set it as the result.
//
//	(2) Caller tries to get the code hash of a non-existent account, state should
//
// return common.Hash{} and zero will be set as the result.
//
//	(3) Caller tries to get the code hash for an account without contract code,
//
// state should return emptyCodeHash(0xc5d246...) as the result.
//
//	(4) Caller tries to get the code hash of a precompiled account, the result
//
// should be zero or emptyCodeHash.
//
// It is worth noting that in order to avoid unnecessary create and clean,
// all precompile accounts on mainnet have been transferred 1 wei, so the return
// here should be emptyCodeHash.
// If the precompile account is not transferred any amount on a private or
// customized chain, the return value will be zero.
//
//	(5) Caller tries to get the code hash for an account which is marked as suicided
//
// in the current transaction, the code hash of this account should be returned.
//
//	(6) Caller tries to get the code hash for an account which is marked as deleted,
//
// this account should be regarded as a non-existent account and zero should be returned.
func opExtCodeHash(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	slot := stack.peek()
	address := common.Address(slot.Bytes20())
	if evm.StateDB.Empty(address) {
		slot.Clear()
	} else if codeHash := evm.StateDB.GetCodeHash(address); codeHash == (common.Hash{}) { // differ from eth
		slot.SetBytes(emptyCodeHash.Bytes())
	} else {
		slot.SetBytes(codeHash.Bytes())
	}
	return nil, nil
}

func opGasprice(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	v, _ := uint256.FromBig(evm.GasPrice)
	stack.push(v)
	return nil, nil
}

func opBlockhash(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	num := stack.peek()
	num64, overflow := num.Uint64WithOverflow()
	if overflow {
		num.Clear()
		return nil, nil
	}
	var upper, lower uint64
	upper = evm.Context.BlockNumber.Uint64()
	if upper < 257 {
		lower = 0
	} else {
		lower = upper - 256
	}
	if num64 >= lower && num64 < upper {
		num.SetBytes(evm.Context.GetHash(num64).Bytes())
	} else {
		num.Clear()
	}
	return nil, nil
}

func opCoinbase(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(new(uint256.Int).SetBytes(evm.Context.Coinbase.Bytes()))
	return nil, nil
}

func opTimestamp(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	v, _ := uint256.FromBig(evm.Context.Time)
	stack.push(v)
	return nil, nil
}

func opNumber(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	v, _ := uint256.FromBig(evm.Context.BlockNumber)
	stack.push(v)
	return nil, nil
}

func opDifficulty(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	v, _ := uint256.FromBig(evm.Context.Difficulty)
	stack.push(v)
	return nil, nil
}

func opGasLimit(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	stack.push(uint256.NewInt(evm.Context.GasLimit))
	return nil, nil
}

func opPop(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.pop()
	return nil, nil
}

func opMload(_ *uint64, _ *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	v := stack.peek()
	offset := int64(v.Uint64())
	v.SetBytes(memory.GetPtr(offset, 32))
	return nil, nil
}

func opMstore(_ *uint64, _ *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	// pop value of the stack
	mStart, val := stack.popptr(), stack.popptr()
	memory.Set32(mStart.Uint64(), val)
	return nil, nil
}

func opMstore8(_ *uint64, _ *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	off, val := stack.popptr(), stack.popptr()
	memory.store[off.Uint64()] = byte(val.Uint64())
	return nil, nil
}

func opSload(_ *uint64, evm *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	loc := stack.peek()
	hash := common.Hash(loc.Bytes32())
	val := evm.StateDB.GetState(contract.Address(), hash)
	loc.SetBytes(val.Bytes())
	return nil, nil
}

func opSstore(_ *uint64, evm *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	loc := stack.popptr()
	val := stack.popptr()
	evm.StateDB.SetState(contract.Address(),
		loc.Bytes32(), val.Bytes32())
	return nil, nil
}

func opJump(pc *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	pos := stack.popptr()
	if !contract.validJumpdest(pos) {
		return nil, ErrInvalidJump
	}
	*pc = pos.Uint64()
	return nil, nil
}

func opJumpi(pc *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	pos, cond := stack.popptr(), stack.popptr()
	if !cond.IsZero() {
		if !contract.validJumpdest(pos) {
			return nil, ErrInvalidJump
		}
		*pc = pos.Uint64()
	} else {
		*pc++
	}
	return nil, nil
}

func opJumpdest(_ *uint64, _ *EVM, _ *Contract, _ *Memory, _ *Stack) ([]byte, error) {
	return nil, nil
}

func opPc(pc *uint64, _ *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(uint256.NewInt(*pc))
	return nil, nil
}

func opMsize(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	stack.push(uint256.NewInt(uint64(memory.Len())))
	return nil, nil
}

func opGas(_ *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	stack.push(uint256.NewInt(contract.Gas))
	return nil, nil
}

func opCreate(_ *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	var (
		value        = stack.popptr()
		offset, size = stack.popptr().Uint64(), stack.popptr().Uint64()
		input        = memory.GetCopy(int64(offset), int64(size))
		gas          = contract.Gas
	)
	if evm.ChainConfig().IsEIP150(evm.BlockNumber) {
		gas -= gas / 64
	}

	contract.UseGas(gas)

	//TODO: use uint256.Int instead of converting with toBig()
	var bigVal = big0
	if !value.IsZero() {
		bigVal = value.ToBig()
	}

	res, addr, returnGas, suberr := evm.Create(contract, input, gas, bigVal)
	// Push item on the stack based on the returned error. If the ruleset is
	// homestead we must check for CodeStoreOutOfGasError (homestead only
	// rule) and treat as an error, if the ruleset is frontier we must
	// ignore this error and pretend the operation was successful.
	if evm.ChainConfig().IsHomestead(evm.BlockNumber) && suberr == ErrCodeStoreOutOfGas {
		stack.push(&uint256.Int{})
	} else if suberr != nil && suberr != ErrCodeStoreOutOfGas {
		stack.push(&uint256.Int{})
	} else {
		stack.push(new(uint256.Int).SetBytes(addr.Bytes()))
	}
	contract.Gas += returnGas

	if suberr == ErrExecutionReverted {
		return res, nil
	}
	return nil, nil
}

func opCreate2(_ *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	var (
		endowment    = stack.popptr()
		offset, size = stack.popptr().Uint64(), stack.popptr().Uint64()
		salt         = stack.pop()
		input        = memory.GetCopy(int64(offset), int64(size))
		gas          = contract.Gas
	)

	// Apply EIP150
	gas -= gas / 64
	contract.UseGas(gas)

	//TODO: use uint256.Int instead of converting with toBig()
	bigEndowment := big0
	if !endowment.IsZero() {
		bigEndowment = endowment.ToBig()
	}
	res, addr, returnGas, suberr := evm.Create2(contract, input, gas, bigEndowment, &salt)
	// Push item on the stack based on the returned error.
	if suberr != nil {
		stack.push(&uint256.Int{})
	} else {
		stack.push(new(uint256.Int).SetBytes(addr.Bytes()))
	}
	contract.Gas += returnGas

	if suberr == ErrExecutionReverted {
		return res, nil
	}
	return nil, nil
}

func opCall(_ *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	// Pop gas. The actual gas in interpreter.evm.callGasTemp.
	stack.pop()
	gas := evm.callGasTemp
	// Pop other call parameters.
	addr, value := stack.popptr().Bytes20(), stack.pop()
	inOffset, inSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	retOffset, retSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	toAddr := common.Address(addr)
	// Get the arguments from the memory.
	args := memory.GetCopy(int64(inOffset), int64(inSize))

	var bigVal = big0
	//TODO: use uint256.Int instead of converting with toBig()
	// By using big0 here, we save an alloc for the most common case (non-ether-transferring contract calls),
	// but it would make more sense to extend the usage of uint256.Int
	if !value.IsZero() {
		gas += params.CallStipend
		bigVal = value.ToBig()
	}

	ret, returnGas, err := evm.Call(contract, toAddr, args, gas, bigVal)
	if err != nil {
		stack.push(&uint256.Int{})
	} else {
		stack.push(new(uint256.Int).SetOne())
	}
	if err == nil || err == ErrExecutionReverted {
		memory.Set(retOffset, retSize, ret)
	}
	contract.Gas += returnGas
	return ret, nil
}

func opCallCode(_ *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	// Pop gas. The actual gas is in interpreter.evm.callGasTemp.
	stack.pop()
	gas := evm.callGasTemp
	// Pop other call parameters.
	addr, value := stack.popptr().Bytes20(), stack.pop()
	inOffset, inSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	retOffset, retSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	toAddr := common.Address(addr)
	// Get arguments from the memory.
	args := memory.GetCopy(int64(inOffset), int64(inSize))

	//TODO: use uint256.Int instead of converting with toBig()
	var bigVal = big0
	if !value.IsZero() {
		gas += params.CallStipend
		bigVal = value.ToBig()
	}

	ret, returnGas, err := evm.CallCode(contract, toAddr, args, gas, bigVal)
	if err != nil {
		stack.push(&uint256.Int{})
	} else {
		stack.push(new(uint256.Int).SetOne())
	}
	if err == nil || err == ErrExecutionReverted {
		memory.Set(retOffset, retSize, ret)
	}
	contract.Gas += returnGas
	return ret, nil
}

func opDelegateCall(_ *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	// Pop gas. The actual gas is in interpreter.evm.callGasTemp.
	stack.pop()
	gas := evm.callGasTemp
	// Pop other call parameters.
	addr := stack.popptr().Bytes20()
	inOffset, inSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	retOffset, retSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	toAddr := common.Address(addr)
	// Get arguments from the memory.
	args := memory.GetCopy(int64(inOffset), int64(inSize))

	ret, returnGas, err := evm.DelegateCall(contract, toAddr, args, gas)
	if err != nil {
		stack.push(&uint256.Int{})
	} else {
		stack.push(new(uint256.Int).SetOne())
	}
	if err == nil || err == ErrExecutionReverted {
		memory.Set(retOffset, retSize, ret)
	}
	contract.Gas += returnGas
	return ret, nil
}

func opStaticCall(_ *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	// We use it as a temporary value
	stack.pop()
	gas := evm.callGasTemp
	// Pop other call parameters.
	addr := stack.popptr().Bytes20()
	inOffset, inSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	retOffset, retSize := stack.popptr().Uint64(), stack.popptr().Uint64()
	toAddr := common.Address(addr)
	// Get arguments from the memory.
	args := memory.GetCopy(int64(inOffset), int64(inSize))

	ret, returnGas, err := evm.StaticCall(contract, toAddr, args, gas)
	if err != nil {
		stack.push(&uint256.Int{})
	} else {
		stack.push(new(uint256.Int).SetOne())
	}
	if err == nil || err == ErrExecutionReverted {
		memory.Set(retOffset, retSize, ret)
	}
	contract.Gas += returnGas
	return ret, nil
}

func opReturn(_ *uint64, _ *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	offset, size := stack.popptr(), stack.popptr()
	ret := memory.GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	return ret, nil
}

func opRevert(_ *uint64, _ *EVM, _ *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	offset, size := stack.popptr(), stack.popptr()
	ret := memory.GetPtr(int64(offset.Uint64()), int64(size.Uint64()))
	return ret, nil
}

func opStop(_ *uint64, _ *EVM, _ *Contract, _ *Memory, _ *Stack) ([]byte, error) {
	return nil, nil
}

func opSuicide(_ *uint64, evm *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	receiver := stack.popptr().Bytes20()

	if evm.vmConfig.Tracer != nil {
		bal := evm.StateDB.GetBalance(contract.Address())
		evm.vmConfig.Tracer.CaptureEnter(SELFDESTRUCT, contract.Address(), receiver, []byte{}, 0, bal)
		evm.vmConfig.Tracer.CaptureExit([]byte{}, 0, nil)
	}

	if evm.OnSuicideContract != nil {
		// let runtime do transfer things
		evm.OnSuicideContract(evm, contract.Address(), common.Address(receiver))
	}

	evm.StateDB.Suicide(contract.Address())
	return nil, nil
}

// opChainID implements CHAINID opcode
func opChainID(_ *uint64, evm *EVM, _ *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	chainID, _ := uint256.FromBig(evm.chainConfig.ChainID)
	stack.push(chainID)
	return nil, nil
}

func opSelfBalance(_ *uint64, evm *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	balance, _ := uint256.FromBig(evm.StateDB.GetBalance(contract.Address()))
	stack.push(balance)
	return nil, nil
}

// opBaseFee implements BASEFEE opcode
func opBaseFee(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	baseFee, _ := uint256.FromBig(evm.Context.BaseFee)
	stack.push(baseFee)
	return nil, nil
}

// opPush0 implements the PUSH0 opcode
func opPush0(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	stack.push(new(uint256.Int))
	return nil, nil
}

// following functions are used by the instruction jump  table

// make log instruction function
func makeLog(size int) executionFunc {
	return func(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
		topics := make([]common.Hash, size)
		mStart, mSize := stack.popptr().Uint64(), stack.popptr().Uint64()
		for i := range size {
			addr := stack.popptr()
			topics[i] = addr.Bytes32()
		}

		d := memory.GetCopy(int64(mStart), int64(mSize))
		evm.StateDB.AddLog(&types.Log{
			Address: contract.Address(),
			Topics:  topics,
			Data:    d,
			// This is a non-consensus field, but assigned here because
			// core/state doesn't know the current block number.
			BlockNumber: evm.BlockNumber.Uint64(),
		})

		return nil, nil
	}
}

// opPush1 is a specialized version of pushN
func opPush1(pc *uint64, _ *EVM, contract *Contract, _ *Memory, stack *Stack) ([]byte, error) {
	var (
		codeLen = uint64(len(contract.Code))
		integer = new(uint256.Int)
	)
	*pc++
	if *pc < codeLen {
		stack.push(integer.SetUint64(uint64(contract.Code[*pc])))
	} else {
		stack.push(integer.Clear())
	}
	return nil, nil
}

// make push instruction function
func makePush(size uint64, pushByteSize int) executionFunc {
	return func(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
		codeLen := len(contract.Code)

		startMin := min(int(*pc+1), codeLen)

		endMin := min(startMin+pushByteSize, codeLen)

		integer := new(uint256.Int)
		stack.push(integer.SetBytes(common.RightPadBytes(contract.Code[startMin:endMin], pushByteSize)))

		*pc += size
		return nil, nil
	}
}

// make dup instruction function
func makeDup(size int64) executionFunc {
	return func(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
		stack.dup(int(size))
		return nil, nil
	}
}

// make swap instruction function
func makeSwap(size int64) executionFunc {
	// switch n + 1 otherwise n would be swapped with n
	size++
	return func(pc *uint64, evm *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
		stack.swap(int(size))
		return nil, nil
	}
}
