// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func TestTxBasics(t *testing.T) {
	fun := []struct {
		getBuilder func() *tx.Builder
	}{
		{
			getBuilder: func() *tx.Builder {
				return txBuilder(0x0, tx.TypeLegacy)
			},
		},
		{
			getBuilder: func() *tx.Builder {
				return txBuilder(0x0, tx.TypeDynamicFee)
			},
		},
	}

	for _, f := range fun {
		trx := f.getBuilder().Build()
		_, err := runtime.ResolveTransaction(trx)
		assert.Equal(t, secp256k1.ErrInvalidSignatureLen.Error(), err.Error())

		trx = f.getBuilder().Gas(21000 - 1).Build()
		_, err = runtime.ResolveTransaction(txSign(trx))
		assert.EqualError(t, err, "intrinsic gas exceeds provided gas")

		address := thor.BytesToAddress([]byte("addr"))
		trx = f.getBuilder().Clause(tx.NewClause(&address).WithValue(big.NewInt(-10)).WithData(nil)).Build()
		_, err = runtime.ResolveTransaction(txSign(trx))
		assert.EqualError(t, err, "clause with negative value")

		trx = f.getBuilder().
			Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
			Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
			Build()
		_, err = runtime.ResolveTransaction(txSign(trx))
		assert.EqualError(t, err, "tx value too large")

		_, err = runtime.ResolveTransaction(txSign(f.getBuilder().Build()))
		assert.Nil(t, err)
	}

	// DynamicFee tx related fields
	trx := txBuilder(0x0, tx.TypeDynamicFee).MaxFeePerGas(big.NewInt(-1)).Build()
	_, err := runtime.ResolveTransaction(txSign(trx))
	assert.EqualError(t, err, "max fee per gas must be positive")

	trx = txBuilder(0x0, tx.TypeDynamicFee).MaxPriorityFeePerGas(big.NewInt(-1)).Build()
	_, err = runtime.ResolveTransaction(txSign(trx))
	assert.EqualError(t, err, "max priority fee per gas must be positive")

	trx = txBuilder(0x0, tx.TypeDynamicFee).MaxFeePerGas(math.BigPow(2, 256)).Build()
	_, err = runtime.ResolveTransaction(txSign(trx))
	assert.EqualError(t, err, "max fee per gas higher than 2^256-1")

	trx = txBuilder(0x0, tx.TypeDynamicFee).MaxPriorityFeePerGas(math.BigPow(2, 256)).Build()
	_, err = runtime.ResolveTransaction(txSign(trx))
	assert.EqualError(t, err, "max priority fee per gas higher than 2^256-1")

	trx = txBuilder(0x0, tx.TypeDynamicFee).MaxPriorityFeePerGas(math.MaxBig256).Build()
	_, err = runtime.ResolveTransaction(txSign(trx))
	assert.EqualError(t, err, "maxFeePerGas is less than maxPriorityFeePerGas")
}

func TestGaspriceLessThanBaseFee(t *testing.T) {
	db := muxdb.NewMem()
	st := state.NewStater(db).NewState(trie.Root{})
	legacyTxBaseGasPrice := big.NewInt(100)
	err := builtin.Params.Native(st).Set(thor.KeyLegacyTxBaseGasPrice, legacyTxBaseGasPrice)
	assert.Nil(t, err)

	trx := txBuilder(0x0, tx.TypeLegacy).GasPriceCoef(0).Build()
	trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)

	obj, err := runtime.ResolveTransaction(trx)
	assert.Nil(t, err)

	_, _, _, _, _, err = obj.BuyGas(st, 0, big.NewInt(101))
	assert.ErrorContains(t, err, "gas price is less than block base fee")

	// can cover the base fee, not return less than base fee error
	_, _, _, _, _, err = obj.BuyGas(st, 0, big.NewInt(100))
	assert.NotNil(t, err)
	assert.NotContains(t, err.Error(), "gas price is less than block base fee")

	trx = txBuilder(0x0, tx.TypeDynamicFee).MaxFeePerGas(big.NewInt(100)).Build()
	trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)

	obj, err = runtime.ResolveTransaction(trx)
	assert.Nil(t, err)

	_, _, _, _, _, err = obj.BuyGas(st, 0, big.NewInt(101))
	assert.ErrorContains(t, err, "gas price is less than block base fee")

	// can cover the base fee, not return less than base fee error
	_, _, _, _, _, err = obj.BuyGas(st, 0, big.NewInt(100))
	assert.NotNil(t, err)
	assert.NotContains(t, err.Error(), "gas price is less than block base fee")
}

func TestResolvedTx(t *testing.T) {
	r, err := newTestResolvedTransaction(t)
	if err != nil {
		t.Fatal(err)
	}

	obValue := reflect.ValueOf(r)
	obType := obValue.Type()
	for i := range obValue.NumMethod() {
		if strings.HasPrefix(obType.Method(i).Name, "Test") {
			obValue.Method(i).Call(nil)
		}
	}
}

type testResolvedTransaction struct {
	t      *testing.T
	assert *assert.Assertions
	repo   *chain.Repository
	stater *state.Stater
}

func newTestResolvedTransaction(t *testing.T) (*testResolvedTransaction, error) {
	db := muxdb.NewMem()

	gen := genesis.NewDevnet()

	stater := state.NewStater(db)
	parent, _, _, err := gen.Build(stater)
	if err != nil {
		return nil, err
	}

	repo, err := chain.NewRepository(db, parent)
	if err != nil {
		return nil, err
	}

	return &testResolvedTransaction{
		t:      t,
		assert: assert.New(t),
		repo:   repo,
		stater: stater,
	}, nil
}

func (tr *testResolvedTransaction) currentState() *state.State {
	h := tr.repo.BestBlockSummary()
	return tr.stater.NewState(h.Root())
}

func (tr *testResolvedTransaction) TestResolveTransaction() {
	fun := []struct {
		getBuilder func() *tx.Builder
	}{
		{
			getBuilder: func() *tx.Builder {
				return txBuilder(tr.repo.ChainTag(), tx.TypeLegacy)
			},
		},
		{
			getBuilder: func() *tx.Builder {
				return txBuilder(tr.repo.ChainTag(), tx.TypeDynamicFee)
			},
		},
	}

	for _, f := range fun {
		trx := f.getBuilder().Build()
		_, err := runtime.ResolveTransaction(trx)
		tr.assert.Equal(secp256k1.ErrInvalidSignatureLen.Error(), err.Error())

		trx = f.getBuilder().Gas(21000 - 1).Build()
		_, err = runtime.ResolveTransaction(txSign(trx))
		tr.assert.NotNil(err)

		address := thor.BytesToAddress([]byte("addr"))
		trx = f.getBuilder().Clause(tx.NewClause(&address).WithValue(big.NewInt(-10)).WithData(nil)).Build()
		_, err = runtime.ResolveTransaction(txSign(trx))
		tr.assert.NotNil(err)

		trx = f.getBuilder().
			Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
			Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
			Build()
		_, err = runtime.ResolveTransaction(txSign(trx))
		tr.assert.NotNil(err)

		_, err = runtime.ResolveTransaction(txSign(f.getBuilder().Build()))
		tr.assert.Nil(err)
	}
}

func (tr *testResolvedTransaction) TestCommonTo() {
	fun := []struct {
		getBuilder func() *tx.Builder
	}{
		{
			getBuilder: func() *tx.Builder {
				return txBuilder(tr.repo.ChainTag(), tx.TypeLegacy)
			},
		},
		{
			getBuilder: func() *tx.Builder {
				return txBuilder(tr.repo.ChainTag(), tx.TypeDynamicFee)
			},
		},
	}

	for _, f := range fun {
		commonTo := func(tx *tx.Transaction, assert func(any, ...any) bool) {
			resolve, err := runtime.ResolveTransaction(tx)
			if err != nil {
				tr.t.Fatal(err)
			}
			to := resolve.CommonTo()
			assert(to)
		}

		legacyTx := f.getBuilder().Build()
		commonTo(txSign(legacyTx), tr.assert.Nil)

		legacyTx = f.getBuilder().Clause(tx.NewClause(nil)).Build()
		commonTo(txSign(legacyTx), tr.assert.Nil)

		legacyTx = f.getBuilder().Clause(clause()).Clause(tx.NewClause(nil)).Build()
		commonTo(txSign(legacyTx), tr.assert.Nil)

		address := thor.BytesToAddress([]byte("addr1"))
		legacyTx = f.getBuilder().
			Clause(clause()).
			Clause(tx.NewClause(&address)).
			Build()
		commonTo(txSign(legacyTx), tr.assert.Nil)

		legacyTx = f.getBuilder().Clause(clause()).Build()
		commonTo(txSign(legacyTx), tr.assert.NotNil)
	}
}

func (tr *testResolvedTransaction) TestBuyGas() {
	state := tr.currentState()

	txBuild := func() *tx.Builder {
		return txBuilder(tr.repo.ChainTag(), tx.TypeLegacy)
	}

	targetTime := tr.repo.BestBlockSummary().Header.Timestamp() + thor.BlockInterval

	buyGas := func(tx *tx.Transaction) thor.Address {
		resolve, err := runtime.ResolveTransaction(tx)
		if err != nil {
			tr.t.Fatal(err)
		}
		_, _, payer, _, returnGas, err := resolve.BuyGas(state, targetTime, nil)
		tr.assert.Nil(err)
		returnGas(100)
		return payer
	}

	tr.assert.Equal(
		genesis.DevAccounts()[0].Address,
		buyGas(txSign(txBuild().Clause(clause().WithValue(big.NewInt(100))).Build())),
	)

	bind := builtin.Prototype.Native(state).Bind(genesis.DevAccounts()[1].Address)
	bind.SetCreditPlan(math.MaxBig256, big.NewInt(1000))
	bind.AddUser(genesis.DevAccounts()[0].Address, targetTime)
	tr.assert.Equal(
		genesis.DevAccounts()[1].Address,
		buyGas(txSign(txBuild().Clause(clause().WithValue(big.NewInt(100))).Build())),
	)

	bind.Sponsor(genesis.DevAccounts()[2].Address, true)
	bind.SelectSponsor(genesis.DevAccounts()[2].Address)
	tr.assert.Equal(
		genesis.DevAccounts()[2].Address,
		buyGas(txSign(txBuild().Clause(clause().WithValue(big.NewInt(100))).Build())),
	)
}

func clause() *tx.Clause {
	address := genesis.DevAccounts()[1].Address
	return tx.NewClause(&address).WithData(nil)
}

func txBuilder(tag byte, txType tx.Type) *tx.Builder {
	return tx.NewBuilder(txType).
		GasPriceCoef(1).
		Gas(1000000).
		Expiration(100).
		Nonce(1).
		ChainTag(tag)
}

func txSign(trx *tx.Transaction) *tx.Transaction {
	return tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
}
