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

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

type TestHook func(*testing.T, *testSetup)

func preTestAddValidation(acc thor.Address) TestHook {
	stake := big.NewInt(0).Mul(staker.MinStake, big.NewInt(2))
	return func(t *testing.T, setup *testSetup) {
		executeNativeFunction(t, setup, "native_addValidation", []any{
			acc,
			acc,
			thor.LowStakingPeriod(),
			stake,
		})
	}
}

func preTestAddDelegation(acc thor.Address) TestHook {
	return func(t *testing.T, setup *testSetup) {
		executeNativeFunction(t, setup, "native_addDelegation", []any{
			acc,
			staker.MinStake,
			uint8(150),
		})
	}
}

func TestStakerNativeGasCosts(t *testing.T) {
	account1 := genesis.DevAccounts()[0].Address

	testCases := []struct {
		function     string
		expectedGas  uint64
		args         []any
		description  string
		err          string
		preTestHooks []TestHook
	}{
		// Basic read operations (no arguments)
		{
			function:    "native_totalStake",
			expectedGas: 200,
			args:        []any{},
			description: "Get total locked stake",
		},
		{
			function:    "native_queuedStake",
			expectedGas: 200,
			args:        []any{},
			description: "Get total queued stake",
		},
		{
			function:    "native_firstActive",
			expectedGas: 200,
			args:        []any{},
			description: "Get first active validator",
		},
		{
			function:    "native_firstQueued",
			expectedGas: 200,
			args:        []any{},
			description: "Get first queued validator",
		},
		{
			function:    "native_getDelegatorContract",
			expectedGas: 200,
			args:        []any{},
			description: "Get delegator contract address",
		},
		{
			function:    "native_addValidation",
			expectedGas: 121200,
			args:        []any{account1, account1, thor.LowStakingPeriod(), staker.MinStake},
			description: "Add a new validator (not implemented yet)",
		},
		{
			function:     "native_getValidation",
			expectedGas:  400,
			args:         []any{account1},
			description:  "Get validator stake by it's ID",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:     "native_getWithdrawable",
			expectedGas:  400,
			args:         []any{account1},
			description:  "Get withdraw information for a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:     "native_next",
			expectedGas:  400,
			args:         []any{account1},
			description:  "Get next validator in the queue",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:     "native_withdrawStake",
			expectedGas:  31200,
			args:         []any{account1, account1},
			description:  "Withdraw stake for a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		// {
		// 	function:    "native_signalExit",
		// 	expectedGas: 16600,
		// 	args: []any{
		// 		account1,
		// 		accToID(account1),
		// 	},
		// 	description:  "Signal exit for a validator",
		// 	preTestHooks: []TestHook{preTestAddValidation(account1)},
		// },
		{
			function:    "native_increaseStake",
			expectedGas: 15800,
			args: []any{
				account1,
				account1,
				staker.MinStake,
			},
			description:  "Increase stake for a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:    "native_decreaseStake",
			expectedGas: 15600,
			args: []any{
				account1,
				account1,
				staker.MinStake,
			},
			description:  "Decrease stake for a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:    "native_addDelegation",
			expectedGas: 36200,
			args: []any{
				account1,
				staker.MinStake,
				uint8(150),
			},
			description:  "Add delegation to a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:    "native_getDelegation",
			expectedGas: 600,
			args: []any{
				big.NewInt(1), // IDs are incremental, starting at 1
			},
			description:  "Get delegation stake by ID",
			preTestHooks: []TestHook{preTestAddValidation(account1), preTestAddDelegation(account1)},
		},
		{
			function:    "native_withdrawDelegation",
			expectedGas: 11000,
			args: []any{
				big.NewInt(1), // IDs are incremental, starting at 1
			},
			description:  "Withdraw delegation from a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1), preTestAddDelegation(account1)},
		},
		// TODO: How can we mint thousands of blocks and perform housekeeping?
		{
			function:    "native_signalDelegationExit",
			expectedGas: 600,
			args: []any{
				big.NewInt(1), // IDs are incremental, starting at 1
			},
			description:  "Update auto-renew setting for a delegation",
			preTestHooks: []TestHook{preTestAddValidation(account1), preTestAddDelegation(account1)},
			err:          "delegation has not started yet, funds can be withdrawn",
		},
		{
			function:    "native_getDelegatorsRewards",
			expectedGas: 200,
			args: []any{
				account1,
				uint32(0),
			},
			description:  "Get rewards for a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:    "native_getValidationTotals",
			expectedGas: 600,
			args: []any{
				account1,
			},
			description:  "Get total stakes and weights and stake for a validator",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:     "native_getValidationsNum",
			expectedGas:  400,
			description:  "Get number of active and queued validators",
			preTestHooks: []TestHook{preTestAddValidation(account1)},
		},
		{
			function:    "native_issuance",
			expectedGas: 200,
			description: "Get issuance for the current block",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.function, func(t *testing.T) {
			setup := createTestSetup(t)

			for _, hook := range tc.preTestHooks {
				hook(t, setup)
			}

			// Capture the charger using test hook
			var capturedCharger *gascharger.Charger
			gascharger.SetTestHook(func(charger *gascharger.Charger) {
				capturedCharger = charger
			})
			defer gascharger.ClearTestHook()

			// Execute the native function
			result := executeNativeFunction(t, setup, tc.function, tc.args)

			// Validate we captured the charger
			require.NotNil(t, capturedCharger, "Should have captured charger for %s", tc.function)
			gasUsed := capturedCharger.TotalGas()

			// Validate gas usage with descriptive error message
			assert.Equal(t, tc.expectedGas, gasUsed,
				"Gas usage mismatch for %s (%s):\nExpected: %d\nActual: %d\nBreakdown: %s",
				tc.function, tc.description, tc.expectedGas, gasUsed, capturedCharger.Breakdown())

			// Validate function executed successfully (no revert)
			require.NotNil(t, result, "Function %s should return result", tc.function)

			// Check if last element is an error string (staker native functions return error as last element)
			if len(result) > 0 {
				lastElem := result[len(result)-1]
				if errorStr, ok := lastElem.(string); ok {
					assert.Contains(t, errorStr, tc.err)
				}
			}

			// Log detailed breakdown for analysis
			// t.Logf("=== %s ===", tc.function)
			// t.Logf("Description: %s", tc.description)
			// t.Logf("Expected gas: %d", tc.expectedGas)
			// t.Logf("Actual gas used: %d", gasUsed)
			// t.Logf("Result length: %d", len(result))
			// t.Logf("Gas breakdown: %s", capturedCharger.Breakdown())

			// Additional validation: gas should be reasonable (not zero, not excessive)
			assert.Greater(t, gasUsed, uint64(0), "Function %s should consume some gas", tc.function)
			assert.Less(t, gasUsed, uint64(200_000), "Function %s gas usage seems excessive: %d", tc.function, gasUsed)
		})
	}
}

// Helper functions
type testSetup struct {
	chain    *testchain.Chain
	contract *vm.Contract
	evm      *vm.EVM
	state    *state.State
}

// Xenv creates a new builtin environment for each method that has to be called
func (s *testSetup) Xenv(method *abi.Method) *xenv.Environment {
	bestBlock := s.chain.Repo().BestBlockSummary()
	master := genesis.DevAccounts()[0].Address

	tx := new(tx.Builder).
		ChainTag(s.chain.Repo().ChainTag()).
		BlockRef(tx.NewBlockRef(bestBlock.Header.Number())).
		Expiration(32).
		Nonce(datagen.RandUint64()).
		Gas(1000000).
		Clause(tx.NewClause(nil)).
		Build()

	blkContext := &xenv.BlockContext{
		Number:     bestBlock.Header.Number(),
		Time:       bestBlock.Header.Timestamp(),
		GasLimit:   bestBlock.Header.GasLimit(),
		TotalScore: bestBlock.Header.TotalScore(),
		Signer:     master,
	}
	txContext := &xenv.TransactionContext{
		ID:         tx.ID(),
		Origin:     master,
		GasPayer:   master,
		ProvedWork: big.NewInt(1000),
		BlockRef:   tx.BlockRef(),
		Expiration: tx.Expiration(),
	}

	return xenv.New(
		method,
		nil,
		s.state,
		blkContext,
		txContext,
		s.evm,
		s.contract,
		0, // Clause index
	)
}

func executeNativeFunction(t *testing.T, setup *testSetup, functionName string, args []any) (result []any) {
	defer func() {
		if e := recover(); e != nil {
			if revertErr, ok := e.(error); ok {
				result = []any{revertErr.Error()}
			} else {
				panic(e) // re-throw the panic after handling it
			}
		}
	}()

	// Find the native function
	stakerAbi := builtin.Staker.NativeABI()
	method, found := stakerAbi.MethodByName(functionName)
	require.True(t, found, "Function %s not found", functionName)

	data, err := method.EncodeInput(args...)
	require.NoError(t, err, "Failed to encode input for method %s", functionName)
	setup.contract.Input = data

	// Get the native method implementation
	methodID := method.ID()
	nativeMethod, run, found := builtin.FindNativeCall(builtin.Staker.Address, methodID[:])
	require.True(t, found, "Native method not found for %s", functionName)
	require.NotNil(t, nativeMethod, "Native method is nil for %s", functionName)

	// Execute the native function - this will trigger our test hook
	result = run(setup.Xenv(nativeMethod))

	return result
}

func createTestSetup(t *testing.T) *testSetup {
	// Create test chain
	chain, err := testchain.NewDefault()
	require.NoError(t, err)
	bestBlock := chain.Repo().BestBlockSummary()

	// Use proper address generation from dev accounts
	master := genesis.DevAccounts()[0].Address

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

	return &testSetup{
		chain:    chain,
		contract: contract,
		evm:      evm,
		state:    chain.Stater().NewState(bestBlock.Root()),
	}
}
