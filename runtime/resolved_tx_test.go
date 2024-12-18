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
	"github.com/vechain/thor/v2/tx"
)

func TestResolvedTx(t *testing.T) {
	r, err := newTestResolvedTransaction(t)
	if err != nil {
		t.Fatal(err)
	}

	obValue := reflect.ValueOf(r)
	obType := obValue.Type()
	for i := 0; i < obValue.NumMethod(); i++ {
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
	return tr.stater.NewState(h.Header.StateRoot(), h.Header.Number(), 0, h.SteadyNum)
}

func (tr *testResolvedTransaction) TestResolveTransaction() {
	legacyTxBuild := func() *tx.LegacyBuilder {
		return legacyTxBuilder(tr.repo.ChainTag())
	}
	dynFeeTxBuild := func() *tx.DynFeeBuilder {
		return dynFeeTxBuilder(tr.repo.ChainTag())
	}

	_, err := runtime.ResolveTransaction(legacyTxBuild().Build())
	tr.assert.Equal(secp256k1.ErrInvalidSignatureLen.Error(), err.Error())
	_, err = runtime.ResolveTransaction(dynFeeTxBuild().Build())
	tr.assert.Equal(secp256k1.ErrInvalidSignatureLen.Error(), err.Error())

	_, err = runtime.ResolveTransaction(txSign(legacyTxBuild().Gas(21000 - 1).Build()))
	tr.assert.NotNil(err)
	_, err = runtime.ResolveTransaction(txSign(dynFeeTxBuild().Gas(21000 - 1).Build()))
	tr.assert.NotNil(err)

	address := thor.BytesToAddress([]byte("addr"))
	_, err = runtime.ResolveTransaction(txSign(legacyTxBuild().Clause(tx.NewClause(&address).WithValue(big.NewInt(-10)).WithData(nil)).Build()))
	tr.assert.NotNil(err)
	_, err = runtime.ResolveTransaction(txSign(dynFeeTxBuild().Clause(tx.NewClause(&address).WithValue(big.NewInt(-10)).WithData(nil)).Build()))
	tr.assert.NotNil(err)

	_, err = runtime.ResolveTransaction(txSign(legacyTxBuild().
		Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
		Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).Build(),
	))
	tr.assert.NotNil(err)
	_, err = runtime.ResolveTransaction(txSign(dynFeeTxBuild().
		Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
		Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).Build(),
	))
	tr.assert.NotNil(err)

	_, err = runtime.ResolveTransaction(txSign(legacyTxBuild().Build()))
	tr.assert.Nil(err)
	_, err = runtime.ResolveTransaction(txSign(dynFeeTxBuild().Build()))
	tr.assert.Nil(err)
}

func (tr *testResolvedTransaction) TestCommonTo() {
	legacyTxBuild := func() *tx.LegacyBuilder {
		return legacyTxBuilder(tr.repo.ChainTag())
	}
	dynFeeTxBuild := func() *tx.DynFeeBuilder {
		return dynFeeTxBuilder(tr.repo.ChainTag())
	}

	commonTo := func(tx *tx.Transaction, assert func(interface{}, ...interface{}) bool) {
		resolve, err := runtime.ResolveTransaction(tx)
		if err != nil {
			tr.t.Fatal(err)
		}
		to := resolve.CommonTo()
		assert(to)
	}

	commonTo(txSign(legacyTxBuild().Build()), tr.assert.Nil)
	commonTo(txSign(dynFeeTxBuild().Build()), tr.assert.Nil)

	commonTo(txSign(legacyTxBuild().Clause(tx.NewClause(nil)).Build()), tr.assert.Nil)
	commonTo(txSign(dynFeeTxBuild().Clause(tx.NewClause(nil)).Build()), tr.assert.Nil)

	commonTo(txSign(legacyTxBuild().Clause(clause()).Clause(tx.NewClause(nil)).Build()), tr.assert.Nil)
	commonTo(txSign(dynFeeTxBuild().Clause(clause()).Clause(tx.NewClause(nil)).Build()), tr.assert.Nil)

	address := thor.BytesToAddress([]byte("addr1"))
	commonTo(txSign(legacyTxBuild().
		Clause(clause()).
		Clause(tx.NewClause(&address)).Build(),
	), tr.assert.Nil)
	commonTo(txSign(dynFeeTxBuild().
		Clause(clause()).
		Clause(tx.NewClause(&address)).Build(),
	), tr.assert.Nil)

	commonTo(txSign(legacyTxBuild().Clause(clause()).Build()), tr.assert.NotNil)
	commonTo(txSign(dynFeeTxBuild().Clause(clause()).Build()), tr.assert.NotNil)
}

func (tr *testResolvedTransaction) TestBuyGas() {
	state := tr.currentState()

	txBuild := func() *tx.LegacyBuilder {
		return legacyTxBuilder(tr.repo.ChainTag())
	}

	targetTime := tr.repo.BestBlockSummary().Header.Timestamp() + thor.BlockInterval

	buyGas := func(tx *tx.Transaction) thor.Address {
		resolve, err := runtime.ResolveTransaction(tx)
		if err != nil {
			tr.t.Fatal(err)
		}
		_, _, payer, _, returnGas, err := resolve.BuyGas(state, targetTime)
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

func legacyTxBuilder(tag byte) *tx.LegacyBuilder {
	return new(tx.LegacyBuilder).
		GasPriceCoef(1).
		Gas(1000000).
		Expiration(100).
		Nonce(1).
		ChainTag(tag)
}

func dynFeeTxBuilder(tag byte) *tx.DynFeeBuilder {
	return new(tx.DynFeeBuilder).
		MaxFeePerGas(big.NewInt(1)).
		Gas(1000000).
		Expiration(100).
		Nonce(1).
		ChainTag(tag)
}

func txSign(trx *tx.Transaction) *tx.Transaction {
	return tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
}
