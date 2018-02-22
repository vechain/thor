package native

import (
	"errors"
	"fmt"

	"github.com/vechain/thor/builtin/abi"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

var errNativeNotPermitted = errors.New("native: not permitted")

// Callable describes a native call.
type Callable struct {
	MethodCodec    *abi.MethodCodec
	Gas            uint64
	RequiredCaller *thor.Address
	Proc           func(env *Env) ([]interface{}, error)
}

// Call do native call.
func (c *Callable) Call(
	state *state.State,
	vmCtx *vm.Context,
	caller thor.Address,
	useGas func(uint64) bool,
	input []byte) (output []byte, err error) {

	if c.RequiredCaller != nil {
		if caller != *c.RequiredCaller {
			return nil, errNativeNotPermitted
		}
	}
	if !useGas(c.Gas) {
		return nil, evm.ErrOutOfGas
	}

	defer func() {
		// handle panic in Env.Args
		if e := recover(); e != nil {
			err = fmt.Errorf("native: %v", e)
		}
	}()

	out, err := c.Proc(&Env{
		state,
		vmCtx,
		caller,
		c,
		input})

	if err != nil {
		return nil, err
	}
	return c.MethodCodec.EncodeOutput(out...)
}

// Env env of native call invocation.
type Env struct {
	State     *state.State
	VMContext *vm.Context
	Caller    thor.Address

	callable *Callable
	input    []byte
}

// Args unpack input into args.
func (env *Env) Args(v interface{}) {
	if err := env.callable.MethodCodec.DecodeInput(env.input, v); err != nil {
		// Callable.Call will handle it
		panic(err)
	}
}
