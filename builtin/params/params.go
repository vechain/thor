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
func (p *Params) Get(key thor.Hash) *big.Int {
	var v big.Int
	p.state.GetStructedStorage(p.addr, key, &v)
	return &v
}

// Set native way to set param.
func (p *Params) Set(key thor.Hash, value *big.Int) {
	p.state.SetStructedStorage(p.addr, key, value)
}
