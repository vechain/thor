// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type Context struct {
	address thor.Address
	state   *state.State
	charger *gascharger.Charger
}

func NewContext(address thor.Address, state *state.State, charger *gascharger.Charger) *Context {
	return &Context{
		address: address,
		state:   state,
		charger: charger,
	}
}

func (c *Context) State() *state.State {
	return c.state
}

func (c *Context) UseGas(gas uint64) {
	if c.charger != nil {
		c.charger.Charge(gas)
	}
}
