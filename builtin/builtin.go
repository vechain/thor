package builtin

import (
	"github.com/vechain/thor/builtin/authority"
	"github.com/vechain/thor/builtin/energy"
	"github.com/vechain/thor/builtin/executor"
	"github.com/vechain/thor/builtin/params"
	"github.com/vechain/thor/state"
)

// Builtin contracts binding.
var (
	Params    = &paramsContract{mustLoadContract("Params")}
	Authority = &authorityContract{mustLoadContract("Authority")}
	Energy    = &energyContract{mustLoadContract("Energy")}
	Executor  = &executorContract{mustLoadContract("Executor")}
)

type (
	paramsContract    struct{ *contract }
	authorityContract struct{ *contract }
	energyContract    struct{ *contract }
	executorContract  struct{ *contract }
)

func (p *paramsContract) WithState(state *state.State) *params.Params {
	return params.New(p.Address, state)
}

func (a *authorityContract) WithState(state *state.State) *authority.Authority {
	return authority.New(a.Address, state)
}

func (e *energyContract) WithState(state *state.State) *energy.Energy {
	return energy.New(e.Address, state)
}

func (e *executorContract) WithState(state *state.State) *executor.Executor {
	return executor.New(e.Address, state)
}
