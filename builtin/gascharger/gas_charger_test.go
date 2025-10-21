// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package gascharger

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/vm"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func TestNew_WithTestHook(t *testing.T) {
	// Clear any existing test hook
	ClearTestHook()

	var hookCalled bool
	var hookedCharger *Charger

	// Set up test hook
	SetTestHook(func(c *Charger) {
		hookCalled = true
		hookedCharger = c
		// Modify the charger in the hook for testing purposes
		c.customGas = 100
		c.totalGas = 100
	})

	env := &xenv.Environment{}
	charger := New(env)

	// Verify hook was called
	assert.True(t, hookCalled, "Test hook should have been called")
	assert.Equal(t, charger, hookedCharger, "Hook should receive the correct charger instance")

	// Verify hook modifications took effect
	assert.Equal(t, uint64(100), charger.customGas)
	assert.Equal(t, uint64(100), charger.totalGas)

	// Clean up
	ClearTestHook()
}

func TestSetTestHook(t *testing.T) {
	ClearTestHook()

	// Verify initially no hook
	assert.Nil(t, testHook)

	// Set a test hook
	hook := func(c *Charger) {
		c.sloadOps = 5
	}
	SetTestHook(hook)

	// Create charger and verify hook effect
	env := &xenv.Environment{}
	charger := New(env)

	assert.Equal(t, uint64(5), charger.sloadOps)

	ClearTestHook()
}

func TestClearTestHook(t *testing.T) {
	// Set a test hook
	SetTestHook(func(c *Charger) {
		c.customGas = 999
	})

	// Verify hook is set
	assert.NotNil(t, testHook)

	// Clear the hook
	ClearTestHook()

	// Verify hook is cleared
	assert.Nil(t, testHook)

	// Create charger and verify no hook effects
	env := &xenv.Environment{}
	charger := New(env)

	assert.Equal(t, uint64(0), charger.customGas)
}

func TestMultipleTestHookCalls(t *testing.T) {
	ClearTestHook()

	callCount := 0
	SetTestHook(func(c *Charger) {
		callCount++
		c.balanceOps = uint64(callCount)
	})

	env := &xenv.Environment{}

	// Create multiple chargers
	charger1 := New(env)
	charger2 := New(env)
	charger3 := New(env)

	// Verify hook was called for each
	assert.Equal(t, 3, callCount)
	assert.Equal(t, uint64(1), charger1.balanceOps)
	assert.Equal(t, uint64(2), charger2.balanceOps)
	assert.Equal(t, uint64(3), charger3.balanceOps)

	ClearTestHook()
}

func TestChargeBasicFunctionality(t *testing.T) {
	ClearTestHook()

	contract := vm.NewContract(vm.AccountRef(datagen.RandAddress()), nil, big.NewInt(10_000_000), 10_000_000)
	env := xenv.New(nil, nil, nil, nil, nil, nil, nil, contract, 0)
	charger := New(env)

	// Test SLOAD gas charging
	charger.Charge(thor.SloadGas)
	assert.Equal(t, uint64(1), charger.sloadOps)
	assert.Equal(t, thor.SloadGas, charger.totalGas)

	// Test SSTORE_SET gas charging
	charger.Charge(thor.SstoreSetGas)
	assert.Equal(t, uint64(1), charger.sstoreSetOps)
	assert.Equal(t, thor.SloadGas+thor.SstoreSetGas, charger.totalGas)

	// Test multiple operations
	charger.Charge(thor.SloadGas * 3)
	assert.Equal(t, uint64(4), charger.sloadOps) // 1 + 3

	// Test custom gas
	charger.Charge(999)
	assert.Equal(t, uint64(999), charger.customGas)
}

func TestBreakdown(t *testing.T) {
	ClearTestHook()

	contract := vm.NewContract(vm.AccountRef(datagen.RandAddress()), nil, big.NewInt(10_000_000), 10_000_000)
	env := xenv.New(nil, nil, nil, nil, nil, nil, nil, contract, 0)
	charger := New(env)

	// Add various operations
	charger.Charge(thor.SloadGas)
	charger.Charge(thor.SloadGas)
	charger.Charge(thor.SstoreSetGas)
	charger.Charge(thor.GetBalanceGas)
	charger.Charge(100) // custom gas

	breakdown := charger.Breakdown()

	// Verify breakdown contains expected information
	assert.Contains(t, breakdown, "SLOAD: 2 ops")
	assert.Contains(t, breakdown, "SSTORE_SET: 1 ops")
	assert.Contains(t, breakdown, "BALANCE: 1 ops")
	assert.Contains(t, breakdown, "CUSTOM: 100 gas")
}

func TestTotalGas(t *testing.T) {
	ClearTestHook()

	contract := vm.NewContract(vm.AccountRef(datagen.RandAddress()), nil, big.NewInt(10_000_000), 10_000_000)
	env := xenv.New(nil, nil, nil, nil, nil, nil, nil, contract, 0)
	charger := New(env)

	expectedTotal := thor.SloadGas + thor.SstoreSetGas + 500
	charger.Charge(thor.SloadGas)
	charger.Charge(thor.SstoreSetGas)
	charger.Charge(500)

	assert.Equal(t, expectedTotal, charger.TotalGas())
	assert.Equal(t, expectedTotal, charger.totalGas)
}
