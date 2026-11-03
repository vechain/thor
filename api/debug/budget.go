// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/tracers"
	"github.com/vechain/thor/v2/vm"
)

// dataBudgetTracer caps the call input and output bytes handed to the wrapped
// tracer over one trace. Gas does not bound them: CALL arguments carry no
// per-byte gas, so an execution can pass the same memory region on every call
// for a flat fee. Once the budget is exceeded the EVM is cancelled, no further
// call data reaches the wrapped tracer and err is set; the caller must then
// discard the trace instead of calling GetResult.
//
// It never calls the wrapped tracer's Stop: the request watchdog may call Stop
// concurrently, and tracers do not guard Stop against concurrent callers.
type dataBudgetTracer struct {
	tracers.Tracer
	max  uint64
	left uint64
	evm  *vm.EVM
	err  error
}

func newDataBudgetTracer(inner tracers.Tracer, maxSize uint64) *dataBudgetTracer {
	return &dataBudgetTracer{Tracer: inner, max: maxSize, left: maxSize}
}

// spend reports whether n more bytes fit in the budget, and trips it otherwise.
func (t *dataBudgetTracer) spend(n int) bool {
	if t.err != nil {
		return false
	}
	if uint64(n) > t.left {
		t.err = fmt.Errorf("trace data exceeds limit of %d bytes", t.max)
		if t.evm != nil {
			t.evm.Cancel()
		}
		return false
	}
	t.left -= uint64(n)
	return true
}

func (t *dataBudgetTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.evm = env
	if t.spend(len(input)) {
		t.Tracer.CaptureStart(env, from, to, create, input, gas, value)
	}
}

func (t *dataBudgetTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if t.spend(len(output)) {
		t.Tracer.CaptureEnd(output, gasUsed, err)
	}
}

func (t *dataBudgetTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.spend(len(input)) {
		t.Tracer.CaptureEnter(typ, from, to, input, gas, value)
	}
}

func (t *dataBudgetTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	if t.spend(len(output)) {
		t.Tracer.CaptureExit(output, gasUsed, err)
	}
}
