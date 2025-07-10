package solidity

import (
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type Context struct {
	Address thor.Address
	State   *state.State
	Charger *gascharger.Charger
}

func (c *Context) UseGas(gas uint64) {
	if c.Charger != nil {
		c.Charger.Charge(gas)
	}
}
