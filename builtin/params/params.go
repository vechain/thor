package params

import (
	"math/big"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Params binder of `Params` contract.
type Params struct {
	addr  thor.Address
	state *state.State
}

func New(addr thor.Address, state *state.State) *Params {
	return &Params{addr, state}
}

// Get native way to get param.
func (p *Params) Get(key thor.Bytes32) *big.Int {
	var v big.Int
	p.state.GetStructuredStorage(p.addr, key, &v)
	return &v
}

// Set native way to set param.
func (p *Params) Set(key thor.Bytes32, value *big.Int) {
	p.state.SetStructuredStorage(p.addr, key, value)
}
