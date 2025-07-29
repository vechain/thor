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

package native

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/tracers"
	"github.com/vechain/thor/v2/vm"
)

func init() {
	tracers.DefaultDirectory.Register("noopTracer", newNoopTracer, false)
}

// noopTracer is a go implementation of the Tracer interface which
// performs no action. It's mostly useful for testing purposes.
type noopTracer struct{}

// newNoopTracer returns a new noop tracer.
func newNoopTracer(_ json.RawMessage) (tracers.Tracer, error) {
	return &noopTracer{}, nil
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *noopTracer) CaptureStart(_ *vm.EVM, _ common.Address, _ common.Address, _ bool, _ []byte, _ uint64, _ *big.Int) {
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *noopTracer) CaptureEnd(_ []byte, _ uint64, _ error) {
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *noopTracer) CaptureState(_ uint64, _ vm.OpCode, _, _ uint64, _ *vm.Memory, _ *vm.Stack, _ *vm.Contract, _ []byte, _ int, _ error) {
}

// CaptureFault implements the EVMLogger interface to trace an execution fault.
func (t *noopTracer) CaptureFault(_ uint64, _ vm.OpCode, _, _ uint64, _ *vm.Memory, _ *vm.Stack, _ *vm.Contract, _ int, _ error) {
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *noopTracer) CaptureEnter(_ vm.OpCode, _ common.Address, _ common.Address, _ []byte, _ uint64, _ *big.Int) {
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *noopTracer) CaptureExit(_ []byte, _ uint64, _ error) {
}

// CaptureClauseStart implements the Tracer interface and is invoked at the beginning of
// clause processing.
func (*noopTracer) CaptureClauseStart(_ uint64) {}

// CaptureClauseEnd implements the Tracer interface and is invoked at the end of
// clause processing.
func (*noopTracer) CaptureClauseEnd(_ uint64) {}

// SetContext set the tracer context
func (t *noopTracer) SetContext(_ *tracers.Context) {
}

// GetResult returns an empty json object.
func (t *noopTracer) GetResult() (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *noopTracer) Stop(_ error) {
}
