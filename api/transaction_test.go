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
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

var testPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

func TestTransaction(t *testing.T) {

	ntx, ts := addTxToBlock(t)
	raw, err := types.ConvertTransaction(ntx)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	res, err := http.Get(ts.URL + fmt.Sprintf("/transactions/%v", ntx.ID().String()))
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

	key, err := crypto.HexToECDSA(testPrivHex)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(ntx.SigningHash().Bytes(), key)
	if err != nil {
		t.Errorf("Sign error: %s", err)
	}
	rawTransaction := &types.RawTransaction{
		Nonce:     1,
		GasPrice:  big.NewInt(100000),
		Gas:       30000,
		DependsOn: "",
		Sig:       sig,
		BlockRef:  [8]byte(tx.NewBlockRef(20)),
		Clauses: types.Clauses{
			types.Clause{
				To:    thor.BytesToAddress([]byte("acc1")).String(),
				Value: big.NewInt(10000),
				Data:  []byte{0x11, 0x12},
			},
		},
	}
	txData, err := json.Marshal(rawTransaction)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.PostForm(ts.URL+"/transactions", url.Values{"rawTransaction": {string(txData)}})
	if err != nil {
		t.Fatal(err)
	}
	r, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

}

func addTxToBlock(t *testing.T) (*tx.Transaction, *httptest.Server) {
	db, _ := lvldb.NewMem()
	chain := chain.New(db)
	ti := api.NewTransactionInterface(chain, txpool.NewTxPool(chain, txpool.PoolConfig{}))
	router := mux.NewRouter()
	api.NewTransactionHTTPRouter(router, ti)
	ts := httptest.NewServer(router)

	stateC := state.NewCreator(db)

	b, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain.WriteGenesis(b)
	address, _ := thor.ParseAddress(testAddress)
	cla := tx.NewClause(&address).WithValue(big.NewInt(10)).WithData(nil)
	tx := new(tx.Builder).
		GasPrice(big.NewInt(1000)).
		Gas(1000).
		Clause(cla).
		Nonce(1).
		Build()

	key, err := crypto.HexToECDSA(testPrivHex)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(tx.SigningHash().Bytes(), key)
	if err != nil {
		t.Errorf("Sign error: %s", err)
	}
	tx = tx.WithSignature(sig)
	best, _ := chain.GetBestBlock()
	bl := new(block.Builder).
		ParentID(best.Header().ID()).
		Transaction(tx).
		Build()
	stat, err := state.New(bl.Header().StateRoot(), db)
	if err != nil {
		t.Fatal(err)
	}
	stat.SetBalance(thor.BytesToAddress([]byte("acc1")), big.NewInt(10000000000000))
	stat.Stage().Commit()
	if err := chain.AddBlock(bl, true); err != nil {
		t.Fatal(err)
	}

	return tx, ts
}

func checkTx(t *testing.T, expectedTx *types.Transaction, actualTx *types.Transaction) {
	assert.Equal(t, expectedTx.From, actualTx.From)
	assert.Equal(t, expectedTx.ID, actualTx.ID)
	assert.Equal(t, expectedTx.Index, actualTx.Index)
	assert.Equal(t, expectedTx.GasPrice.String(), actualTx.GasPrice.String())
	assert.Equal(t, expectedTx.Gas, actualTx.Gas)
	for i, c := range expectedTx.Clauses {
		assert.Equal(t, string(c.Data), string(actualTx.Clauses[i].Data))
		assert.Equal(t, c.Value.String(), actualTx.Clauses[i].Value.String())
		assert.Equal(t, c.To, actualTx.Clauses[i].To)
	}

}
