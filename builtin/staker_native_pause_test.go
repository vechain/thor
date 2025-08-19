// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

type pauseTestSetup struct {
	chain  *testchain.Chain
	staker *vm.Contract
	params *vm.Contract
	evm    *vm.EVM
	state  *state.State
}

var (
	MinStake = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	MaxStake = big.NewInt(0).Mul(big.NewInt(600e6), big.NewInt(1e18))
)

// Xenv creates a new builtin environment for each contract and method that has to be called
func (s *pauseTestSetup) Xenv(contract *vm.Contract, method *abi.Method) *xenv.Environment {
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
		contract,
		0, // Clause index
	)
}

func executeParamesNativeMethod(t *testing.T, setup *pauseTestSetup, functionName string, args []any) []any {
	// Find the native function
	abi := builtin.Params.NativeABI()
	method, found := abi.MethodByName(functionName)
	require.True(t, found, "Function %s not found", functionName)

	data, err := method.EncodeInput(args...)
	require.NoError(t, err, "Failed to encode input for method %s", functionName)
	setup.params.Input = data

	// Get the native method implementation
	methodID := method.ID()
	nativeMethod, run, found := builtin.FindNativeCall(builtin.Params.Address, methodID[:])
	require.True(t, found, "Native method not found for %s", functionName)
	require.NotNil(t, nativeMethod, "Native method is nil for %s", functionName)

	// Execute the native function - this will trigger our test hook
	result := run(setup.Xenv(setup.params, nativeMethod))

	return result
}

func executeStakerNativeMethod(t *testing.T, setup *pauseTestSetup, functionName string, args []any) (result []any) {
	defer func() {
		if e := recover(); e != nil {
			// if its an err
			if revertErr, ok := e.(error); ok {
				result = []any{revertErr.Error()}
			} else {
				panic(e) // re-throw the panic after handling it
			}
		}
	}()
	// Find the native function
	abi := builtin.Staker.NativeABI()
	method, found := abi.MethodByName(functionName)
	require.True(t, found, "Function %s not found", functionName)

	data, err := method.EncodeInput(args...)
	require.NoError(t, err, "Failed to encode input for method %s", functionName)
	setup.staker.Input = data

	// Get the native method implementation
	methodID := method.ID()
	nativeMethod, run, found := builtin.FindNativeCall(builtin.Staker.Address, methodID[:])
	require.True(t, found, "Native method not found for %s", functionName)
	require.NotNil(t, nativeMethod, "Native method is nil for %s", functionName)

	// Execute the native function - this will trigger our test hook
	result = run(setup.Xenv(setup.staker, nativeMethod))

	return result
}

func createPauseTestSetup(t *testing.T) *pauseTestSetup {
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

	// Create Staker contract
	newStaker := vm.NewContract(
		vm.AccountRef(master),
		vm.AccountRef(builtin.Staker.Address),
		big.NewInt(0),
		1000000,
	)

	// Create Params contract
	newParams := vm.NewContract(
		vm.AccountRef(master),
		vm.AccountRef(builtin.Params.Address),
		big.NewInt(0),
		1000000,
	)

	return &pauseTestSetup{
		chain:  chain,
		staker: newStaker,
		params: newParams,
		evm:    evm,
		state:  chain.Stater().NewState(bestBlock.Root()),
	}
}

func unpackResult(result []any) ([]any, error) {
	if len(result) > 0 {
		lastElem := result[len(result)-1]
		if errorStr, ok := lastElem.(string); ok {
			if errorStr != "" {
				return result[:len(result)-1], errors.New(errorStr)
			} else {
				// If the last element is an empty string, it means no error
				result = result[:len(result)-1] // Remove the last element (error message)
				return result, nil
			}
		}
	}
	return result, nil
}

func TestIsStargatePaused(t *testing.T) {
	setup := createPauseTestSetup(t)
	charger := gascharger.New(setup.Xenv(setup.params, nil))

	// The KeyStargateSwitches not initialized, so the stargate is not paused
	err := builtin.IsStargatePaused(setup.state, charger)
	require.NoError(t, err, "Function IsStargatePaused should not return error %s", err)

	// Set Stargate pause active
	result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(3)}) // Set the first bit to 1
	_, err = unpackResult(result)
	require.NoError(t, err, "Function native_set should not return error %s", err)

	err = builtin.IsStargatePaused(setup.state, charger)
	require.ErrorContains(t, err, "stargate is paused", "Function IsStargatePaused should return error")

	// Set Stargate pause inactive
	result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // Set the first bit to 0
	_, err = unpackResult(result)
	require.NoError(t, err, "Function native_set should not return error %s", err)

	err = builtin.IsStargatePaused(setup.state, charger)
	require.NoError(t, err, "Function IsStargatePaused should not return error %s", err)
}

func TestIsStakerPaused(t *testing.T) {
	setup := createPauseTestSetup(t)
	charger := gascharger.New(setup.Xenv(setup.params, nil))

	// The KeyStargateSwitches not initialized, so the Staker is not paused
	err := builtin.IsStakerPaused(setup.state, charger)
	require.NoError(t, err, "Function IsStakerPaused should not return error %s", err)

	// Set Staker pause active
	result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // Set the second bit to 1
	_, err = unpackResult(result)
	require.NoError(t, err, "Function native_set should not return error %s", err)

	err = builtin.IsStakerPaused(setup.state, charger)
	require.ErrorContains(t, err, "staker is paused", "Function IsStakerPaused should return error")

	// Set Staker pause inactive
	result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(4)}) // Set the second bit to 0
	_, err = unpackResult(result)
	require.NoError(t, err, "Function native_set should not return error %s", err)

	err = builtin.IsStakerPaused(setup.state, charger)
	require.NoError(t, err, "Function IsStakerPaused should not return error %s", err)

	setup.state.SetRawStorage(builtin.Params.Address, thor.KeyStargateSwitches, rlp.RawValue{0xFF})

	err = builtin.IsStakerPaused(setup.state, charger)
	require.Error(t, err)

	err = builtin.IsStargatePaused(setup.state, charger)
	require.Error(t, err)
}

func TestAddAndExitValidatorForPause(t *testing.T) {
	setup := createPauseTestSetup(t)

	newValidator := genesis.DevAccounts()[1].Address

	// Add newValidator as a validator1
	_, err := builtin.Authority.Native(setup.state).Add(newValidator, newValidator, thor.Bytes32{})
	require.NoError(t, err, "Function Add should not return error %s", err)

	// Set Staker pause active, so the validator could not be added
	t.Run("Step1", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // Set the second bit to 1
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_addValidation", []any{newValidator, newValidator, uint32(360) * 24 * 15, MinStake})
		require.NotNil(t, result, "Function native_addValidation should return result")
		datas, err := unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Staker pause inactive, so the validator could be added
	t.Run("Step2", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // Set the second bit to 0
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_addValidation", []any{newValidator, newValidator, uint32(360) * 24 * 15, MinStake})
		require.NotNil(t, result, "Function native_addValidation should return result")
		datas, err := unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
		require.NoError(t, err, "Function native_set should not return error %s", err)
	})

	// Set Staker pause active, so the validator could not be exited
	t.Run("Step3", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // Set the second bit to 1
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_signalExit", []any{newValidator, newValidator})
		require.NotNil(t, result, "Function native_signalExit should return result")
		datas, err := unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_signalExit not run datas")
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Staker pause inactive, so the validator could be exited
	t.Run("Step4", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // Set the second bit to 0
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_signalExit", []any{newValidator, newValidator})
		require.NotNil(t, result, "Function native_signalExit should return result")
		_, err := unpackResult(result)
		if err != nil {
			assert.False(t, strings.Contains(err.Error(), "staker is paused"))
		}
	})
}

func TestIncreaseAndDecreaseStakeForPause(t *testing.T) {
	setup := createPauseTestSetup(t)
	newValidator := genesis.DevAccounts()[0].Address

	// add new validator
	result := executeStakerNativeMethod(t, setup, "native_addValidation", []any{newValidator, newValidator, uint32(360) * 24 * 15, MinStake})
	require.NotNil(t, result, "Function native_addValidation should return result")
	datas, err := unpackResult(result)
	require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
	require.NoError(t, err, "Function native_addValidation should not return error %s", err)

	// Set Staker pause active, so the validator could not to increased stake
	t.Run("Step1", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // Set the second bit to 1
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_increaseStake", []any{newValidator, newValidator, big.NewInt(500)})
		require.NotNil(t, result, "Function native_increaseStake should return result")
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Staker pause inactive， so the validator could to increased stake
	t.Run("Step2", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // Set the second bit to 0
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		// Increase stake
		result = executeStakerNativeMethod(t, setup, "native_increaseStake", []any{newValidator, newValidator, big.NewInt(500)})
		require.NotNil(t, result, "Function native_increaseStake should return result")
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
		require.NoError(t, err, "Function native_increaseStake should not return error %s", err)

		// Decrease stake
		result = executeStakerNativeMethod(t, setup, "native_decreaseStake", []any{newValidator, newValidator, big.NewInt(100)})
		require.NotNil(t, result, "Function native_decreaseStake should return result")
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_decreaseStake not run datas")
		require.NoError(t, err, "Function native_decreaseStake should not return error %s", err)
	})

	// Set Staker pause inactive， so the validator could not to decrease stake
	t.Run("Step3", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // Set the second bit to 1
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_decreaseStake", []any{newValidator, newValidator, big.NewInt(100)})
		require.NotNil(t, result, "Function native_decreaseStake should return result")
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
		require.ErrorContains(t, err, "staker is paused")
	})
}

func TestWithdrawStakeForPause(t *testing.T) {
	setup := createPauseTestSetup(t)
	newValidator := genesis.DevAccounts()[0].Address

	// add new validator
	result := executeStakerNativeMethod(t, setup, "native_addValidation", []any{newValidator, newValidator, uint32(360) * 24 * 15, MinStake})
	require.NotNil(t, result, "Function native_addValidation should return result")
	datas, err := unpackResult(result)
	require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
	require.NoError(t, err, "Function native_addValidation should not return error %s", err)

	// Set Staker pause active, so the validator could not to withdrawn
	t.Run("Step1", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // Set the second bit to 1
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_withdrawStake", []any{newValidator, newValidator})
		require.NotNil(t, result, "Function native_withdrawStake should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Staker pause inactive, so the validator could to withdrawn
	t.Run("Step2", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(1)}) // Set the second bit to 1
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_withdrawStake", []any{newValidator, newValidator})
		require.NotNil(t, result, "Function native_withdrawStake should return result")
		datas, err = unpackResult(result)
		require.NoError(t, err, "Function native_withdrawStake should not return error %s", err)
		require.Len(t, datas, 1)
		require.IsType(t, datas[0], &big.Int{}, "Function native_withdrawStake will return a big.Int data")
		require.Equal(t, datas[0].(*big.Int), MinStake)
	})
}

func TestDelegationAddAndExitForPause(t *testing.T) {
	setup := createPauseTestSetup(t)
	newValidator := genesis.DevAccounts()[0].Address
	delegationID := big.NewInt(0)

	// add new validator
	result := executeStakerNativeMethod(t, setup, "native_addValidation", []any{newValidator, newValidator, uint32(360) * 24 * 15, MinStake})
	require.NotNil(t, result, "Function native_addValidation should return result")
	datas, err := unpackResult(result)
	require.True(t, len(datas) == 0, "Function native_addValidation not run datas")
	require.NoError(t, err, "Function native_addValidation should not return error %s", err)

	// Set Stargate pause active and Staker pause inactive, so the delegator could not be added
	t.Run("Step1", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(1)}) // (binary: 0 1)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_addDelegation", []any{newValidator, big.NewInt(100), uint8(1)})
		require.NotNil(t, result, "Function native_addDelegation should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "stargate is paused")
	})

	// Set Stargate pause inactive and Staker pause active, so the delegator could not be added
	t.Run("Step2", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // (binary: 1 0)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_addDelegation", []any{newValidator, big.NewInt(100), uint8(1)})
		require.NotNil(t, result, "Function native_addDelegation should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Stargate pause and Staker pause both active, so the delegator could not be added
	t.Run("Step3", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(3)}) // (binary: 1 1)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_addDelegation", []any{newValidator, big.NewInt(100), uint8(1)})
		require.NotNil(t, result, "Function native_addDelegation should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "stargate is paused")
	})

	// Set Stargate pause and Staker pause both inactive, so the delegator could be added
	t.Run("Step4", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // (binary: 0 0)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_addDelegation", []any{newValidator, big.NewInt(100), uint8(1)})
		require.NotNil(t, result, "Function native_addDelegation should return result")
		datas, err = unpackResult(result)
		require.NoError(t, err, "Function native_addDelegation should not return error %s", err)
		require.True(t, len(datas) == 1, " Function native_addDelegation will run a data")
		require.IsType(t, datas[0], &big.Int{}, "Function native_addDelegation will return a big.Int data")
		require.NotNil(t, datas[0].(*big.Int))
		delegationID = datas[0].(*big.Int)
	})

	// Set Stargate pause active and Staker pause inactive, so the delegator could not be exited
	t.Run("Step5", func(t *testing.T) {
		result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(1)}) // (binary: 0 1)
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_signalDelegationExit", []any{delegationID})
		require.NotNil(t, result, "Function native_signalDelegationExit should return result")
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0)
		require.ErrorContains(t, err, "stargate is paused")
	})

	// Set Stargate pause inactive and Staker pause active, so the delegator could not be exited
	t.Run("Step6", func(t *testing.T) {
		result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // (binary: 1 0)
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_signalDelegationExit", []any{delegationID})
		require.NotNil(t, result)
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0)
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Stargate pause and Staker pause both active, so the delegator could not be exited
	t.Run("Step7", func(t *testing.T) {
		result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(3)}) // (binary: 1 1)
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_signalDelegationExit", []any{delegationID})
		require.NotNil(t, result)
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0)
		require.ErrorContains(t, err, "stargate is paused")
	})

	// Set Stargate pause and Staker pause both inactive, so the delegator could be exited
	t.Run("Step8", func(t *testing.T) {
		result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // (binary: 0 0)
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_signalDelegationExit", []any{delegationID})
		require.NotNil(t, result)
		datas, err = unpackResult(result)
		require.True(t, len(datas) == 0)
		if err != nil {
			assert.False(t, strings.Contains(err.Error(), "staker is paused"))
		}
	})
}

func TestWithdrawDelegationPause(t *testing.T) {
	setup := createPauseTestSetup(t)
	newValidator := genesis.DevAccounts()[0].Address
	stakeValue := big.NewInt(1000)

	// add new validator
	result := executeStakerNativeMethod(t, setup, "native_addValidation", []any{newValidator, newValidator, uint32(360) * 24 * 15, MinStake})
	require.NotNil(t, result, "Function native_addValidation should return result")
	_, err := unpackResult(result)
	require.NoError(t, err, "Function native_addValidation should not return error %s", err)

	// add delegation
	result = executeStakerNativeMethod(t, setup, "native_addDelegation", []any{newValidator, stakeValue, uint8(1)})
	require.NotNil(t, result, "Function native_addDelegation should return result")
	datas, err := unpackResult(result)
	require.NoError(t, err, "Function native_addDelegation should not return error %s", err)
	require.True(t, len(datas) == 1)
	require.IsType(t, datas[0], &big.Int{})
	delegationID := datas[0].(*big.Int)
	require.NotNil(t, delegationID)

	// Set Stargate pause active and Staker pause inactive, so the delegator could not to withdrawn
	t.Run("Step1", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(1)}) // (binary: 0 1)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_withdrawDelegation", []any{delegationID})
		require.NotNil(t, result, "Function native_withdrawDelegation should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "stargate is paused")
	})

	// Set Stargate pause inactive and Staker pause active, so the delegator could not to withdrawn
	t.Run("Step2", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(2)}) // (binary: 1 0)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_withdrawDelegation", []any{delegationID})
		require.NotNil(t, result, "Function native_withdrawDelegation should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "staker is paused")
	})

	// Set Stargate pause and Staker pause both active, so the delegator could not to withdrawn
	t.Run("Step3", func(t *testing.T) {
		result := executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(3)}) // (binary: 1 1)
		_, err := unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result = executeStakerNativeMethod(t, setup, "native_withdrawDelegation", []any{delegationID})
		require.NotNil(t, result, "Function native_withdrawDelegation should return result")
		datas, err = unpackResult(result)
		require.Len(t, datas, 0)
		require.ErrorContains(t, err, "stargate is paused")
	})

	// Set Stargate pause and Staker pause both inactive, so the delegator could be withdrawn
	t.Run("Step4", func(t *testing.T) {
		result = executeParamesNativeMethod(t, setup, "native_set", []any{thor.KeyStargateSwitches, big.NewInt(0)}) // (binary: 0 0)
		_, err = unpackResult(result)
		require.NoError(t, err, "Function native_set should not return error %s", err)

		result := executeStakerNativeMethod(t, setup, "native_withdrawDelegation", []any{delegationID})
		require.NotNil(t, result)
		datas, err := unpackResult(result)
		require.NoError(t, err, "Function native_withdrawDelegation should not return error %s", err)
		require.True(t, len(datas) == 1)
		require.IsType(t, datas[0], &big.Int{})
		require.Equal(t, datas[0].(*big.Int), stakeValue)
	})
}
