// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	_ "github.com/vechain/thor/v2/tracers/native"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

var thorChain *testchain.Chain

type TestTxDescription struct {
	t          *testing.T
	abi        *abi.ABI
	methodName string
	address    thor.Address
	acc        genesis.DevAccount
	args       []any
	duplicate  bool
	vet        *big.Int
}

func inspectClauseWithBlockRef(clause *tx.Clause, blockRef *tx.BlockRef) ([]byte, uint64, error) {
	builder := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(42000).
		Clause(clause)

	if blockRef != nil {
		builder.BlockRef(*blockRef)
	}

	trx := builder.Build()
	return thorChain.ClauseCall(genesis.DevAccounts()[0], trx, 0)
}

func getClause(abi *abi.ABI, methodName string, address thor.Address, args ...any) (*tx.Clause, *abi.Method, error) {
	m, ok := abi.MethodByName(methodName)
	if !ok {
		return nil, nil, fmt.Errorf("method %s not found", methodName)
	}
	input, err := m.EncodeInput(args...)
	return tx.NewClause(&address).WithData(input), m, err
}

func callContractAndGetOutput(abi *abi.ABI, methodName string, address thor.Address, output any, args ...any) (uint64, error) {
	clause, m, err := getClause(abi, methodName, address, args...)
	if err != nil {
		return 0, err
	}
	decoded, gasUsed, err := inspectClauseWithBlockRef(clause, nil)
	if err != nil {
		return 0, err
	}
	return gasUsed, m.DecodeOutput(decoded, output)
}

func executeTxAndGetReceipt(description TestTxDescription) (*tx.Receipt, *thor.Bytes32, error) {
	m, ok := description.abi.MethodByName(description.methodName)
	if !ok {
		return nil, nil, fmt.Errorf("method %s not found", description.methodName)
	}
	input, err := m.EncodeInput(description.args...)
	if err != nil {
		return nil, nil, err
	}

	clause := tx.NewClause(&description.address).WithData(input)
	if description.vet != nil {
		clause = clause.WithValue(description.vet)
	}

	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(1000000).
		Clause(clause).
		Build()

	if description.duplicate {
		trx = new(tx.Builder).
			ChainTag(thorChain.Repo().ChainTag()).
			Expiration(50).
			Gas(100000).
			Clause(clause).
			Nonce(2).
			Build()
	}

	trx = tx.MustSign(trx, description.acc.PrivateKey)
	err = thorChain.MintTransactions(description.acc, trx)
	if err != nil {
		return nil, nil, err
	}

	id := trx.ID()
	fetchedTx, err := thorChain.GetTxReceipt(id)

	return fetchedTx, &id, err
}

func TestParamsNative(t *testing.T) {
	thorChain, _ = testchain.NewDefault()

	toAddr := builtin.Params.Address
	abi := builtin.Params.ABI

	var addr common.Address
	_, err := callContractAndGetOutput(abi, "executor", toAddr, &addr)

	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	key := thor.BytesToBytes32([]byte("key"))
	value := big.NewInt(999)
	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "set",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{key, value},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Params.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, key, fetchedTx.Outputs[0].Events[0].Topics[1])

	require.NoError(t, err)
	require.Equal(t, value, big.NewInt(0).SetBytes(fetchedTx.Outputs[0].Events[0].Data))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "set",
		address:    toAddr,
		acc:        genesis.DevAccounts()[1],
		args:       []any{key, value},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.True(t, fetchedTx.Reverted)

	var decodedVal *big.Int
	_, err = callContractAndGetOutput(abi, "get", toAddr, &decodedVal, key)
	require.NoError(t, err)
	require.Equal(t, value, decodedVal)
}

func TestAuthorityNative(t *testing.T) {
	thorChain, _ = testchain.NewDefault()
	var (
		master1   = genesis.DevAccounts()[1]
		endorsor1 = genesis.DevAccounts()[2]
		identity1 = genesis.DevAccounts()[3]

		master2   = genesis.DevAccounts()[4]
		endorsor2 = genesis.DevAccounts()[5]
		identity2 = genesis.DevAccounts()[6]

		master3   = genesis.DevAccounts()[7]
		endorsor3 = genesis.DevAccounts()[8]
		identity3 = genesis.DevAccounts()[9]
	)
	toAddr := builtin.Authority.Address

	abi := builtin.Authority.ABI

	var addr common.Address
	_, err := callContractAndGetOutput(abi, "first", toAddr, &addr)

	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	var b32 [32]uint8
	copy(b32[12:], identity1.Address.Bytes())
	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master1.Address, endorsor1.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	added := "added"
	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	copy(b32[12:], identity2.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master2.Address, endorsor2.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	copy(b32[12:], identity3.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master3.Address, endorsor3.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master3.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	_, err = callContractAndGetOutput(abi, "first", toAddr, &addr)
	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, genesis.DevAccounts()[0].Address)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, master1.Address)
	require.NoError(t, err)
	require.Equal(t, master2.Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, master2.Address)
	require.NoError(t, err)
	require.Equal(t, master3.Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, master3.Address)
	require.NoError(t, err)
	require.Equal(t, thor.Address{}.Bytes(), addr.Bytes())

	copy(b32[12:], identity1.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        master2,
		args:       []any{master1.Address, endorsor1.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        master1,
		args:       []any{master1.Address, endorsor1.Address, b32},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "revoke",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master1.Address},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	revoked := "revoked"
	require.NoError(t, err)
	require.Equal(t, revoked, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "revoke",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master1.Address},
		duplicate:  true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	f := new(big.Float).SetFloat64(9.976e26)
	i := new(big.Int)
	f.Int(i)

	clause := tx.NewClause(&endorsor3.Address).WithValue(i)
	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(100000).
		Clause(clause).
		Nonce(2).
		Build()

	trx = tx.MustSign(trx, endorsor2.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(endorsor2, trx))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "revoke",
		address:    toAddr,
		acc:        master3,
		args:       []any{master2.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, revoked, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))
}

func TestEnergyNative(t *testing.T) {
	var (
		acc1 = genesis.DevAccounts()[0]
		acc2 = genesis.DevAccounts()[1]
		acc3 = thor.BytesToAddress([]byte("some acc"))
		acc4 = thor.BytesToAddress([]byte("stargate address"))
	)

	toAddr := builtin.Energy.Address

	abi := builtin.Energy.ABI

	fc := &thor.SoloFork
	fc.HAYABUSA = 4
	thorChain, _ = testchain.NewWithFork(fc, 1)

	var stringOutput string
	_, err := callContractAndGetOutput(abi, "name", toAddr, &stringOutput)

	veThor := "VeThor"
	require.NoError(t, err)
	require.Equal(t, veThor, stringOutput)

	var uint8Output uint8
	_, err = callContractAndGetOutput(abi, "decimals", toAddr, &uint8Output)

	exDecimals := uint8(18)
	require.NoError(t, err)
	require.Equal(t, exDecimals, uint8Output)

	_, err = callContractAndGetOutput(abi, "symbol", toAddr, &stringOutput)

	exSymbol := "VTHO"
	require.NoError(t, err)
	require.Equal(t, exSymbol, stringOutput)

	var bigIntOutput *big.Int
	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)

	exSupply := new(big.Int)
	exSupply.SetString("10000000000000000000000000000", 10)
	require.NoError(t, err)
	require.Equal(t, exSupply, bigIntOutput)

	_, err = callContractAndGetOutput(abi, "totalBurned", toAddr, &bigIntOutput)

	exBurned := big.Int{}
	require.NoError(t, err)
	require.Equal(t, exBurned.String(), bigIntOutput.String())

	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transfer",
		address:    toAddr,
		acc:        acc1,
		args:       []any{acc3, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc3, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	f := new(big.Float).SetFloat64(1e30)
	i := new(big.Int)
	f.Int(i)
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transfer",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc3, i},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "move",
		address:    toAddr,
		acc:        acc1,
		args:       []any{acc1.Address, acc3, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc3, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "move",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc1.Address, acc3, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "approve",
		address:    toAddr,
		acc:        acc1,
		args:       []any{acc2.Address, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	_, err = callContractAndGetOutput(abi, "allowance", toAddr, &bigIntOutput, acc1.Address, acc2.Address)

	exAllowance := big.NewInt(1001)
	require.NoError(t, err)
	require.Equal(t, exAllowance, bigIntOutput)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transferFrom",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc1.Address, acc3, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc3, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transferFrom",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc1.Address, acc3, big.NewInt(1000)},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)
	require.NoError(t, err)
	best := thorChain.Repo().BestBlockSummary().Header.Number()

	growth := new(big.Int)
	growth.SetUint64(thor.BlockInterval())
	growth.Mul(growth, exSupply)
	growth.Mul(growth, thor.EnergyGrowthRate)
	growth.Div(growth, big.NewInt(1e18))

	hayabusa := thorChain.GetForkConfig().HAYABUSA
	for i := uint32(0); i < best && i < hayabusa; i++ {
		exSupply = exSupply.Add(exSupply, growth)
	}
	require.Equal(t, exSupply, bigIntOutput)

	//_________________________________________________________
	// -- START SETUP HAYABUSA FORK AND TRANSITION TO POS --
	//---------------------------------------------------------

	// 1: Set MaxBlockProposers to 1
	params := thorChain.Contract(builtin.Params.Address, builtin.Params.ABI, genesis.DevAccounts()[0])
	staker := thorChain.Contract(builtin.Staker.Address, builtin.Staker.ABI, genesis.DevAccounts()[0])

	shouldCollectEnergy := func() {
		if thorChain.Repo().BestBlockSummary().Header.Number() <= hayabusa {
			exSupply = exSupply.Add(exSupply, growth)
		}
	}

	assert.NoError(t, params.MintTransaction("set", big.NewInt(0), thor.KeyMaxBlockProposers, big.NewInt(1)))
	shouldCollectEnergy()
	assert.NoError(t, params.MintTransaction("set", big.NewInt(0), thor.KeyDelegatorContractAddress, big.NewInt(0).SetBytes(acc4.Bytes())))
	shouldCollectEnergy()

	// 2: Add a validator to the queue
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	assert.NoError(t, staker.MintTransaction("addValidation", minStake, acc1.Address, uint32(360)*24*7))
	shouldCollectEnergy()

	validatorMap := make(map[uint64]*big.Int)

	// 3: Mint some blocks
	summary := thorChain.Repo().BestBlockSummary()
	firstPOS := summary.Header.Number() + 1
	st := thorChain.Stater().NewState(summary.Root())
	err = builtin.Params.Native(st).Set(thor.KeyCurveFactor, thor.InitialCurveFactor)
	assert.NoError(t, err)

	energyAtBlock, err := builtin.Energy.Native(st, summary.Header.Timestamp()).Get(summary.Header.Beneficiary())
	require.NoError(t, err)
	validatorMap[summary.Header.Timestamp()] = energyAtBlock
	require.NoError(t, err)

	var totalSupplyBefore *big.Int
	var totalSupplyAfter *big.Int

	for range 5 {
		_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &totalSupplyBefore)
		require.NoError(t, err)
		require.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))
		summary = thorChain.Repo().BestBlockSummary()
		st := thorChain.Stater().NewState(summary.Root())
		energyAtBlock, err = builtin.Energy.Native(st, summary.Header.Timestamp()).Get(summary.Header.Beneficiary())
		require.NoError(t, err)
		validatorMap[thorChain.Repo().BestBlockSummary().Header.Timestamp()] = energyAtBlock

		_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &totalSupplyAfter)
		require.NoError(t, err)

		validatorEnergyBefore := validatorMap[thorChain.Repo().BestBlockSummary().Header.Timestamp()-10]

		// check after POS that sum of all rewards for block is equal to total supply growth
		rewards := big.NewInt(0).Sub(energyAtBlock, validatorEnergyBefore)
		growth = big.NewInt(0).Sub(totalSupplyAfter, totalSupplyBefore)
		assert.Equal(t, rewards, growth)
	}
	best = thorChain.Repo().BestBlockSummary().Header.Number()

	stakeRewards := big.NewInt(0)
	for i := firstPOS; i <= best; i++ {
		summary, err := thorChain.Repo().NewBestChain().GetBlockSummary(i)
		require.NoError(t, err)
		st = thorChain.Stater().NewState(summary.Root())
		staker := builtin.Staker.Native(st)
		energy := builtin.Energy.Native(st, summary.Header.Timestamp())
		reward, err := energy.CalculateRewards(staker)
		require.NoError(t, err)
		stakeRewards.Add(stakeRewards, reward)

		energyAtBlock := validatorMap[summary.Header.Timestamp()]
		energyBeforeBlock := validatorMap[summary.Header.Timestamp()-10]

		// there are no delegators, so the validator gets the whole reward
		expectedReward := new(big.Int).Set(reward)
		require.True(t, expectedReward.Cmp(big.NewInt(0).Sub(energyAtBlock, energyBeforeBlock)) == 0)
	}
	exSupply = exSupply.Add(exSupply, stakeRewards)

	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)
	require.NoError(t, err)
	require.Equal(t, exSupply, bigIntOutput)

	firstActiveRes := new(common.Address)
	_, err = callContractAndGetOutput(builtin.Staker.ABI, "firstActive", builtin.Staker.Address, firstActiveRes)
	assert.NoError(t, err)
}

func TestPrototypeNative(t *testing.T) {
	var (
		acc1         = thor.BytesToAddress([]byte("acc1"))
		master1      = genesis.DevAccounts()[0]
		master2      = genesis.DevAccounts()[1]
		sponsor      = genesis.DevAccounts()[2]
		credit       = big.NewInt(1000)
		recoveryRate = big.NewInt(10)
	)

	thorChain, _ = testchain.NewDefault()
	abi := builtin.Prototype.ABI
	toAddr := builtin.Prototype.Address

	code, _ := hex.DecodeString(
		"60606040523415600e57600080fd5b603580601b6000396000f3006060604052600080fd00a165627a7a72305820edd8a93b651b5aac38098767f0537d9b25433278c9d155da2135efc06927fc960029",
	)
	clause := tx.NewClause(nil).WithData(code)
	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(100000).
		Clause(clause).
		Build()

	trx = tx.MustSign(trx, master1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(master1, trx))

	id := trx.ID()
	fetchedTx, err := thorChain.GetTxReceipt(id)
	assert.NoError(t, err)
	contractAddr := fetchedTx.Outputs[0].Events[0].Address

	var outputAddr common.Address
	_, err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, acc1)

	exMaster := thor.Address{}
	require.NoError(t, err)
	require.Equal(t, exMaster.Bytes(), outputAddr.Bytes())

	_, err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setMaster",
		address:    toAddr,
		acc:        master2,
		args:       []any{master2.Address, master1.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, master2.Address, fetchedTx.Outputs[0].Events[0].Address)

	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setMaster",
		address:    toAddr,
		acc:        master2,
		args:       []any{master1.Address, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, master2.Address)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), outputAddr.Bytes())

	var outputBool bool
	_, err = callContractAndGetOutput(abi, "hasCode", toAddr, &outputBool, master1.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	_, err = callContractAndGetOutput(abi, "hasCode", toAddr, &outputBool, contractAddr)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setCreditPlan",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, credit, recoveryRate},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)

	decodedVal := fetchedTx.Outputs[0].Events[0].Data

	require.NoError(t, err)
	require.Equal(t, credit.Bytes(), bytes.Trim(decodedVal[:32], "\x00"))
	require.Equal(t, recoveryRate.Bytes(), bytes.Trim(decodedVal[32:], "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setCreditPlan",
		address:    toAddr,
		acc:        master2,
		args:       []any{contractAddr, credit, recoveryRate},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	type CreditPlan struct {
		Credit       *big.Int
		RecoveryRate *big.Int
	}
	plan := &CreditPlan{}
	_, err = callContractAndGetOutput(abi, "creditPlan", toAddr, plan, contractAddr)

	require.NoError(t, err)
	require.Equal(t, credit.String(), plan.Credit.String())
	require.Equal(t, recoveryRate, plan.RecoveryRate)

	_, err = callContractAndGetOutput(abi, "isUser", toAddr, &outputBool, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	added := "added"
	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master2,
		args:       []any{contractAddr, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, acc1},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "isUser", toAddr, &outputBool, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	var outputBigInt *big.Int
	_, err = callContractAndGetOutput(abi, "userCredit", toAddr, &outputBigInt, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, credit, outputBigInt)

	_, err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "sponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, sponsor.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	sponsored := "sponsored"
	require.NoError(t, err)
	require.Equal(t, sponsored, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "sponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	_, err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, common.Address{}, outputAddr)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, sponsor.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, sponsor.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	selected := "selected"
	require.NoError(t, err)
	require.Equal(t, selected, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master2,
		args:       []any{contractAddr, sponsor.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, sponsor.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "unsponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, sponsor.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "unsponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	storageKey := thor.Blake2b(contractAddr.Bytes(), []byte("credit-plan"))
	ch := thorChain.Repo().NewBestChain()
	summary, err := thorChain.Repo().GetBlockSummary(ch.HeadID())
	assert.NoError(t, err)
	st := state.New(thorChain.Database(), trie.Root{Hash: summary.Header.StateRoot(), Ver: trie.Version{Major: summary.Header.Number()}})
	storageValDecoded, err := st.GetStorage(toAddr, storageKey)
	assert.NoError(t, err)

	var uint8Array [32]uint8
	_, err = callContractAndGetOutput(abi, "storageFor", toAddr, &uint8Array, builtin.Prototype.Address, thor.Blake2b([]byte("credit-plan")))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])

	_, err = callContractAndGetOutput(
		abi,
		"storageFor",
		toAddr,
		&uint8Array,
		builtin.Prototype.Address,
		thor.Blake2b(contractAddr.Bytes(), []byte("credit-plan")),
	)

	require.NoError(t, err)
	require.Equal(t, storageValDecoded.Bytes(), uint8Array[:])

	var outputInt *big.Int
	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputInt, acc1, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputInt.String())

	clause = tx.NewClause(&acc1).WithValue(big.NewInt(1001))
	trx = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Build()

	trx = tx.MustSign(trx, master1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(master1, trx))
	bestBlock, err := thorChain.BestBlock()
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputInt, acc1, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001), outputInt)

	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputInt, acc1, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputInt.String())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []any{acc1, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputInt, acc1, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000), outputInt)
}

func TestPrototypeNativeWithLongerBlockNumber(t *testing.T) {
	var (
		acc1   = genesis.DevAccounts()[1]
		acc2   = thor.BytesToAddress([]byte("acc2"))
		toAddr = builtin.Prototype.Address
		abi    = builtin.Prototype.ABI
	)
	thorChain, _ = testchain.NewDefault()

	var outputBigInt *big.Int
	_, err := callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputBigInt.String())

	clause := tx.NewClause(&acc2).WithValue(big.NewInt(1))
	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Build()

	trx = tx.MustSign(trx, acc1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(acc1, trx))
	bestBlock, err := thorChain.BestBlock()
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), outputBigInt)

	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputBigInt.String())

	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        acc1,
		args:       []any{acc2, big.NewInt(1)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), outputBigInt)

	clause = tx.NewClause(&acc2).WithValue(big.NewInt(1))
	trx = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Nonce(2).
		Build()

	trx = tx.MustSign(trx, acc1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(acc1, trx))
	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), outputBigInt)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        acc1,
		args:       []any{acc2, big.NewInt(1)},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), outputBigInt)
}

func TestExtensionNative(t *testing.T) {
	thorChain, _ = testchain.NewDefault()

	master1 := genesis.DevAccounts()[0]
	master2 := genesis.DevAccounts()[1]
	b0 := thorChain.GenesisBlock()
	abi := builtin.Extension.V2.ABI
	toAddr := builtin.Extension.Address

	var uint8Array [32]uint8
	_, err := callContractAndGetOutput(abi, "blake2b256", toAddr, &uint8Array, []byte("hello world"))

	require.NoError(t, err)
	require.Equal(t, thor.Blake2b([]byte("hello world")).Bytes(), uint8Array[:])

	var expected *big.Int
	var bigIntOutput *big.Int
	_, err = callContractAndGetOutput(builtin.Energy.ABI, "totalSupply", builtin.Energy.Address, &expected)
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)

	require.NoError(t, err)
	require.Equal(t, expected, bigIntOutput)

	m, _ := abi.MethodByName("txBlockRef")
	input, err := m.EncodeInput()
	assert.NoError(t, err)
	clause := tx.NewClause(&toAddr).WithData(input)
	blkRef := tx.NewBlockRef(1)

	outBytes, _, err := inspectClauseWithBlockRef(clause, &blkRef)

	require.NoError(t, err)
	require.Equal(t, tx.NewBlockRef(1).Number(), binary.BigEndian.Uint32(outBytes))

	_, err = callContractAndGetOutput(abi, "txExpiration", toAddr, &bigIntOutput)

	require.NoError(t, err)
	require.Equal(t, big.NewInt(50), bigIntOutput)

	m, _ = abi.MethodByName("txProvedWork")
	input, err = m.EncodeInput()
	assert.NoError(t, err)
	clause = tx.NewClause(&toAddr).WithData(input)

	builder := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(42000).
		Clause(clause)

	trx := builder.Build()
	outBytes, _, err = thorChain.ClauseCall(master1, trx, 0)
	assert.NoError(t, err)

	block, err := thorChain.BestBlock()
	assert.NoError(t, err)

	getBlockID := func(_ uint32) (thor.Bytes32, error) {
		return thor.Bytes32{}, nil
	}
	provedWork, err := trx.ProvedWork(block.Header().Number(), getBlockID)

	require.NoError(t, err)
	require.Equal(t, provedWork.String(), new(big.Int).SetBytes(outBytes).String())

	m, _ = abi.MethodByName("txID")
	input, err = m.EncodeInput()
	assert.NoError(t, err)
	clause = tx.NewClause(&toAddr).WithData(input)

	builder = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(42000).
		Clause(clause)

	trx = builder.Build()
	trxID, _, err := thorChain.ClauseCall(master1, trx, 0)
	assert.NoError(t, err)

	require.NoError(t, err)
	require.Equal(t, trx.ID().Bytes(), trxID)

	fetchedTx, id, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []any{master2.Address, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	fetchedTx2, id2, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []any{master2.Address, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx2.Outputs))
	require.False(t, fetchedTx2.Reverted)

	gasUsed, err := callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(3))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])
	assert.Equal(t, uint64(570), gasUsed)

	gasUsed, err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(2))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])
	assert.Equal(t, uint64(570), gasUsed)

	gasUsed, err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(1))

	require.NoError(t, err)
	assert.Equal(t, uint64(770), gasUsed)
	bl, err := thorChain.GetTxBlock(id)
	require.NoError(t, err)
	require.Equal(t, bl.Header().ID().Bytes(), uint8Array[:])

	gasUsed, err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(0))
	require.NoError(t, err)
	assert.Equal(t, uint64(770), gasUsed)
	require.Equal(t, b0.Header().ID().Bytes(), uint8Array[:])

	var uint64Output uint64
	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(3))
	require.NoError(t, err)
	assert.Equal(t, uint64(454), gasUsed)
	require.Equal(t, uint64(0), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(2))
	assert.NoError(t, err)
	assert.Equal(t, uint64(454), gasUsed)
	bl2, err := thorChain.GetTxBlock(id2)
	require.NoError(t, err)
	block2, err := thorChain.Repo().GetBlock(bl2.Header().ID())

	require.NoError(t, err)
	require.Equal(t, block2.Header().TotalScore(), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(1))
	assert.Equal(t, uint64(854), gasUsed)
	assert.NoError(t, err)
	block1, err := thorChain.Repo().GetBlock(bl.Header().ID())

	require.NoError(t, err)
	require.Equal(t, block1.Header().TotalScore(), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(0))

	require.NoError(t, err)
	assert.Equal(t, uint64(854), gasUsed)
	require.Equal(t, b0.Header().TotalScore(), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(3))

	require.NoError(t, err)
	assert.Equal(t, uint64(404), gasUsed)
	require.Equal(t, big.NewInt(0).String(), bigIntOutput.String())

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(2))

	require.NoError(t, err)
	assert.Equal(t, uint64(404), gasUsed)
	require.Equal(t, new(big.Int).SetUint64(block2.Header().Timestamp()), bigIntOutput)

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(1))

	require.NoError(t, err)
	assert.Equal(t, uint64(804), gasUsed)
	require.Equal(t, new(big.Int).SetUint64(block1.Header().Timestamp()), bigIntOutput)

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(0))

	require.NoError(t, err)
	assert.Equal(t, uint64(804), gasUsed)
	require.Equal(t, new(big.Int).SetUint64(b0.Header().Timestamp()), bigIntOutput)

	var addressOutput common.Address
	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(3))

	require.NoError(t, err)
	assert.Equal(t, uint64(432), gasUsed)
	require.Equal(t, common.Address{}, addressOutput)

	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(2))

	require.NoError(t, err)
	assert.Equal(t, uint64(432), gasUsed)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())

	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(1))

	require.NoError(t, err)
	assert.Equal(t, uint64(832), gasUsed)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())

	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(0))

	require.NoError(t, err)
	assert.Equal(t, uint64(832), gasUsed)
	require.Equal(t, common.Address{}, addressOutput)

	gasUsed, err = callContractAndGetOutput(abi, "txGasPayer", toAddr, &addressOutput)

	require.NoError(t, err)
	assert.Equal(t, uint64(372), gasUsed)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())
}

func TestStakerContract_Native(t *testing.T) {
	fc := &thor.SoloFork
	fc.HAYABUSA = 2
	hayabusaTP := uint32(2)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	var err error
	thorChain, err = testchain.NewWithFork(fc, 1)
	assert.NoError(t, err)

	// add validator as authority
	endorsor := genesis.DevAccounts()[0] // dev acc, since it has to be in PoA
	master := genesis.DevAccounts()[8]
	var b32 [32]uint8
	copy(b32[12:], genesis.DevAccounts()[3].Address.Bytes())
	receipt, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Authority.ABI,
		methodName: "add",
		address:    builtin.Authority.Address,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master.Address, endorsor.Address, b32},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	energyAbi := builtin.Energy.ABI
	energyAddress := builtin.Energy.Address

	mbp := TestTxDescription{
		t:          t,
		abi:        builtin.Params.ABI,
		methodName: "set",
		address:    builtin.Params.Address,
		acc:        genesis.DevAccounts()[0],
		args:       []any{thor.KeyMaxBlockProposers, big.NewInt(1)},
	}
	receipt, a, err := executeTxAndGetReceipt(mbp) // mint block 1
	assert.NoError(t, err)
	assert.NotNil(t, receipt)
	assert.NotNil(t, a)

	abi := builtin.Staker.ABI
	toAddr := builtin.Staker.Address

	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 2: hayabusa should fork here and set the contract bytecode

	totalNumRes := make([]any, 2)
	totalNumRes[0] = new(uint64)
	totalNumRes[1] = new(uint64)
	_, err = callContractAndGetOutput(abi, "getValidationsNum", toAddr, &totalNumRes)
	assert.NoError(t, err, "dsada")

	totalBurnedBefore := new(big.Int)
	_, err = callContractAndGetOutput(energyAbi, "totalBurned", energyAddress, &totalBurnedBefore)
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "getValidationsNum", toAddr, &totalNumRes)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), *(totalNumRes[0].(*uint64)))
	assert.Equal(t, uint64(0), *(totalNumRes[1].(*uint64)))

	// parameters
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	minStakingPeriod := uint32(360) * 24 * 15

	// addValidation
	addValidationArgs := []any{master.Address, minStakingPeriod}
	desc := TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addValidation",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       addValidationArgs,
		vet:        minStake,
	}
	receipt, trxid, err := executeTxAndGetReceipt(desc) // mint block 3
	assert.NoError(t, err)
	assert.False(t, receipt.Reverted)
	assert.Equal(t, master.Address, thor.BytesToAddress(receipt.Outputs[0].Events[0].Topics[1][:]))
	block, err := thorChain.GetTxBlock(trxid)
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "getValidationsNum", toAddr, &totalNumRes)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), *(totalNumRes[0].(*uint64)))
	assert.Equal(t, uint64(1), *(totalNumRes[1].(*uint64)))

	totalBurned := new(big.Int)
	_, err = callContractAndGetOutput(energyAbi, "totalBurned", energyAddress, &totalBurned)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(big.NewInt(0).SetUint64(receipt.GasUsed), block.Header().BaseFee()), big.NewInt(0).Sub(totalBurned, totalBurnedBefore))

	// setBeneficiary
	beneficiary := genesis.DevAccounts()[9].Address
	setBeneficiaryArgs := []any{master.Address, beneficiary}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setBeneficiary",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       setBeneficiaryArgs,
	}
	receipt, trxid, err = executeTxAndGetReceipt(desc) // mint block 4
	assert.NoError(t, err)
	assert.False(t, receipt.Reverted)
	assert.Equal(t, master.Address, thor.BytesToAddress(receipt.Outputs[0].Events[0].Topics[1][:]))
	_, err = thorChain.GetTxBlock(trxid)
	assert.NoError(t, err)

	node := master.Address
	// getStake
	getStakeRes := make([]any, 6)
	getStakeRes[0] = new(common.Address)
	getStakeRes[1] = new(*big.Int)
	getStakeRes[2] = new(*big.Int)
	getStakeRes[3] = new(*big.Int)
	getStakeRes[4] = new(uint8)
	getStakeRes[5] = new(uint32)

	_, err = callContractAndGetOutput(abi, "getValidation", toAddr, &getStakeRes, node)
	assert.NoError(t, err)

	expectedEndorsor := common.BytesToAddress(endorsor.Address.Bytes())
	assert.Equal(t, &expectedEndorsor, getStakeRes[0])
	assert.Equal(t, big.NewInt(0).Cmp(*getStakeRes[1].(**big.Int)), 0) // stake - should be 0 while queued
	assert.Equal(t, big.NewInt(0).Cmp(*getStakeRes[2].(**big.Int)), 0) // weight - should be 0 while queued
	assert.Equal(t, minStake.Cmp(*getStakeRes[3].(**big.Int)), 0)      // queue stake
	assert.Equal(t, validation.StatusQueued, *getStakeRes[4].(*uint8))
	assert.Equal(t, uint32(math.MaxUint32), *getStakeRes[5].(*uint32)) // last offline block

	// getPeriod
	getPeriodRes := make([]any, 4)
	getPeriodRes[0] = new(uint32)
	getPeriodRes[1] = new(uint32)
	getPeriodRes[2] = new(uint32)
	getPeriodRes[3] = new(uint32)
	_, err = callContractAndGetOutput(abi, "getValidationPeriodDetails", toAddr, &getPeriodRes, node)
	assert.NoError(t, err)
	assert.Equal(t, uint32(360*24*15), *getPeriodRes[0].(*uint32))
	assert.Equal(t, uint32(0), *getPeriodRes[1].(*uint32))              // start period
	assert.Equal(t, uint32(math.MaxUint32), *getPeriodRes[2].(*uint32)) // exit block
	assert.Equal(t, uint32(0), *getPeriodRes[3].(*uint32))              // completed periods

	// firstQueued
	firstQueuedRes := new(common.Address)
	_, err = callContractAndGetOutput(abi, "firstQueued", toAddr, firstQueuedRes)
	assert.NoError(t, err)
	assert.Equal(t, common.Address(master.Address), *firstQueuedRes)

	// queuedStake
	queuedStakeRes := new(*big.Int)
	_, err = callContractAndGetOutput(abi, "queuedStake", toAddr, &queuedStakeRes)
	assert.NoError(t, err)
	expectedTotalStake := big.NewInt(25_000_000)
	expectedTotalStake = big.NewInt(0).Mul(expectedTotalStake, big.NewInt(1e18))
	assert.Equal(t, expectedTotalStake, *queuedStakeRes)

	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 4: Transition period block
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 5: PoS should become active and active the queued validators

	// firstActive
	firstActiveRes := new(common.Address)
	_, err = callContractAndGetOutput(abi, "firstActive", toAddr, firstActiveRes)
	assert.NoError(t, err)
	assert.Equal(t, common.Address(master.Address), *firstActiveRes)

	// totalStake
	totalStakeRes := make([]any, 2)
	totalStakeRes[0] = new(*big.Int)
	totalStakeRes[1] = new(*big.Int)
	_, err = callContractAndGetOutput(abi, "totalStake", toAddr, &totalStakeRes)
	assert.NoError(t, err)
	expectedTotalStake = big.NewInt(25_000_000)
	expectedTotalStake = expectedTotalStake.Mul(expectedTotalStake, big.NewInt(1e18))
	assert.Equal(t, expectedTotalStake, *totalStakeRes[0].(**big.Int))
	assert.Equal(t, expectedTotalStake, *totalStakeRes[1].(**big.Int))

	// queuedStake
	queuedStakeRes = new(*big.Int)
	_, err = callContractAndGetOutput(abi, "queuedStake", toAddr, &queuedStakeRes)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Int64(), (*queuedStakeRes).Int64())

	_, err = callContractAndGetOutput(abi, "getValidationsNum", toAddr, &totalNumRes)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), *(totalNumRes[0].(*uint64)))
	assert.Equal(t, uint64(0), *(totalNumRes[1].(*uint64)))

	reward := new(*big.Int)
	_, err = callContractAndGetOutput(abi, "getDelegatorsRewards", toAddr, reward, node, uint32(1))
	assert.NoError(t, err)
	assert.Equal(t, new(big.Int).String(), (*reward).String())

	// GetValidatorsTotals
	getValidatorsTotals := make([]any, 5)
	getValidatorsTotals[0] = new(*big.Int)
	getValidatorsTotals[1] = new(*big.Int)
	getValidatorsTotals[2] = new(*big.Int)
	getValidatorsTotals[3] = new(*big.Int)
	getValidatorsTotals[4] = new(*big.Int)

	_, err = callContractAndGetOutput(abi, "getValidationTotals", toAddr, &getValidatorsTotals, common.Address(master.Address))
	assert.NoError(t, err)
	assert.Equal(t, minStake, *getValidatorsTotals[0].(**big.Int))
	assert.Equal(t, minStake, *getValidatorsTotals[1].(**big.Int))
	assert.Equal(t, big.NewInt(0).String(), (*getValidatorsTotals[2].(**big.Int)).String())
	assert.Equal(t, big.NewInt(0).String(), (*getValidatorsTotals[3].(**big.Int)).String())
	assert.Equal(t, minStake, *getValidatorsTotals[4].(**big.Int))
}

func TestStakerContract_Native_Revert(t *testing.T) {
	fc := &thor.SoloFork
	fc.HAYABUSA = 2
	hayabusaTP := uint32(2)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	var err error
	thorChain, err = testchain.NewWithFork(fc, 180)
	assert.NoError(t, err)

	mbp := TestTxDescription{
		t:          t,
		abi:        builtin.Params.ABI,
		methodName: "set",
		address:    builtin.Params.Address,
		acc:        genesis.DevAccounts()[0],
		args:       []any{thor.KeyMaxBlockProposers, big.NewInt(1)},
	}
	receipt, a, err := executeTxAndGetReceipt(mbp) // mint block 1
	assert.NoError(t, err)
	assert.NotNil(t, receipt)
	assert.NotNil(t, a)

	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 2: hayabusa should fork here and set the contract bytecode

	abi := builtin.Staker.ABI
	toAddr := builtin.Staker.Address
	endorsor := genesis.DevAccounts()[0]
	master := genesis.DevAccounts()[8]

	// parameters
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	minStakingPeriod := uint32(360) * 24 * 15

	// addValidator
	addValidatorArgs := []any{master.Address, minStakingPeriod + 1}
	desc := TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addValidation",
		address:    toAddr,
		acc:        endorsor,
		args:       addValidatorArgs,
		vet:        minStake,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// update auto renew
	signalExitArgs := []any{thor.Bytes32{}}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "signalExit",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       signalExitArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// increase stake
	increaseStakeArgs := []any{thor.Bytes32{}}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "increaseStake",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       increaseStakeArgs,
		vet:        big.NewInt(1),
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// decrease stake
	decreaseStakeArgs := []any{thor.Bytes32{}, big.NewInt(1)}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "decreaseStake",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       decreaseStakeArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// update delegator auto renew
	updateDelegatorAutoRenewArgs := []any{big.NewInt(0)}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "signalDelegationExit",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       updateDelegatorAutoRenewArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// addDelegation
	addDelegationArgs := []any{thor.Bytes32{}, uint8(1)}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addDelegation",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       addDelegationArgs,
		vet:        big.NewInt(10),
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// withdrawDelegation
	withdrawDelegationArgs := []any{big.NewInt(0)}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "withdrawDelegation",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       withdrawDelegationArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)
}

func TestStakerContract_Native_WithdrawQueued(t *testing.T) {
	fc := &thor.SoloFork
	fc.HAYABUSA = 1
	hayabusaTP := uint32(2)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	var err error
	thorChain, err = testchain.NewWithFork(fc, 180)
	assert.NoError(t, err)
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	abi := builtin.Staker.ABI
	toAddr := builtin.Staker.Address
	endorsor := genesis.DevAccounts()[0]
	master := genesis.DevAccounts()[0]

	// parameters
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	minStakingPeriod := uint32(360) * 24 * 15

	// addValidator
	addValidatorArgs := []any{master.Address, minStakingPeriod}
	desc := TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addValidation",
		address:    toAddr,
		acc:        endorsor,
		args:       addValidatorArgs,
		vet:        minStake,
	}
	receipt, _, err := executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	id := receipt.Outputs[0].Events[0].Topics[2]

	// withdraw queued
	withdrawArgs := []any{id}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "withdrawStake",
		address:    toAddr,
		acc:        endorsor,
		args:       withdrawArgs,
		vet:        big.NewInt(0),
	}
	_, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	// getValidation
	getRes := make([]any, 6)
	getRes[0] = new(common.Address)
	getRes[1] = new(*big.Int)
	getRes[2] = new(*big.Int)
	getRes[3] = new(*big.Int)
	getRes[4] = new(uint8)
	getRes[5] = new(uint32)
	_, err = callContractAndGetOutput(abi, "getValidation", toAddr, &getRes, id)
	assert.NoError(t, err)
	assert.Equal(t, validation.StatusExit, *getRes[4].(*uint8))

	// firstQueued
	firstQueuedRes := new(common.Address)
	_, err = callContractAndGetOutput(abi, "firstQueued", toAddr, firstQueuedRes)
	assert.NoError(t, err)
	expectedMaster := common.Address{}
	assert.Equal(t, &expectedMaster, firstQueuedRes)
}

func TestExtensionV3(t *testing.T) {
	fc := thor.SoloFork
	chain, err := testchain.NewWithFork(&fc, 180)
	assert.Nil(t, err)

	// galactica fork happens at block 1
	assert.NoError(t, chain.MintBlock(genesis.DevAccounts()[0]))
	assert.NoError(t, chain.MintBlock(genesis.DevAccounts()[0]))

	// setup txClauseIndex call data
	txClauseIndexABI, ok := builtin.Extension.V3.ABI.MethodByName("txClauseIndex")
	assert.True(t, ok)
	txClauseIndex, err := txClauseIndexABI.EncodeInput()
	assert.Nil(t, err)
	txClauseIndexClause := tx.NewClause(&builtin.Extension.Address).WithData(txClauseIndex)

	// setup txClauseCount call data
	txClauseCountABI, ok := builtin.Extension.V3.ABI.MethodByName("txClauseCount")
	assert.True(t, ok)
	txClauseCount, err := txClauseCountABI.EncodeInput()
	assert.Nil(t, err)
	txClauseCountClause := tx.NewClause(&builtin.Extension.Address).WithData(txClauseCount)

	// init the runtime
	best := chain.Repo().BestBlockSummary()
	rtChain := chain.Repo().NewChain(best.Header.ParentID())
	rtStater := chain.Stater().NewState(best.Root())
	rt := runtime.New(rtChain, rtStater, &xenv.BlockContext{Number: best.Header.Number(), Time: best.Header.Timestamp(), TotalScore: 1}, &thor.ForkConfig{})

	// test txClauseIndex
	clauseIndex := uint32(934)
	exec, _ := rt.PrepareClause(txClauseIndexClause, clauseIndex, math.MaxUint64, &xenv.TransactionContext{})
	out, _, err := exec()
	assert.Nil(t, err)
	val := new(big.Int).SetBytes(out.Data)
	assert.Equal(t, uint64(clauseIndex), val.Uint64())

	// test txClauseCount
	clauseCount := uint32(712)
	exec, _ = rt.PrepareClause(txClauseCountClause, 0, math.MaxUint64, &xenv.TransactionContext{
		ClauseCount: clauseCount,
	})
	out, _, err = exec()
	assert.Nil(t, err)
	val = new(big.Int).SetBytes(out.Data)
	assert.Equal(t, uint64(clauseCount), val.Uint64())
}
