// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package gascharger

import (
	"fmt"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

// Test hook - only used during testing
var testHook func(*Charger) = nil

type Charger struct {
	env            *xenv.Environment
	sloadOps       uint64
	sstoreSetOps   uint64
	sstoreResetOps uint64
	customGas      uint64
	balanceOps     uint64
	totalGas       uint64
}

func New(env *xenv.Environment) *Charger {
	charger := &Charger{
		env: env,
	}

	// Call test hook if it exists
	if testHook != nil {
		testHook(charger)
	}

	return charger
}

func (c *Charger) Charge(gas uint64) {
	c.totalGas += gas

	switch {
	// Handle multiples and single operations
	case gas%thor.SstoreSetGas == 0 && gas > 0:
		count := gas / thor.SstoreSetGas
		c.sstoreSetOps += count

	case gas%thor.SstoreResetGas == 0 && gas > 0:
		count := gas / thor.SstoreResetGas
		c.sstoreResetOps += count

	case gas%thor.GetBalanceGas == 0 && gas > 0:
		count := gas / thor.GetBalanceGas
		c.balanceOps += count

	case gas%thor.SloadGas == 0 && gas > 0:
		count := gas / thor.SloadGas
		c.sloadOps += count

	default:
		// Unknown/custom gas amount
		c.customGas += gas
	}

	c.env.UseGas(gas)
}

func (c *Charger) Breakdown() string {
	return fmt.Sprintf(
		"SLOAD: %d ops (%d gas) | SSTORE_SET: %d ops (%d gas) | SSTORE_RESET: %d ops (%d gas) | BALANCE: %d ops (%d gas) | CUSTOM: %d gas | TOTAL: %d gas",
		c.sloadOps,
		c.sloadOps*thor.SloadGas,
		c.sstoreSetOps,
		c.sstoreSetOps*thor.SstoreSetGas,
		c.sstoreResetOps,
		c.sstoreResetOps*thor.SstoreResetGas,
		c.balanceOps,
		c.balanceOps*thor.GetBalanceGas,
		c.customGas,
		c.totalGas,
	)
}

func (c *Charger) TotalGas() uint64 {
	return c.totalGas
}

// Test helper functions

func SetTestHook(hook func(*Charger)) {
	testHook = hook
}

func ClearTestHook() {
	testHook = nil
}
