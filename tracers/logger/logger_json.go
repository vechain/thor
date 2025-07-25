// Copyright 2021 The go-ethereum Authors
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

package logger

import (
	"encoding/json"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/vm"
)

type JSONLogger struct {
	encoder *json.Encoder
	cfg     *Config
	env     *vm.EVM
}

// NewJSONLogger creates a new EVM tracer that prints execution steps as JSON objects
// into the provided stream.
func NewJSONLogger(cfg *Config, writer io.Writer) *JSONLogger {
	l := &JSONLogger{encoder: json.NewEncoder(writer), cfg: cfg}
	if l.cfg == nil {
		l.cfg = &Config{}
	}
	return l
}

func (l *JSONLogger) CaptureStart(env *vm.EVM, _, _ common.Address, _ bool, _ []byte, _ uint64, _ *big.Int) {
	l.env = env
}

func (l *JSONLogger) CaptureFault(
	pc uint64,
	op vm.OpCode,
	gas uint64,
	cost uint64,
	memory *vm.Memory,
	stack *vm.Stack,
	contract *vm.Contract,
	depth int,
	err error,
) {
	// TODO: Add rData to this interface as well
	l.CaptureState(pc, op, gas, cost, memory, stack, contract, nil, depth, err)
}

// CaptureState outputs state information on the logger.
func (l *JSONLogger) CaptureState(
	pc uint64,
	op vm.OpCode,
	gas, cost uint64,
	memory *vm.Memory,
	stack *vm.Stack,
	_ *vm.Contract,
	rData []byte,
	depth int,
	err error,
) {
	log := StructLog{
		Pc:            pc,
		Op:            op,
		Gas:           gas,
		GasCost:       cost,
		MemorySize:    memory.Len(),
		Depth:         depth,
		RefundCounter: l.env.StateDB.GetRefund(),
		Err:           err,
	}
	if l.cfg.EnableMemory {
		log.Memory = memory.Data()
	}
	if !l.cfg.DisableStack {
		log.Stack = stack.Data()
	}
	if l.cfg.EnableReturnData {
		log.ReturnData = rData
	}
	l.encoder.Encode(log)
}

// CaptureEnd is triggered at end of execution.
func (l *JSONLogger) CaptureEnd(output []byte, gasUsed uint64, err error) {
	type endLog struct {
		Output  string              `json:"output"`
		GasUsed math.HexOrDecimal64 `json:"gasUsed"`
		Err     string              `json:"error,omitempty"`
	}
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	l.encoder.Encode(endLog{common.Bytes2Hex(output), math.HexOrDecimal64(gasUsed), errMsg})
}

func (l *JSONLogger) CaptureEnter(_ vm.OpCode, _ common.Address, _ common.Address, _ []byte, _ uint64, _ *big.Int) {
}

func (l *JSONLogger) CaptureExit(_ []byte, _ uint64, _ error) {}
