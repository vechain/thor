package builtin

import (
	"errors"
	"fmt"

	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

var errNativeNotPermitted = errors.New("native call: not permitted")

// nativeMethod describes a native call.
type nativeMethod struct {
	addr   thor.Address
	method *abi.Method
	gas    uint64
	run    func(env *env) ([]interface{}, error)
}

// Call do native call.
func (n *nativeMethod) Call(
	state *state.State,
	vmCtx *vm.Context,
	caller thor.Address,
	useGas func(uint64) bool,
	input []byte) (output []byte, err error) {

	if n.addr != caller {
		return nil, errNativeNotPermitted
	}

	if !useGas(n.gas) {
		return nil, evm.ErrOutOfGas
	}

	defer func() {
		// handle panic in Env.Args
		if e := recover(); e != nil {
			err = fmt.Errorf("native: %v", e)
		}
	}()

	out, err := n.run(&env{
		state,
		vmCtx,
		input,
		n.method})

	if err != nil {
		return nil, err
	}
	return n.method.EncodeOutput(out...)
}

// env env of native call invocation.
type env struct {
	State     *state.State
	VMContext *vm.Context

	input  []byte
	method *abi.Method
}

// Args unpack input into args.
func (e *env) Args(v interface{}) {
	if err := e.method.DecodeInput(e.input, v); err != nil {
		// Callable.Call will handle it
		panic(err)
	}
}
