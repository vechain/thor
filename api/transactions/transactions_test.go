package transactions_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/api/transfers"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

func TestTransaction(t *testing.T) {
	transaction, ts := initTransactionServer(t)
	defer ts.Close()
	getTx(t, ts, transaction)
	getTxReceipt(t, ts, transaction)
	senTx(t, ts, transaction)
	getTransfers(t, ts)
}

func getTx(t *testing.T, ts *httptest.Server, tx *tx.Transaction) {
	raw, err := transactions.ConvertTransaction(tx)
	if err != nil {
		t.Fatal(err)
	}
	res := httpGet(t, ts.URL+"/transactions/"+tx.ID().String())
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	checkTx(t, raw, rtx)
}

func getTxReceipt(t *testing.T, ts *httptest.Server, tx *tx.Transaction) {
	r := httpGet(t, ts.URL+"/transactions/"+tx.ID().String()+"/receipt")
	var receipt *transactions.Receipt
	if err := json.Unmarshal(r, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, uint64(receipt.GasUsed), tx.Gas(), "gas should be equal")
}

func senTx(t *testing.T, ts *httptest.Server, transaction *tx.Transaction) {
	rlpTx, err := rlp.EncodeToBytes(transaction)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(transactions.RawTx{Raw: hexutil.Encode(rlpTx)})
	if err != nil {
		t.Fatal(err)
	}
	res := httpPost(t, ts.URL+"/transactions", raw)
	var txObj map[string]string
	if err = json.Unmarshal(res, &txObj); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, transaction.ID().String(), txObj["id"], "shoudl be the same transaction")
}

func getTransfers(t *testing.T, ts *httptest.Server) {
	limit := 100
	res := httpGet(t, ts.URL+"/transactions/"+thor.Bytes32{}.String()+"/transfers")
	var tLogs []*transfers.FilteredTransfer
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, limit, len(tLogs), "should be `limit` transfers")
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
	transdb, err := transferdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	from := thor.BytesToAddress([]byte("from"))
	to := thor.BytesToAddress([]byte("to"))
	value := big.NewInt(10)
	header := new(block.Builder).Build().Header()
	var trs []*transferdb.Transfer
	count := 100
	for i := 0; i < count; i++ {
		transLog := &tx.Transfer{
			Sender:    from,
			Recipient: to,
			Amount:    value,
		}
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
		trans := transferdb.NewTransfer(header, uint32(i), thor.Bytes32{}, from, transLog)
		trs = append(trs, trans)
	}
	err = transdb.Insert(trs, nil)
	if err != nil {
		t.Fatal(err)
	}

	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	gene, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := gene.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tx := new(tx.Builder).
		ChainTag(chain.Tag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tx = tx.WithSignature(sig)
	pack := packer.New(chain, stateC, genesis.DevAccounts()[0].Address, genesis.DevAccounts()[0].Address)
	_, adopt, commit, err := pack.Prepare(b.Header(), uint64(time.Now().Unix()))
	err = adopt(tx)
	if err != nil {
		t.Fatal(err)
	}
	b, receipts, _, err := commit(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chain.AddBlock(b, receipts, true); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	pool := txpool.New(chain, stateC)
	defer pool.Stop()

	transactions.New(chain, pool, transdb).Mount(router, "/transactions")
	ts := httptest.NewServer(router)
	return tx, ts
}

func checkTx(t *testing.T, expectedTx *transactions.Transaction, actualTx *transactions.Transaction) {
	assert.Equal(t, expectedTx.Origin, actualTx.Origin)
	assert.Equal(t, expectedTx.ID, actualTx.ID)
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
