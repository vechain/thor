package transactions_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/txpool"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestTransaction(t *testing.T) {

	transaction, ts := initTransactionServer(t)
	defer ts.Close()
	getTx(t, ts, transaction)
	getTxReceipt(t, ts, transaction)
	senTx(t, ts, transaction)
}

func getTx(t *testing.T, ts *httptest.Server, tx *tx.Transaction) {
	raw, err := transactions.ConvertTransaction(tx)
	if err != nil {
		t.Fatal(err)
	}
	res := httpGet(t, ts.URL+fmt.Sprintf("/transactions/%v", tx.ID()))
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	checkTx(t, raw, rtx)
}

func getTxReceipt(t *testing.T, ts *httptest.Server, tx *tx.Transaction) {
	r := httpGet(t, ts.URL+fmt.Sprintf("/transactions/%v/receipts", tx.ID().String()))
	var receipt *transactions.Receipt
	if err := json.Unmarshal(r, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, uint64(receipt.GasUsed), tx.Gas(), "gas should be equal")
}

func senTx(t *testing.T, ts *httptest.Server, transaction *tx.Transaction) {
	sig, err := crypto.Sign(transaction.SigningHash().Bytes(), genesis.Dev.Accounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	to := thor.BytesToAddress([]byte("to"))
	v := big.NewInt(10000)
	blockRef := tx.NewBlockRef(0)
	rawTransaction := &transactions.RawTransaction{
		Nonce:        1,
		ChainTag:     transaction.ChainTag(),
		GasPriceCoef: 1,
		Gas:          21000,
		Sig:          hexutil.Encode(sig),
		BlockRef:     hexutil.Encode(blockRef[:]),
		Clauses: transactions.Clauses{
			transactions.Clause{
				To:    &to,
				Value: math.HexOrDecimal256(*v),
				Data:  hexutil.Encode(nil),
			},
		},
	}
	txData, err := json.Marshal(rawTransaction)
	if err != nil {
		t.Fatal(err)
	}
	res := httpPost(t, ts.URL+"/transactions", txData)
	var txID string
	if err = json.Unmarshal(res, &txID); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, transaction.ID().String(), txID, "shoudl be the same transaction")
}

func httpPost(t *testing.T, url string, data []byte) []byte {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func initTransactionServer(t *testing.T) (*tx.Transaction, *httptest.Server) {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	b, _, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tx := new(tx.Builder).
		ChainTag(chain.Tag()).
		GasPriceCoef(1).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), genesis.Dev.Accounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tx = tx.WithSignature(sig)
	pack := packer.New(chain, stateC, genesis.Dev.Accounts()[0].Address, genesis.Dev.Accounts()[0].Address)
	_, adopt, commit, err := pack.Prepare(b.Header(), uint64(time.Now().Unix()))
	err = adopt(tx)
	if err != nil {
		t.Fatal(err)
	}
	b, receipts, err := commit(genesis.Dev.Accounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chain.AddBlock(b, receipts, true); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	pool := txpool.New(chain, stateC)
	defer pool.Stop()
	transactions.New(chain, pool).Mount(router, "/transactions")
	ts := httptest.NewServer(router)
	return tx, ts
}

func checkTx(t *testing.T, expectedTx *transactions.Transaction, actualTx *transactions.Transaction) {
	assert.Equal(t, expectedTx.Signer, actualTx.Signer)
	assert.Equal(t, expectedTx.ID, actualTx.ID)
	assert.Equal(t, expectedTx.TxIndex, actualTx.TxIndex)
	assert.Equal(t, expectedTx.GasPriceCoef, actualTx.GasPriceCoef)
	assert.Equal(t, expectedTx.Gas, actualTx.Gas)
	for i, c := range expectedTx.Clauses {
		assert.Equal(t, string(c.Data), string(actualTx.Clauses[i].Data))
		assert.Equal(t, c.Value, actualTx.Clauses[i].Value)
		assert.Equal(t, c.To, actualTx.Clauses[i].To)
	}

}

func httpGet(t *testing.T, url string) []byte {
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
