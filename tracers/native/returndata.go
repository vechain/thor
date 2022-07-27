// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package native

import (
	"encoding/json"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/vm"
)

func init() {
	register("returnDataTracer", newReturnDataTracer)
}

type returnData struct {
	Error  string `json:"error,omitempty"`
	Output string `json:"output"`
}

type returnDataTracer struct {
	env    *vm.EVM
	data   returnData
	reason error // Textual reason for the interruption
}

// newReturnDataTracer returns a native go tracer which reveals
// the return data of a tx, and implements vm.EVMLogger.
func newReturnDataTracer(ctx *tracers.Context) tracers.Tracer {
	return &returnDataTracer{}
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *returnDataTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.env = env
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *returnDataTracer) CaptureEnd(output []byte, gasUsed uint64, _ time.Duration, err error) {
	if err == nil {
		t.data.Output = bytesToHex(output)
	} else {
		t.data.Error = err.Error()
		if err == vm.ErrExecutionReverted && len(output) > 0 {
			t.data.Output = bytesToHex(output)
		}
	}
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *returnDataTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, memory *vm.Memory, stack *vm.Stack, contract *vm.Contract, rData []byte, depth int, err error) {
}

// CaptureFault implements the EVMLogger interface to trace an execution fault.
func (t *returnDataTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, memory *vm.Memory, stack *vm.Stack, contract *vm.Contract, depth int, err error) {
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *returnDataTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *returnDataTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
}

// GetResult returns the json-encoded return data, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *returnDataTracer) GetResult() (json.RawMessage, error) {
	res, err := json.Marshal(t.data)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(res), t.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *returnDataTracer) Stop(err error) {
	t.reason = err
	t.env.Cancel()
}
