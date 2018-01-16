package api_test

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransaction(t *testing.T) {

	signing, tx, ts := addTxToBlock(t)
	raw := types.ConvertTransactionWithSigning(tx, signing)
	defer ts.Close()

	res, err := http.Get(ts.URL + fmt.Sprintf("/transaction/hash/%v", tx.Hash().String()))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	rtx := new(types.Transaction)
	if err := json.Unmarshal(r, &rtx); err != nil {
		t.Fatal(err)
	}

	checkTx(t, raw, rtx)

	//get transaction from blocknumber with index
	res, err = http.Get(ts.URL + fmt.Sprintf("/transaction/blocknumber/%v/txindex/%v", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	r, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	rt := new(types.Transaction)
	if err := json.Unmarshal(r, &rt); err != nil {
		t.Fatal(err)
	}

	checkTx(t, raw, rt)

}

func addTxToBlock(t *testing.T) (*cry.Signing, *tx.Transaction, *httptest.Server) {
	db, _ := lvldb.NewMem()
	hash, _ := thor.ParseHash(emptyRootHash)
	s, _ := state.New(hash, db)
	chain := chain.New(db)
	ti := api.NewTransactionInterface(chain)
	router := mux.NewRouter()
	api.NewTransactionHTTPRouter(router, ti)
	ts := httptest.NewServer(router)

	b, err := genesis.Build(s)
	if err != nil {
		t.Fatal(err)
	}

	chain.WriteGenesis(b)
	key, _ := crypto.GenerateKey()
	address, _ := thor.ParseAddress(testAddress)
	cla := &tx.Clause{To: &address, Value: big.NewInt(10), Data: nil}
	genesisHash, _ := thor.ParseHash("0x000000006d2958e8510b1503f612894e9223936f1008be2a218c310fa8c11423")
	signing := cry.NewSigning(genesisHash)
	tx := new(tx.Builder).
		GasPrice(big.NewInt(1000)).
		Gas(1000).
		TimeBarrier(10000).
		Clause(cla).
		Nonce(1).
		Build()

	sig, _ := signing.Sign(tx, crypto.FromECDSA(key))
	tx = tx.WithSignature(sig)
	best, _ := chain.GetBestBlock()
	bl := new(block.Builder).
		ParentHash(best.Hash()).
		Transaction(tx).
		Build()
	if err := chain.AddBlock(bl, true); err != nil {
		t.Fatal(err)
	}

	return signing, tx, ts
}

func checkTx(t *testing.T, expectedTx *types.Transaction, actualTx *types.Transaction) {
	assert.Equal(t, expectedTx.From, actualTx.From)
	assert.Equal(t, expectedTx.Hash, actualTx.Hash)
	assert.Equal(t, expectedTx.GasPrice.String(), actualTx.GasPrice.String())
	assert.Equal(t, expectedTx.Gas, actualTx.Gas)
	assert.Equal(t, expectedTx.TimeBarrier, actualTx.TimeBarrier)
	for i, c := range expectedTx.Clauses {
		assert.Equal(t, string(c.Data), string(actualTx.Clauses[i].Data))
		assert.Equal(t, c.Value.String(), actualTx.Clauses[i].Value.String())
		assert.Equal(t, c.To, actualTx.Clauses[i].To)
	}

}
