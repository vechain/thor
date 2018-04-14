package bridge

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

// NativeMethod defines abi and impl of a native method.
type NativeMethod struct {
	ABI *abi.Method
	Run func(env *Bridge) []interface{}
}

// Bridge bridges evm CALL and native method implementation.
type Bridge struct {
	method *NativeMethod
	state  *state.State
	vmCtx  *vm.Context
	useGas func(uint64) bool
	log    func(*types.Log)
	to     thor.Address
	input  []byte
	caller thor.Address
}

// New create a new Bridge instance.
func New(
	method *NativeMethod,
	state *state.State,
	vmCtx *vm.Context,
	useGas func(uint64) bool,
	log func(*types.Log),
	to thor.Address,
	input []byte,
	caller thor.Address,
) *Bridge {
	return &Bridge{
		method,
		state,
		vmCtx,
		useGas,
		log,
		to,
		input,
		caller,
	}
}

// State returns state object.
func (b *Bridge) State() *state.State {
	return b.state
}

// VMContext returns evm context of OP CALL.
func (b *Bridge) VMContext() *vm.Context {
	return b.vmCtx
}

// To returns destination of OP CALL.
func (b *Bridge) To() thor.Address {
	return b.to
}

// Caller returns msg sender of OP CALL.
func (b *Bridge) Caller() thor.Address {
	return b.caller
}

// Require similar to 'require' in solidity.
func (b *Bridge) Require(cond bool) {
	if !cond {
		panic(&recoverable{evm.ErrExecutionReverted()})
	}
}

// ParseArgs parses input data according to method ABI.
func (b *Bridge) ParseArgs(val interface{}) {
	if err := b.method.ABI.DecodeInput(b.input, val); err != nil {
		panic(&recoverable{errors.Wrap(err, "decode native input")})
	}
}

// Log simulates OP_LOG*.
func (b *Bridge) Log(event *abi.Event, topics []thor.Bytes32, args ...interface{}) {
	data, err := event.Encode(args...)
	if err != nil {
		panic(errors.Wrap(err, "encode native event"))
	}
	b.UseGas(ethparams.LogGas + ethparams.LogTopicGas*uint64(len(topics)) + ethparams.LogDataGas*uint64(len(data)))

	etopics := make([]common.Hash, 0, len(topics)+1)
	etopics = append(etopics, common.Hash(event.ID()))
	for _, t := range topics {
		etopics = append(etopics, common.Hash(t))
	}
	b.log(&types.Log{
		Address: common.Address(b.to),
		Topics:  etopics,
		Data:    data,
	})
}

// UseGas records gas using during native call.
func (b *Bridge) UseGas(gas uint64) {
	if !b.useGas(gas) {
		panic(&recoverable{evm.ErrOutOfGas})
	}
}

// Call performs native call.
func (b *Bridge) Call() (output []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			if rec, ok := e.(*recoverable); ok {
				err = rec.cause
			} else {
				panic(e)
			}
		}
	}()

	outArgs := b.method.Run(b)
	output, err = b.method.ABI.EncodeOutput(outArgs...)
	if err != nil {
		panic(errors.Wrap(err, "encode native output"))
	}
	return
}

// recoverable error
// used at
// 1. parsing input data
// 2. use gas
// 3. require condition
type recoverable struct {
	cause error
}
