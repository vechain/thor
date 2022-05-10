package vm

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Logger is used to collect execution traces from an EVM transaction
// execution. CaptureState is called for each step of the VM with the
// current VM state.
// Note that reference types are actual VM data structures; make copies
// if you need to retain them beyond the current call.
type Logger interface {
	// Top call frame
	CaptureStart(env *EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int)
	CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error)
	// Rest of call frames
	CaptureEnter(typ OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) // !!
	CaptureExit(output []byte, gasUsed uint64, err error)                                                      // !!
	// Opcode level
	CaptureState(pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, rData []byte, depth int, err error)
	CaptureFault(pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, depth int, err error)
}
