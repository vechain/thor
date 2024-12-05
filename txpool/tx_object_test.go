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
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newChainRepo(db *muxdb.MuxDB) *chain.Repository {
	gene := genesis.NewDevnet()
	b0, _, _, _ := gene.Build(state.NewStater(db))
	repo, _ := chain.NewRepository(db, b0)
	return repo
}

func newTx(txType int, chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, features tx.Features, from genesis.DevAccount) *tx.Transaction {
	var trx *tx.Transaction
	switch txType {
	case tx.LegacyTxType:
		trx = legacyTxBuilder(chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build()
	case tx.DynamicFeeTxType:
		trx = dynFeeTxBuilder(chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build()
	default:
		panic(tx.ErrTxTypeNotSupported)
	}
	return tx.MustSign(trx, from.PrivateKey)
}

func newDelegatedTx(txType int, chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, from genesis.DevAccount, delegator genesis.DevAccount) *tx.Transaction {
	var features tx.Features
	features.SetDelegated(true)

	var trx *tx.Transaction
	switch txType {
	case tx.LegacyTxType:
		trx = legacyTxBuilder(chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build()
	case tx.DynamicFeeTxType:
		trx = dynFeeTxBuilder(chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build()
	default:
		panic(tx.ErrTxTypeNotSupported)
	}

	trx = tx.MustSignDelegated(
		trx,
		from.PrivateKey,
		delegator.PrivateKey,
	)

	return trx
}

func legacyTxBuilder(chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, features tx.Features) *tx.LegacyBuilder {
	builder := new(tx.LegacyBuilder).ChainTag(chainTag)
	for _, c := range clauses {
		builder.Clause(c)
	}

	return builder.BlockRef(blockRef).
		Expiration(expiration).
		Nonce(rand.Uint64()). //#nosec G404
		DependsOn(dependsOn).
		Features(features).
		Gas(gas)
}

func dynFeeTxBuilder(chainTag byte, clauses []*tx.Clause, gas uint64, blockRef tx.BlockRef, expiration uint32, dependsOn *thor.Bytes32, features tx.Features) *tx.DynFeeBuilder {
	builder := new(tx.DynFeeBuilder).ChainTag(chainTag)
	for _, c := range clauses {
		builder.Clause(c)
	}

	return builder.BlockRef(blockRef).
		Expiration(expiration).
		Nonce(rand.Uint64()). //#nosec G404
		DependsOn(dependsOn).
		Features(features).
		Gas(gas)
}

func SetupTest() (genesis.DevAccount, *chain.Repository, *block.Block, *state.State) {
	acc := genesis.DevAccounts()[0]

	db := muxdb.NewMem()
	repo := newChainRepo(db)
	b0 := repo.GenesisBlock()
	b1 := new(block.Builder).ParentID(b0.Header().ID()).GasLimit(10000000).TotalScore(100).Build()
	repo.AddBlock(b1, nil, 0)
	st := state.New(db, repo.GenesisBlock().Header().StateRoot(), 0, 0, 0)

	return acc, repo, b1, st
}

func TestExecutableWithError(t *testing.T) {
	acc, repo, b1, st := SetupTest()

	tests := []struct {
		tx          *tx.Transaction
		expected    bool
		expectedErr string
	}{
		{newTx(tx.LegacyTxType, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), acc), false, ""},
		{newTx(tx.DynamicFeeTxType, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), acc), false, ""},
	}

	for _, tt := range tests {
		txObj, err := resolveTx(tt.tx, false)
		assert.Nil(t, err)

		// pass custom headID
		chain := repo.NewChain(thor.Bytes32{0})

		exe, err := txObj.Executable(chain, st, b1.Header())
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Equal(t, err.Error(), "leveldb: not found")
			assert.Equal(t, tt.expected, exe)
		}
	}
}

func TestSort(t *testing.T) {
	objs := []*txObject{
		{overallGasPrice: big.NewInt(10)},
		{overallGasPrice: big.NewInt(20)},
		{overallGasPrice: big.NewInt(30)},
	}
	sortTxObjsByOverallGasPriceDesc(objs)

	assert.Equal(t, big.NewInt(30), objs[0].overallGasPrice)
	assert.Equal(t, big.NewInt(20), objs[1].overallGasPrice)
	assert.Equal(t, big.NewInt(10), objs[2].overallGasPrice)
}

func TestResolve(t *testing.T) {
	acc := genesis.DevAccounts()[0]
	trx := newTx(tx.LegacyTxType, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	txObj, err := resolveTx(trx, false)
	assert.Nil(t, err)
	assert.Equal(t, trx, txObj.Transaction)

	assert.Equal(t, acc.Address, txObj.Origin())

	trx = newTx(tx.DynamicFeeTxType, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	txObj, err = resolveTx(trx, false)
	assert.Nil(t, err)
	assert.Equal(t, trx, txObj.Transaction)
	assert.Equal(t, acc.Address, txObj.Origin())
}

func TestExecutable(t *testing.T) {
	acc, repo, b1, st := SetupTest()

	tests := []struct {
		tx          *tx.Transaction
		expected    bool
		expectedErr string
	}{
		{newTx(tx.LegacyTxType, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.LegacyTxType, 0, nil, math.MaxUint64, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "gas too large"},
		{newTx(tx.LegacyTxType, 0, nil, 21000, tx.BlockRef{1}, 100, nil, tx.Features(0), acc), true, "block ref out of schedule"},
		{newTx(tx.LegacyTxType, 0, nil, 21000, tx.BlockRef{0}, 0, nil, tx.Features(0), acc), true, "expired"},
		{newTx(tx.LegacyTxType, 0, nil, 21000, tx.BlockRef{0}, 100, &thor.Bytes32{}, tx.Features(0), acc), false, ""},
		{newTx(tx.DynamicFeeTxType, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.DynamicFeeTxType, 0, nil, math.MaxUint64, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "gas too large"},
		{newTx(tx.DynamicFeeTxType, 0, nil, 21000, tx.BlockRef{1}, 100, nil, tx.Features(0), acc), true, "block ref out of schedule"},
		{newTx(tx.DynamicFeeTxType, 0, nil, 21000, tx.BlockRef{0}, 0, nil, tx.Features(0), acc), true, "expired"},
		{newTx(tx.DynamicFeeTxType, 0, nil, 21000, tx.BlockRef{0}, 100, &thor.Bytes32{}, tx.Features(0), acc), false, ""},
	}

	for _, tt := range tests {
		txObj, err := resolveTx(tt.tx, false)
		assert.Nil(t, err)

		exe, err := txObj.Executable(repo.NewChain(b1.Header().ID()), st, b1.Header())
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, exe)
		}
	}
}
