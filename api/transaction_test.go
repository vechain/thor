package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common/math"
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

var testPrivHex = "efa321f290811731036e5eccd373114e5186d9fe419081f5a607231279d5ef01"

func TestTransaction(t *testing.T) {

	ntx, ts := initTransactionServer(t)
	raw, err := types.ConvertTransaction(ntx)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	r, err := httpGet(ts, ts.URL+fmt.Sprintf("/transactions/%v", ntx.ID().String()))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("tx", string(r))
	rtx := new(types.Transaction)
	if err := json.Unmarshal(r, &rtx); err != nil {
		t.Fatal(err)
	}
	checkTx(t, raw, rtx)

	r, err = httpGet(ts, ts.URL+fmt.Sprintf("/transactions/%v/receipts", ntx.ID().String()))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("receipts", string(r))
	// receipt := new(tx.Receipt)
	// if err := json.Unmarshal(r, &receipt); err != nil {
	// 	t.Fatal(err)
	// }

	key, err := crypto.HexToECDSA(testPrivHex)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(ntx.SigningHash().Bytes(), key)

	if err != nil {
		t.Errorf("Sign error: %s", err)
	}
	to := thor.BytesToAddress([]byte("acc1"))
	hash := thor.BytesToHash([]byte("DependsOn"))
	v := big.NewInt(10000)
	m := math.HexOrDecimal256(*v)
	rawTransaction := &types.RawTransaction{
		Nonce:        1,
		GasPriceCoef: 1,
		Gas:          30000,
		DependsOn:    &hash,
		Sig:          sig,
		BlockRef:     [8]byte(tx.NewBlockRef(20)),
		Clauses: types.Clauses{
			types.Clause{
				To:    &to,
				Value: &m,
				Data:  []byte{0x11, 0x12},
			},
		},
	}
	txData, err := json.Marshal(rawTransaction)
	if err != nil {
		t.Fatal(err)
	}
	r, err = httpPost(ts, ts.URL+"/transactions", txData)
	if err != nil {
		t.Fatal(err)
	}

}
func httpPost(ts *httptest.Server, url string, data []byte) ([]byte, error) {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	return r, nil
}
func httpPostForm(ts *httptest.Server, url string, body url.Values) ([]byte, error) {
	res, err := http.PostForm(url, body)
	if err != nil {
		return nil, err
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func initTransactionServer(t *testing.T) (*tx.Transaction, *httptest.Server) {
	db, _ := lvldb.NewMem()
	chain := chain.New(db)
	ti := api.NewTransactionInterface(chain, txpool.New())
	router := mux.NewRouter()
	api.NewTransactionHTTPRouter(router, ti)
	ts := httptest.NewServer(router)

	stateC := state.NewCreator(db)

	b, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain.WriteGenesis(b)
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(1000))
	tx := new(tx.Builder).
		ChainTag(0).
		GasPriceCoef(1).
		Gas(1000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
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
	if _, err := chain.AddBlock(bl, true); err != nil {
		t.Fatal(err)
	}
	return tx, ts
}

func checkTx(t *testing.T, expectedTx *types.Transaction, actualTx *types.Transaction) {
	assert.Equal(t, expectedTx.From, actualTx.From)
	assert.Equal(t, expectedTx.ID, actualTx.ID)
	assert.Equal(t, expectedTx.Index, actualTx.Index)
	assert.Equal(t, expectedTx.GasPriceCoef, actualTx.GasPriceCoef)
	assert.Equal(t, expectedTx.Gas, actualTx.Gas)
	for i, c := range expectedTx.Clauses {
		assert.Equal(t, string(c.Data), string(actualTx.Clauses[i].Data))
		assert.Equal(t, c.Value, actualTx.Clauses[i].Value)
		assert.Equal(t, c.To.String(), actualTx.Clauses[i].To.String())
	}

}
