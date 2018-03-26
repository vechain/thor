package executor

import (
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type Executor struct {
	addr  thor.Address
	state *state.State
}

func New(addr thor.Address, state *state.State) *Executor {
	return &Executor{addr, state}
}
