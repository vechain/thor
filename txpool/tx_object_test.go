// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func newChainRepo() *chain.Repository {
	tchain, _ := testchain.NewWithFork(&thor.SoloFork)
	return tchain.Repo()
}

func newTx(txType tx.Type, chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, features tx.Features, from genesis.DevAccount) *tx.Transaction {
	trx := txBuilder(txType, chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build()
	return tx.MustSign(trx, from.PrivateKey)
}

func newDelegatedTx(txType tx.Type, chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, from genesis.DevAccount, delegator genesis.DevAccount) *tx.Transaction {
	var features tx.Features
	features.SetDelegated(true)

	return tx.MustSignDelegated(
		txBuilder(txType, chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build(),
		from.PrivateKey,
		delegator.PrivateKey,
	)
}

func txBuilder(txType tx.Type, chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, features tx.Features) *tx.Builder {
	builder := tx.NewBuilder(txType).ChainTag(chainTag)
	for _, c := range clauses {
		builder.Clause(c)
	}

	return builder.BlockRef(blockRef).
		Expiration(expiration).
		Nonce(rand.Uint64()). //#nosec G404
		DependsOn(dependsOn).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxPriorityFeePerGas(big.NewInt(10000)).
		Features(features).
		Gas(gas)
}

func SetupTest() (genesis.DevAccount, *chain.Repository, *block.Block, *state.State, *thor.ForkConfig) {
	tchain, _ := testchain.NewWithFork(&thor.SoloFork)
	repo := tchain.Repo()
	db := tchain.Database()

	b0 := repo.GenesisBlock()
	b1 := new(block.Builder).ParentID(b0.Header().ID()).GasLimit(10000000).TotalScore(100).BaseFee(big.NewInt(thor.InitialBaseFee)).Build()
	repo.AddBlock(b1, nil, 0, false)
	st := state.New(db, trie.Root{Hash: repo.GenesisBlock().Header().StateRoot()})

	return genesis.DevAccounts()[0], tchain.Repo(), b1, st, tchain.GetForkConfig()
}

func TestExecutableWithError(t *testing.T) {
	acc, repo, b1, st, fc := SetupTest()

	tests := []struct {
		tx          *tx.Transaction
		expected    bool
		expectedErr string
	}{
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), acc), false, ""},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), acc), false, ""},
	}

	baseFee := galactica.CalcBaseFee(b1.Header(), fc)
	for _, tt := range tests {
		txObj, err := resolveTx(tt.tx, false)
		assert.Nil(t, err)

		// pass custom headID
		chain := repo.NewChain(thor.Bytes32{0})

		exe, err := txObj.Executable(chain, st, b1.Header(), fc, baseFee)
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Equal(t, err.Error(), "leveldb: not found")
			assert.Equal(t, tt.expected, exe)
		}
	}
}

func TestSort(t *testing.T) {
	addr1 := thor.BytesToAddress([]byte("addr1"))
	addr2 := thor.BytesToAddress([]byte("addr2"))
	objs := []*txObject{
		{priorityGasPrice: big.NewInt(0)},
		{priorityGasPrice: big.NewInt(10), timeAdded: 20, payer: &addr1},
		{priorityGasPrice: big.NewInt(10), timeAdded: 3, payer: &addr2},
		{priorityGasPrice: big.NewInt(20)},
		{priorityGasPrice: big.NewInt(30)},
	}
	sortTxObjsByPriorityGasPriceDesc(objs)

	assert.Equal(t, big.NewInt(30), objs[0].priorityGasPrice)
	assert.Equal(t, big.NewInt(20), objs[1].priorityGasPrice)
	assert.Equal(t, big.NewInt(10), objs[2].priorityGasPrice)
	assert.Equal(t, int64(20), objs[2].timeAdded)
	assert.Equal(t, &addr1, objs[2].payer)
	assert.Equal(t, big.NewInt(10), objs[3].priorityGasPrice)
	assert.Equal(t, int64(3), objs[3].timeAdded)
	assert.Equal(t, &addr2, objs[3].payer)
	assert.Equal(t, big.NewInt(0), objs[4].priorityGasPrice)
}

func TestResolve(t *testing.T) {
	acc := genesis.DevAccounts()[0]
	trx := newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	txObj, err := resolveTx(trx, false)
	assert.Nil(t, err)
	assert.Equal(t, trx, txObj.Transaction)

	assert.Equal(t, acc.Address, txObj.Origin())

	trx = newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	txObj, err = resolveTx(trx, false)
	assert.Nil(t, err)
	assert.Equal(t, trx, txObj.Transaction)
	assert.Equal(t, acc.Address, txObj.Origin())
}

func TestExecutable(t *testing.T) {
	acc := genesis.DevAccounts()[0]

	tchain, err := testchain.NewWithFork(&thor.SoloFork)
	assert.Nil(t, err)
	repo := tchain.Repo()
	db := tchain.Database()

	b0 := repo.GenesisBlock()
	st := state.New(db, trie.Root{Hash: repo.GenesisBlock().Header().StateRoot()})

	tests := []struct {
		tx          *tx.Transaction
		expected    bool
		expectedErr string
	}{
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.TypeLegacy, 0, nil, b0.Header().GasLimit(), tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.TypeLegacy, 0, nil, b0.Header().GasLimit()+1, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "gas too large"},
		{newTx(tx.TypeLegacy, 0, nil, math.MaxUint64, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "gas too large"},
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{1}, 100, nil, tx.Features(0), acc), true, "block ref out of schedule"},
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 0, nil, tx.Features(0), acc), true, "expired"},
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, &thor.Bytes32{}, tx.Features(0), acc), false, ""},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.TypeDynamicFee, 0, nil, math.MaxUint64, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "gas too large"},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{1}, 100, nil, tx.Features(0), acc), true, "block ref out of schedule"},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 0, nil, tx.Features(0), acc), true, "expired"},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 100, &thor.Bytes32{}, tx.Features(0), acc), false, ""},
	}

	baseFee := galactica.CalcBaseFee(b0.Header(), tchain.GetForkConfig())
	for _, tt := range tests {
		txObj, err := resolveTx(tt.tx, false)
		assert.Nil(t, err)

		exe, err := txObj.Executable(repo.NewChain(b0.Header().ID()), st, b0.Header(), tchain.GetForkConfig(), baseFee)
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, exe)
		}
	}
}

func TestExecutableRejectNonLegacyBeforeGalactica(t *testing.T) {
	forkConfig := &thor.ForkConfig{
		GALACTICA:   2,
		HAYABUSA_TP: math.MaxUint32,
		HAYABUSA:    math.MaxUint32,
	}

	dynamicFeeTx := newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])

	tchain, _ := testchain.NewWithFork(forkConfig)
	repo := tchain.Repo()
	baseFee := galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig)

	txObj1, err := resolveTx(dynamicFeeTx, false)
	assert.Nil(t, err)

	st := tchain.Stater().NewState(repo.BestBlockSummary().Root())
	exe, err := txObj1.Executable(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, baseFee)
	assert.False(t, exe)
	assert.Equal(t, tx.ErrTxTypeNotSupported, err)

	legacyTx := newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	txObj2, err := resolveTx(legacyTx, false)
	assert.Nil(t, err)

	exe, err = txObj2.Executable(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, baseFee)
	assert.True(t, exe)
	assert.Nil(t, err)

	// add a block 1
	tchain.MintBlock(genesis.DevAccounts()[0])

	// recalculate the base fee since new block is added
	baseFee = galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig)
	st = tchain.Stater().NewState(repo.BestBlockSummary().Root())
	_, err = txObj1.Executable(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, baseFee)
	assert.Nil(t, err)
}

func TestExecutableRejectUnsupportedFeatures(t *testing.T) {
	forkConfig := &thor.ForkConfig{
		VIP191:      2,
		HAYABUSA:    math.MaxUint32,
		HAYABUSA_TP: math.MaxUint32,
	}

	tchain, _ := testchain.NewWithFork(forkConfig)
	repo := tchain.Repo()

	tx1 := newDelegatedTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, nil, genesis.DevAccounts()[0], genesis.DevAccounts()[1])
	txObj1, err := resolveTx(tx1, false)
	assert.Nil(t, err)

	st := tchain.Stater().NewState(repo.BestBlockSummary().Root())
	exe, err := txObj1.Executable(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig))
	assert.False(t, exe)
	assert.ErrorContains(t, err, "unsupported features")

	// add a block 1
	tchain.MintBlock(genesis.DevAccounts()[0])

	st = tchain.Stater().NewState(repo.BestBlockSummary().Root())
	_, err = txObj1.Executable(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig))
	assert.Nil(t, err)
}
