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
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func newChainRepo() *chain.Repository {
	tchain, _ := testchain.NewWithFork(thor.SoloFork)
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

func SetupTest() (genesis.DevAccount, *chain.Repository, *block.Block, *state.State, thor.ForkConfig) {
	tchain, _ := testchain.NewWithFork(thor.SoloFork)
	repo := tchain.Repo()
	db := tchain.Database()

	b0 := repo.GenesisBlock()
	b1 := new(block.Builder).ParentID(b0.Header().ID()).GasLimit(10000000).TotalScore(100).Build()
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

	for _, tt := range tests {
		txObj, err := resolveTx(tt.tx, false)
		assert.Nil(t, err)

		// pass custom headID
		chain := repo.NewChain(thor.Bytes32{0})

		exe, err := txObj.Executable(chain, st, b1.Header(), &fc)
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
	sortTxObjsByOverallGasPriceDesc(objs)

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

	tchain, err := testchain.NewWithFork(thor.SoloFork)
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

	for _, tt := range tests {
		txObj, err := resolveTx(tt.tx, false)
		assert.Nil(t, err)

		fc := tchain.GetForkConfig()
		exe, err := txObj.Executable(repo.NewChain(b0.Header().ID()), st, b0.Header(), &fc)
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, exe)
		}
	}
}
