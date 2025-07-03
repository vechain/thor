// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// This test file validates gas costs for staker native functions using a test hook mechanism
// in the gascharger to capture and analyze gas usage patterns.
//
// Test Structure:
//   - Creates a test blockchain environment with proper dev accounts
//   - Uses gascharger test hooks to capture gas usage during native function execution
//   - Validates both successful execution and expected gas costs
//   - Provides detailed breakdowns for gas usage analysis
//
// Current Status:
//   - Covers basic read operations (no arguments)
//   - TODO: Implement argument handling for functions that require parameters
//   - TODO: Add test cases for state-modifying operations
package builtin_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

func TestStakerNativeGasCosts(t *testing.T) {
	testCases := []struct {
		name        string
		function    string
		expectedGas uint64
		args        []any
		description string
	}{
		// Basic read operations (no arguments)
		{
			name:        "native_totalStake",
			function:    "native_totalStake",
			expectedGas: 400,
			args:        []any{},
			description: "Get total locked stake",
		},
		{
			name:        "native_queuedStake",
			function:    "native_queuedStake",
			expectedGas: 400,
			args:        []any{},
			description: "Get total queued stake",
		},
		{
			name:        "native_firstActive",
			function:    "native_firstActive",
			expectedGas: 200,
			args:        []any{},
			description: "Get first active validator",
		},
		{
			name:        "native_firstQueued",
			function:    "native_firstQueued",
			expectedGas: 200,
			args:        []any{},
			description: "Get first queued validator",
		},
		{
			name:        "native_getDelegatorContract",
			function:    "native_getDelegatorContract",
			expectedGas: 200,
			args:        []any{},
			description: "Get delegator contract address",
		},
		// TODO: Functions with arguments - need proper argument handling implementation
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setup := createTestSetup(t, tc.function)

			// Capture the charger using test hook
			var capturedCharger *gascharger.Charger
			gascharger.SetTestHook(func(charger *gascharger.Charger) {
				capturedCharger = charger
			})
			defer gascharger.ClearTestHook()

			// Execute the native function
			result := executeNativeFunction(t, setup, tc.function, tc.args)

			// Validate we captured the charger
			require.NotNil(t, capturedCharger, "Should have captured charger for %s", tc.name)
			gasUsed := capturedCharger.TotalGas()

			// Validate gas usage with descriptive error message
			assert.Equal(t, tc.expectedGas, gasUsed,
				"Gas usage mismatch for %s (%s):\nExpected: %d\nActual: %d\nBreakdown: %s",
				tc.name, tc.description, tc.expectedGas, gasUsed, capturedCharger.Breakdown())

			// Validate function executed successfully (no revert)
			require.NotNil(t, result, "Function %s should return result", tc.function)
			require.Greater(t, len(result), 0, "Function %s should return at least one value", tc.function)

			// Check if last element is an error string (staker native functions return error as last element)
			if len(result) > 0 {
				lastElem := result[len(result)-1]
				if errorStr, ok := lastElem.(string); ok {
					assert.Empty(t, errorStr, "Function %s should not return error but got: %s", tc.function, errorStr)
				}
			}

			// Log detailed breakdown for analysis
			//t.Logf("=== %s ===", tc.name)
			//t.Logf("Function: %s", tc.function)
			//t.Logf("Description: %s", tc.description)
			//t.Logf("Expected gas: %d", tc.expectedGas)
			//t.Logf("Actual gas used: %d", gasUsed)
			//t.Logf("Result length: %d", len(result))
			//t.Logf("Gas breakdown: %s", capturedCharger.Breakdown())

			// Additional validation: gas should be reasonable (not zero, not excessive)
			assert.Greater(t, gasUsed, uint64(0), "Function %s should consume some gas", tc.function)
			assert.Less(t, gasUsed, uint64(10000), "Function %s gas usage seems excessive: %d", tc.function, gasUsed)
		})
	}
}

// Helper functions

type testSetup struct {
	chain    *testchain.Chain
	env      *xenv.Environment
	master   thor.Address
	endorsor thor.Address
}

func executeNativeFunction(t *testing.T, setup *testSetup, functionName string, args []any) []any {
	// Find the native function
	stakerAbi := builtin.Staker.NativeABI()
	method, found := stakerAbi.MethodByName(functionName)
	require.True(t, found, "Function %s not found", functionName)

	// TODO: Handle method arguments properly when test cases with args are added
	// For now, we'll implement argument handling when we have test cases that need it
	if len(args) > 0 {
		t.Logf("Function %s called with %d arguments (argument handling to be implemented)", functionName, len(args))
	}

	// Get the native method implementation
	methodID := method.ID()
	nativeMethod, run, found := builtin.FindNativeCall(builtin.Staker.Address, methodID[:])
	require.True(t, found, "Native method not found for %s", functionName)
	require.NotNil(t, nativeMethod, "Native method is nil for %s", functionName)

	// Execute the native function - this will trigger our test hook
	result := run(setup.env)

	return result
}

func createTestSetup(t *testing.T, functionName string) *testSetup {
	// Create test chain
	chain, err := testchain.NewDefault()
	require.NoError(t, err)

	// Use proper address generation from dev accounts
	master := genesis.DevAccounts()[0].Address
	endorsor := genesis.DevAccounts()[1].Address

	// Get latest state
	bestBlock, err := chain.BestBlock()
	require.NoError(t, err)

	// Create transaction context
	tx := new(tx.Builder).
		ChainTag(chain.Repo().ChainTag()).
		BlockRef(tx.NewBlockRef(bestBlock.Header().Number())).
		Expiration(32).
		Gas(1000000).
		Clause(tx.NewClause(nil)).
		Build()

	// Create mock EVM and contract
	evm := vm.NewEVM(
		vm.Context{
			BlockNumber: big.NewInt(1),
			GasPrice:    big.NewInt(1),
			CanTransfer: vm.NoopCanTransfer,
			Transfer:    vm.NoopTransfer,
		},
		vm.NoopStateDB{},
		&vm.ChainConfig{ChainConfig: *params.TestChainConfig}, vm.Config{})
	contract := vm.NewContract(
		vm.AccountRef(master),
		vm.AccountRef(builtin.Staker.Address),
		big.NewInt(0),
		1000000,
	)

	// Use the actual function name being tested
	method, ok := builtin.Staker.NativeABI().MethodByName(functionName)
	require.True(t, ok, "Method %s not found", functionName)

	root := chain.Repo().BestBlockSummary().Root()
	// Create environment
	env := xenv.New(
		method,
		nil,
		chain.Stater().NewState(root),
		&xenv.BlockContext{
			Number:     bestBlock.Header().Number(),
			Time:       bestBlock.Header().Timestamp(),
			GasLimit:   bestBlock.Header().GasLimit(),
			TotalScore: bestBlock.Header().TotalScore(),
			Signer:     master,
		},
		&xenv.TransactionContext{
			ID:         tx.ID(),
			Origin:     master,
			GasPayer:   master,
			ProvedWork: big.NewInt(1000),
			BlockRef:   tx.BlockRef(),
			Expiration: tx.Expiration(),
		},
		evm,
		contract,
		0, // Clause index
	)

	return &testSetup{
		chain:    chain,
		env:      env,
		master:   master,
		endorsor: endorsor,
	}
}
