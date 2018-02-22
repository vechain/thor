package contracts

import (
	"math/big"

	"github.com/vechain/thor/contracts/sslot"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Params binder of `Params` contract.
var Params = func() *params {
	c := loadContract("Params")
	return &params{
		c,
		sslot.NewMap(c.Address, 100),
	}
}()

type params struct {
	*contract
	dataSlot *sslot.Map
}

// Get native way to get param.
func (c *params) Get(state *state.State, key thor.Hash) *big.Int {
	var v big.Int
	c.dataSlot.ForKey(key).Load(state, &v)
	return &v
}

// Set native way to set param.
func (c *params) Set(state *state.State, key thor.Hash, value *big.Int) {
	c.dataSlot.ForKey(key).Save(state, value)
}
