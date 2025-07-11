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
	Address thor.Address
	State   *state.State
	Charger *gascharger.Charger
}

func (c *Context) UseGas(gas uint64) {
	if c.Charger != nil {
		c.Charger.Charge(gas)
	}
}
