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
	"errors"

	"github.com/ethereum/go-ethereum/params"
)

type (
	executionFunc       func(pc *uint64, env *EVM, contract *Contract, memory *Memory, stack *Stack) ([]byte, error)
	gasFunc             func(params.GasTable, *EVM, *Contract, *Stack, *Memory, uint64) (uint64, error) // last parameter is the requested memory size as a uint64
	stackValidationFunc func(*Stack) error
	memorySizeFunc      func(*Stack) (size uint64, overflow bool)
)

var errGasUintOverflow = errors.New("gas uint64 overflow")

type operation struct {
	// execute is the operation function
	execute executionFunc
	// gasCost is the gas function and returns the gas required for execution
	gasCost gasFunc
	// validateStack validates the stack (size) for the operation
	validateStack stackValidationFunc
	// memorySize returns the memory size required for the operation
	memorySize memorySizeFunc

	halts   bool // indicates whether the operation should halt further execution
	jumps   bool // indicates whether the program counter should not increment
	writes  bool // determines whether this a state modifying operation
	reverts bool // determines whether the operation reverts state (implicitly halts)
	returns bool // determines whether the operations sets the return data content
}

var (
	frontierInstructionSet       = NewFrontierInstructionSet()
	homesteadInstructionSet      = NewHomesteadInstructionSet()
	byzantiumInstructionSet      = NewByzantiumInstructionSet()
	constantinopleInstructionSet = NewConstantinopleInstructionSet()
	istanbulInstructionSet       = NewIstanbulInstructionSet()
)

type JumpTable *[256]*operation

func NewIstanbulInstructionSet() JumpTable {
	instructionSet := NewConstantinopleInstructionSet()
	// ChainID opcode
	instructionSet[CHAINID] = &operation{
		execute:       opChainID,
		gasCost:       constGasFunc(GasQuickStep),
		validateStack: makeStackFunc(0, 1),
	}

	// SelfBalance opcode
	instructionSet[SELFBALANCE] = &operation{
		execute:       opSelfBalance,
		gasCost:       constGasFunc(GasFastStep),
		validateStack: makeStackFunc(0, 1),
	}

	return instructionSet
}

// NewConstantinopleInstructionSet returns the frontier, homestead
// byzantium and contantinople instructions.
func NewConstantinopleInstructionSet() JumpTable {
	// instructions that can be executed during the byzantium phase.
	instructionSet := NewByzantiumInstructionSet()
	instructionSet[SHL] = &operation{
		execute:       opSHL,
		gasCost:       constGasFunc(GasFastestStep),
		validateStack: makeStackFunc(2, 1),
	}
	instructionSet[SHR] = &operation{
		execute:       opSHR,
		gasCost:       constGasFunc(GasFastestStep),
		validateStack: makeStackFunc(2, 1),
	}
	instructionSet[SAR] = &operation{
		execute:       opSAR,
		gasCost:       constGasFunc(GasFastestStep),
		validateStack: makeStackFunc(2, 1),
	}
	instructionSet[EXTCODEHASH] = &operation{
		execute:       opExtCodeHash,
		gasCost:       gasExtCodeHash,
		validateStack: makeStackFunc(1, 1),
	}
	instructionSet[CREATE2] = &operation{
		execute:       opCreate2,
		gasCost:       gasCreate2,
		validateStack: makeStackFunc(4, 1),
		memorySize:    memoryCreate2,
		writes:        true,
		returns:       true,
	}
	return instructionSet
}

// NewByzantiumInstructionSet returns the frontier, homestead and
// byzantium instructions.
func NewByzantiumInstructionSet() JumpTable {
	// instructions that can be executed during the homestead phase.
	instructionSet := NewHomesteadInstructionSet()
	instructionSet[STATICCALL] = &operation{
		execute:       opStaticCall,
		gasCost:       gasStaticCall,
		validateStack: makeStackFunc(6, 1),
		memorySize:    memoryStaticCall,
		returns:       true,
	}
	instructionSet[RETURNDATASIZE] = &operation{
		execute:       opReturnDataSize,
		gasCost:       constGasFunc(GasQuickStep),
		validateStack: makeStackFunc(0, 1),
	}
	instructionSet[RETURNDATACOPY] = &operation{
		execute:       opReturnDataCopy,
		gasCost:       gasReturnDataCopy,
		validateStack: makeStackFunc(3, 0),
		memorySize:    memoryReturnDataCopy,
	}
	instructionSet[REVERT] = &operation{
		execute:       opRevert,
		gasCost:       gasRevert,
		validateStack: makeStackFunc(2, 0),
		memorySize:    memoryRevert,
		reverts:       true,
		returns:       true,
	}
	return instructionSet
}

// NewHomesteadInstructionSet returns the frontier and homestead
// instructions that can be executed during the homestead phase.
func NewHomesteadInstructionSet() JumpTable {
	instructionSet := NewFrontierInstructionSet()
	instructionSet[DELEGATECALL] = &operation{
		execute:       opDelegateCall,
		gasCost:       gasDelegateCall,
		validateStack: makeStackFunc(6, 1),
		memorySize:    memoryDelegateCall,
		returns:       true,
	}
	return instructionSet
}

// NewFrontierInstructionSet returns the frontier instructions
// that can be executed during the frontier phase.
func NewFrontierInstructionSet() JumpTable {
	return &[256]*operation{
		STOP: {
			execute:       opStop,
			gasCost:       constGasFunc(0),
			validateStack: makeStackFunc(0, 0),
			halts:         true,
		},
		ADD: {
			execute:       opAdd,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		MUL: {
			execute:       opMul,
			gasCost:       constGasFunc(GasFastStep),
			validateStack: makeStackFunc(2, 1),
		},
		SUB: {
			execute:       opSub,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		DIV: {
			execute:       opDiv,
			gasCost:       constGasFunc(GasFastStep),
			validateStack: makeStackFunc(2, 1),
		},
		SDIV: {
			execute:       opSdiv,
			gasCost:       constGasFunc(GasFastStep),
			validateStack: makeStackFunc(2, 1),
		},
		MOD: {
			execute:       opMod,
			gasCost:       constGasFunc(GasFastStep),
			validateStack: makeStackFunc(2, 1),
		},
		SMOD: {
			execute:       opSmod,
			gasCost:       constGasFunc(GasFastStep),
			validateStack: makeStackFunc(2, 1),
		},
		ADDMOD: {
			execute:       opAddmod,
			gasCost:       constGasFunc(GasMidStep),
			validateStack: makeStackFunc(3, 1),
		},
		MULMOD: {
			execute:       opMulmod,
			gasCost:       constGasFunc(GasMidStep),
			validateStack: makeStackFunc(3, 1),
		},
		EXP: {
			execute:       opExp,
			gasCost:       gasExp,
			validateStack: makeStackFunc(2, 1),
		},
		SIGNEXTEND: {
			execute:       opSignExtend,
			gasCost:       constGasFunc(GasFastStep),
			validateStack: makeStackFunc(2, 1),
		},
		LT: {
			execute:       opLt,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		GT: {
			execute:       opGt,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		SLT: {
			execute:       opSlt,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		SGT: {
			execute:       opSgt,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		EQ: {
			execute:       opEq,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		ISZERO: {
			execute:       opIszero,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(1, 1),
		},
		AND: {
			execute:       opAnd,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		XOR: {
			execute:       opXor,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		OR: {
			execute:       opOr,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		NOT: {
			execute:       opNot,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(1, 1),
		},
		BYTE: {
			execute:       opByte,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(2, 1),
		},
		SHA3: {
			execute:       opSha3,
			gasCost:       gasSha3,
			validateStack: makeStackFunc(2, 1),
			memorySize:    memorySha3,
		},
		ADDRESS: {
			execute:       opAddress,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		BALANCE: {
			execute:       opBalance,
			gasCost:       gasBalance,
			validateStack: makeStackFunc(1, 1),
		},
		ORIGIN: {
			execute:       opOrigin,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		CALLER: {
			execute:       opCaller,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		CALLVALUE: {
			execute:       opCallValue,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		CALLDATALOAD: {
			execute:       opCallDataLoad,
			gasCost:       constGasFunc(GasFastestStep),
			validateStack: makeStackFunc(1, 1),
		},
		CALLDATASIZE: {
			execute:       opCallDataSize,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		CALLDATACOPY: {
			execute:       opCallDataCopy,
			gasCost:       gasCallDataCopy,
			validateStack: makeStackFunc(3, 0),
			memorySize:    memoryCallDataCopy,
		},
		CODESIZE: {
			execute:       opCodeSize,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		CODECOPY: {
			execute:       opCodeCopy,
			gasCost:       gasCodeCopy,
			validateStack: makeStackFunc(3, 0),
			memorySize:    memoryCodeCopy,
		},
		GASPRICE: {
			execute:       opGasprice,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		EXTCODESIZE: {
			execute:       opExtCodeSize,
			gasCost:       gasExtCodeSize,
			validateStack: makeStackFunc(1, 1),
		},
		EXTCODECOPY: {
			execute:       opExtCodeCopy,
			gasCost:       gasExtCodeCopy,
			validateStack: makeStackFunc(4, 0),
			memorySize:    memoryExtCodeCopy,
		},
		BLOCKHASH: {
			execute:       opBlockhash,
			gasCost:       constGasFunc(GasExtStep),
			validateStack: makeStackFunc(1, 1),
		},
		COINBASE: {
			execute:       opCoinbase,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		TIMESTAMP: {
			execute:       opTimestamp,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		NUMBER: {
			execute:       opNumber,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		DIFFICULTY: {
			execute:       opDifficulty,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		GASLIMIT: {
			execute:       opGasLimit,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		POP: {
			execute:       opPop,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(1, 0),
		},
		MLOAD: {
			execute:       opMload,
			gasCost:       gasMLoad,
			validateStack: makeStackFunc(1, 1),
			memorySize:    memoryMLoad,
		},
		MSTORE: {
			execute:       opMstore,
			gasCost:       gasMStore,
			validateStack: makeStackFunc(2, 0),
			memorySize:    memoryMStore,
		},
		MSTORE8: {
			execute:       opMstore8,
			gasCost:       gasMStore8,
			memorySize:    memoryMStore8,
			validateStack: makeStackFunc(2, 0),
		},
		SLOAD: {
			execute:       opSload,
			gasCost:       gasSLoad,
			validateStack: makeStackFunc(1, 1),
		},
		SSTORE: {
			execute:       opSstore,
			gasCost:       gasSStore,
			validateStack: makeStackFunc(2, 0),
			writes:        true,
		},
		JUMP: {
			execute:       opJump,
			gasCost:       constGasFunc(GasMidStep),
			validateStack: makeStackFunc(1, 0),
			jumps:         true,
		},
		JUMPI: {
			execute:       opJumpi,
			gasCost:       constGasFunc(GasSlowStep),
			validateStack: makeStackFunc(2, 0),
			jumps:         true,
		},
		PC: {
			execute:       opPc,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		MSIZE: {
			execute:       opMsize,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		GAS: {
			execute:       opGas,
			gasCost:       constGasFunc(GasQuickStep),
			validateStack: makeStackFunc(0, 1),
		},
		JUMPDEST: {
			execute:       opJumpdest,
			gasCost:       constGasFunc(params.JumpdestGas),
			validateStack: makeStackFunc(0, 0),
		},
		PUSH1: {
			execute:       opPush1,
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH2: {
			execute:       makePush(2, 2),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH3: {
			execute:       makePush(3, 3),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH4: {
			execute:       makePush(4, 4),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH5: {
			execute:       makePush(5, 5),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH6: {
			execute:       makePush(6, 6),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH7: {
			execute:       makePush(7, 7),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH8: {
			execute:       makePush(8, 8),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH9: {
			execute:       makePush(9, 9),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH10: {
			execute:       makePush(10, 10),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH11: {
			execute:       makePush(11, 11),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH12: {
			execute:       makePush(12, 12),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH13: {
			execute:       makePush(13, 13),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH14: {
			execute:       makePush(14, 14),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH15: {
			execute:       makePush(15, 15),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH16: {
			execute:       makePush(16, 16),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH17: {
			execute:       makePush(17, 17),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH18: {
			execute:       makePush(18, 18),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH19: {
			execute:       makePush(19, 19),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH20: {
			execute:       makePush(20, 20),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH21: {
			execute:       makePush(21, 21),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH22: {
			execute:       makePush(22, 22),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH23: {
			execute:       makePush(23, 23),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH24: {
			execute:       makePush(24, 24),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH25: {
			execute:       makePush(25, 25),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH26: {
			execute:       makePush(26, 26),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH27: {
			execute:       makePush(27, 27),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH28: {
			execute:       makePush(28, 28),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH29: {
			execute:       makePush(29, 29),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH30: {
			execute:       makePush(30, 30),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH31: {
			execute:       makePush(31, 31),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		PUSH32: {
			execute:       makePush(32, 32),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
		},
		DUP1: {
			execute:       makeDup(1),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(1),
		},
		DUP2: {
			execute:       makeDup(2),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(2),
		},
		DUP3: {
			execute:       makeDup(3),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(3),
		},
		DUP4: {
			execute:       makeDup(4),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(4),
		},
		DUP5: {
			execute:       makeDup(5),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(5),
		},
		DUP6: {
			execute:       makeDup(6),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(6),
		},
		DUP7: {
			execute:       makeDup(7),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(7),
		},
		DUP8: {
			execute:       makeDup(8),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(8),
		},
		DUP9: {
			execute:       makeDup(9),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(9),
		},
		DUP10: {
			execute:       makeDup(10),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(10),
		},
		DUP11: {
			execute:       makeDup(11),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(11),
		},
		DUP12: {
			execute:       makeDup(12),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(12),
		},
		DUP13: {
			execute:       makeDup(13),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(13),
		},
		DUP14: {
			execute:       makeDup(14),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(14),
		},
		DUP15: {
			execute:       makeDup(15),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(15),
		},
		DUP16: {
			execute:       makeDup(16),
			gasCost:       gasDup,
			validateStack: makeDupStackFunc(16),
		},
		SWAP1: {
			execute:       makeSwap(1),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(2),
		},
		SWAP2: {
			execute:       makeSwap(2),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(3),
		},
		SWAP3: {
			execute:       makeSwap(3),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(4),
		},
		SWAP4: {
			execute:       makeSwap(4),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(5),
		},
		SWAP5: {
			execute:       makeSwap(5),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(6),
		},
		SWAP6: {
			execute:       makeSwap(6),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(7),
		},
		SWAP7: {
			execute:       makeSwap(7),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(8),
		},
		SWAP8: {
			execute:       makeSwap(8),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(9),
		},
		SWAP9: {
			execute:       makeSwap(9),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(10),
		},
		SWAP10: {
			execute:       makeSwap(10),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(11),
		},
		SWAP11: {
			execute:       makeSwap(11),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(12),
		},
		SWAP12: {
			execute:       makeSwap(12),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(13),
		},
		SWAP13: {
			execute:       makeSwap(13),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(14),
		},
		SWAP14: {
			execute:       makeSwap(14),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(15),
		},
		SWAP15: {
			execute:       makeSwap(15),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(16),
		},
		SWAP16: {
			execute:       makeSwap(16),
			gasCost:       gasSwap,
			validateStack: makeSwapStackFunc(17),
		},
		LOG0: {
			execute:       makeLog(0),
			gasCost:       makeGasLog(0),
			validateStack: makeStackFunc(2, 0),
			memorySize:    memoryLog,
			writes:        true,
		},
		LOG1: {
			execute:       makeLog(1),
			gasCost:       makeGasLog(1),
			validateStack: makeStackFunc(3, 0),
			memorySize:    memoryLog,
			writes:        true,
		},
		LOG2: {
			execute:       makeLog(2),
			gasCost:       makeGasLog(2),
			validateStack: makeStackFunc(4, 0),
			memorySize:    memoryLog,
			writes:        true,
		},
		LOG3: {
			execute:       makeLog(3),
			gasCost:       makeGasLog(3),
			validateStack: makeStackFunc(5, 0),
			memorySize:    memoryLog,
			writes:        true,
		},
		LOG4: {
			execute:       makeLog(4),
			gasCost:       makeGasLog(4),
			validateStack: makeStackFunc(6, 0),
			memorySize:    memoryLog,
			writes:        true,
		},
		CREATE: {
			execute:       opCreate,
			gasCost:       gasCreate,
			validateStack: makeStackFunc(3, 1),
			memorySize:    memoryCreate,
			writes:        true,
			returns:       true,
		},
		CALL: {
			execute:       opCall,
			gasCost:       gasCall,
			validateStack: makeStackFunc(7, 1),
			memorySize:    memoryCall,
			returns:       true,
		},
		CALLCODE: {
			execute:       opCallCode,
			gasCost:       gasCallCode,
			validateStack: makeStackFunc(7, 1),
			memorySize:    memoryCall,
			returns:       true,
		},
		RETURN: {
			execute:       opReturn,
			gasCost:       gasReturn,
			validateStack: makeStackFunc(2, 0),
			memorySize:    memoryReturn,
			halts:         true,
		},
		SELFDESTRUCT: {
			execute:       opSuicide,
			gasCost:       gasSuicide,
			validateStack: makeStackFunc(1, 0),
			halts:         true,
			writes:        true,
		},
	}
}
