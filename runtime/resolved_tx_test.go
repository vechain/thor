package runtime_test

import (
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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
	t            *testing.T
	assert       *assert.Assertions
	chain        *chain.Chain
	stateCreator *state.Creator
}

func newTestResolvedTransaction(t *testing.T) (*testResolvedTransaction, error) {
	db, err := lvldb.NewMem()
	if err != nil {
		return nil, err
	}

	gen := genesis.NewDevnet()

	stateCreator := state.NewCreator(db)
	parent, _, err := gen.Build(stateCreator)
	if err != nil {
		return nil, err
	}

	c, err := chain.New(db, parent)
	if err != nil {
		return nil, err
	}

	return &testResolvedTransaction{
		t:            t,
		assert:       assert.New(t),
		chain:        c,
		stateCreator: stateCreator,
	}, nil
}

func (tr *testResolvedTransaction) currentState() (*state.State, error) {
	return tr.stateCreator.NewState(tr.chain.BestBlock().Header().StateRoot())
}

func (tr *testResolvedTransaction) TestResolveTransaction() {

	txBuild := func() *tx.Builder {
		return txBuilder(tr.chain.Tag())
	}

	_, err := runtime.ResolveTransaction(txBuild().Build())
	tr.assert.Equal(secp256k1.ErrInvalidSignatureLen, err)

	_, err = runtime.ResolveTransaction(txSign(txBuild().Gas(21000 - 1)))
	tr.assert.NotNil(err)

	address := thor.BytesToAddress([]byte("addr"))
	_, err = runtime.ResolveTransaction(txSign(txBuild().Clause(tx.NewClause(&address).WithValue(big.NewInt(-10)).WithData(nil))))
	tr.assert.NotNil(err)

	_, err = runtime.ResolveTransaction(txSign(txBuild().
		Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)).
		Clause(tx.NewClause(&address).WithValue(math.MaxBig256).WithData(nil)),
	))
	tr.assert.NotNil(err)

	_, err = runtime.ResolveTransaction(txSign(txBuild()))
	tr.assert.Nil(err)
}

func (tr *testResolvedTransaction) TestCommonTo() {

	txBuild := func() *tx.Builder {
		return txBuilder(tr.chain.Tag())
	}

	commonTo := func(tx *tx.Transaction, assert func(interface{}, ...interface{}) bool) {
		resolve, err := runtime.ResolveTransaction(tx)
		if err != nil {
			tr.t.Fatal(err)
		}
		to := resolve.CommonTo()
		assert(to)
	}

	commonTo(txSign(txBuild()), tr.assert.Nil)

	commonTo(txSign(txBuild().Clause(tx.NewClause(nil))), tr.assert.Nil)

	commonTo(txSign(txBuild().Clause(clause()).Clause(tx.NewClause(nil))), tr.assert.Nil)

	address := thor.BytesToAddress([]byte("addr1"))
	commonTo(txSign(txBuild().
		Clause(clause()).
		Clause(tx.NewClause(&address)),
	), tr.assert.Nil)

	commonTo(txSign(txBuild().Clause(clause())), tr.assert.NotNil)
}

func (tr *testResolvedTransaction) TestBuyGas() {
	state, err := tr.currentState()
	if err != nil {
		tr.t.Fatal(err)
	}

	txBuild := func() *tx.Builder {
		return txBuilder(tr.chain.Tag())
	}

	targetTime := tr.chain.BestBlock().Header().Timestamp() + thor.BlockInterval

	buyGas := func(tx *tx.Transaction) thor.Address {
		resolve, err := runtime.ResolveTransaction(tx)
		if err != nil {
			tr.t.Fatal(err)
		}
		_, _, payer, returnGas, err := resolve.BuyGas(state, targetTime)
		tr.assert.Nil(err)
		returnGas(100)
		return payer
	}

	tr.assert.Equal(
		genesis.DevAccounts()[0].Address,
		buyGas(txSign(txBuild().Clause(clause().WithValue(big.NewInt(100))))),
	)

	bind := builtin.Prototype.Native(state).Bind(genesis.DevAccounts()[1].Address)
	bind.SetCreditPlan(math.MaxBig256, big.NewInt(1000))
	bind.AddUser(genesis.DevAccounts()[0].Address, targetTime)
	tr.assert.Equal(
		genesis.DevAccounts()[1].Address,
		buyGas(txSign(txBuild().Clause(clause().WithValue(big.NewInt(100))))),
	)

	bind.Sponsor(genesis.DevAccounts()[2].Address, true)
	bind.SelectSponsor(genesis.DevAccounts()[2].Address)
	tr.assert.Equal(
		genesis.DevAccounts()[2].Address,
		buyGas(txSign(txBuild().Clause(clause().WithValue(big.NewInt(100))))),
	)
}

func clause() *tx.Clause {
	address := genesis.DevAccounts()[1].Address
	return tx.NewClause(&address).WithData(nil)
}

func txBuilder(tag byte) *tx.Builder {
	return new(tx.Builder).
		GasPriceCoef(1).
		Gas(1000000).
		Expiration(100).
		Nonce(1).
		ChainTag(tag)
}

func txSign(builder *tx.Builder) *tx.Transaction {
	transaction := builder.Build()
	sig, _ := crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	return transaction.WithSignature(sig)
}
