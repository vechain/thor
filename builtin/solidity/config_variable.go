// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/thor"
)

type ConfigVariable struct {
	slot        thor.Bytes32
	name        string
	value       uint32
	initialised bool
}

func NewConfigVariable(name string, defaultValue uint32) *ConfigVariable {
	return &ConfigVariable{
		slot:        thor.BytesToBytes32([]byte(name)),
		name:        name,
		value:       defaultValue,
		initialised: false,
	}
}

func (c *ConfigVariable) Get() uint32 {
	return c.value
}

func (c *ConfigVariable) Name() string {
	return c.name
}

func (c *ConfigVariable) Slot() thor.Bytes32 {
	return c.slot
}

func (c *ConfigVariable) Override(ctx *Context) {
	// TODO: fix and uncomment this code. It seems to cause consensus issues
	// TODO: when starting a node with fork block = 0, the staker contract produces gas mismatches
	// TODO: are the variables being overridden at different points? eg. 1 node during consensus, another during transactions?
	//if c.initialised { // early return to prevent subsequent reads
	//	return
	//}
	//log.Warn("🛑🛑overriding config variable", "slot", c.Name(), "value", c.Get())
	num, err := NewUint256(ctx, c.slot).Get()
	if err != nil {
		log.Warn("failed to read config value", "slot", c.Name(), "error", err)
		return
	}
	c.initialised = true

	if num.Uint64() != 0 {
		c.value = uint32(num.Uint64())
		log.Debug("debug override found new config value", "slot", c.Name(), "value", c.Get())
	} else {
		log.Debug("using default config value", "slot", c.Name(), "value", c.Get())
	}
}
