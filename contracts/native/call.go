package native

import (
	"errors"

	"github.com/vechain/thor/contracts/abi"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

var errNativeNotPermitted = errors.New("native: not permitted")

// Callable describes a native call.
type Callable struct {
	MethodPacker   *abi.MethodPacker
	Gas            uint64
	RequiredCaller *thor.Address
	AllocArg       func() interface{}
	Proc           func(env *Env) ([]interface{}, error)
}

// Env env of native call invocation.
type Env struct {
	State     *state.State
	VMContext *vm.Context
	Caller    thor.Address
	Arg       interface{}
}

// Call do native call.
func (c *Callable) Call(
	state *state.State,
	vmCtx *vm.Context,
	caller thor.Address,
	useGas func(uint64) bool,
	input []byte) ([]byte, error) {

	if c.RequiredCaller != nil {
		if caller != *c.RequiredCaller {
			return nil, errNativeNotPermitted
		}
	}
	if !useGas(c.Gas) {
		return nil, evm.ErrOutOfGas
	}

	var arg interface{}
	if c.AllocArg != nil {
		arg = c.AllocArg()
		if err := c.MethodPacker.UnpackInput(input, arg); err != nil {
			return nil, err
		}
	}

	out, err := c.Proc(&Env{state, vmCtx, caller, arg})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return c.MethodPacker.PackOutput(out...)
}
