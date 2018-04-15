package builtin

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

// nativeMethod defines abi and impl of a native method.
type nativeMethod struct {
	ABI *abi.Method
	Run func(env *bridge) []interface{}
}

// bridge bridges VM OPCALL to native implementation.
type bridge struct {
	Method *nativeMethod
	State  *state.State
	VMCtx  *vm.Context
	To     thor.Address
	Caller thor.Address

	UseGas func(uint64)
	Log    func(event *abi.Event, topics []thor.Bytes32, args ...interface{})

	ParseArgs func(val interface{})
	Require   func(cond bool)
}

// newBridge creates a new birdge instance.
func newBridge(
	method *nativeMethod,
	state *state.State,
	vmCtx *vm.Context,
	to thor.Address,
	input []byte,
	caller thor.Address,
	useGas func(uint64) bool,
	log func(*types.Log),
) *bridge {

	mustUseGas := func(gas uint64) {
		if !useGas(gas) {
			panic(&recoverable{evm.ErrOutOfGas})
		}
	}
	mustLog := func(event *abi.Event, topics []thor.Bytes32, args ...interface{}) {
		data, err := event.Encode(args...)
		if err != nil {
			panic(errors.Wrap(err, "encode native event"))
		}
		mustUseGas(ethparams.LogGas + ethparams.LogTopicGas*uint64(len(topics)) + ethparams.LogDataGas*uint64(len(data)))

		etopics := make([]common.Hash, 0, len(topics)+1)
		etopics = append(etopics, common.Hash(event.ID()))
		for _, t := range topics {
			etopics = append(etopics, common.Hash(t))
		}
		log(&types.Log{
			Address: common.Address(to),
			Topics:  etopics,
			Data:    data,
		})
	}
	mustParseArgs := func(val interface{}) {
		if err := method.ABI.DecodeInput(input, val); err != nil {
			panic(&recoverable{errors.Wrap(err, "decode native input")})
		}
	}
	require := func(cond bool) {
		if !cond {
			panic(&recoverable{evm.ErrExecutionReverted()})
		}
	}

	return &bridge{
		method,
		state,
		vmCtx,
		to,
		caller,
		mustUseGas,
		mustLog,
		mustParseArgs,
		require,
	}
}

func (b *bridge) Call() (output []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			if rec, ok := e.(*recoverable); ok {
				err = rec.cause
			} else {
				panic(e)
			}
		}
	}()

	outArgs := b.Method.Run(b)
	output, err = b.Method.ABI.EncodeOutput(outArgs...)
	if err != nil {
		panic(errors.Wrap(err, "encode native output"))
	}
	return
}

// recoverable error, that can be passed to VM.
// used at
// 1. parsing input data
// 2. use gas
// 3. require condition
type recoverable struct {
	cause error
}
