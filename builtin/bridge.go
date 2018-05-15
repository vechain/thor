package builtin

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
)

// nativeMethod defines abi and impl of a native method.
type nativeMethod struct {
	ABI *abi.Method
	Run func(env *bridge) []interface{}
}

// bridge bridges VM OPCALL to native implementation.
type bridge struct {
	Method   *nativeMethod
	State    *state.State
	VM       *evm.EVM
	Contract *evm.Contract
}

// newBridge creates a new birdge instance.
func newBridge(
	method *nativeMethod,
	state *state.State,
	vm *evm.EVM,
	contract *evm.Contract,
) *bridge {
	return &bridge{
		method,
		state,
		vm,
		contract,
	}
}
func (b *bridge) UseGas(gas uint64) {
	if !b.Contract.UseGas(gas) {
		panic(&vmerror{evm.ErrOutOfGas})
	}
}

func (b *bridge) Log(event *abi.Event, topics []thor.Bytes32, args ...interface{}) {
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
	b.VM.StateDB.AddLog(&types.Log{
		Address: common.Address(b.Contract.Address()),
		Topics:  etopics,
		Data:    data,
	})
}

func (b *bridge) Call() (output []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			if rec, ok := e.(*vmerror); ok {
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

func (b *bridge) ParseArgs(val interface{}) {
	if err := b.Method.ABI.DecodeInput(b.Contract.Input, val); err != nil {
		panic(&vmerror{errors.Wrap(err, "decode native input")})
	}
}

func (b *bridge) Require(cond bool) {
	if !cond {
		panic(&vmerror{evm.ErrExecutionReverted()})
	}
}

func (b *bridge) BlockTime() uint64 {
	return b.VM.Time.Uint64()
}

// func (b *bridge) BlockNumber() uint32 {
// 	return uint32(b.VM.BlockNumber.Uint64())
// }

func (b *bridge) Caller() thor.Address {
	return thor.Address(b.Contract.Caller())
}

func (b *bridge) To() thor.Address {
	return thor.Address(b.Contract.Address())
}

func (b *bridge) Stop(vmerr error) {
	panic(&vmerror{vmerr})
}

// vmerror, that is safe to be returned to VM.
// used at
// 1. parsing input data
// 2. use gas
// 3. require condition
// 4. stop
type vmerror struct {
	cause error
}
