// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var repo *chain.Repository
var ts *httptest.Server
var transaction *tx.Transaction
var mempoolTx *tx.Transaction

func TestTransaction(t *testing.T) {
	initTransactionServer(t)
	defer ts.Close()

	getTx(t)
	getTxReceipt(t)
	sendTx(t)
	getTxWithBadId(t)
	getReceiptWithBadId(t)
	txWithBadHeader(t)
	checkBlockSummaryExistsInRepoForNonExistingBlock(t)
	getNonExistingRawTransactionWhenTxStillInMempool(t)
	getNonPendingRawTransactionWhenTxStillInMempool(t)
	getRawTransactionWhenTxStillInMempool(t)
	getTransactionByIDTxNotFound(t)
	getTransactionByIDPendingTxNotFound(t)
	sendTxWithBadFormat(t)
	sendTxThatCannotBeAcceptedInLocalMempool(t)
	handleGetTransactionByIDWithBadQueryParams(t)
}

func getTx(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String(), 200)
	var rtx *Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	checkMatchingTx(t, transaction, rtx)

	res = httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"?raw=true", 200)
	var rawTx map[string]interface{}
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}
	rlpTx, err := rlp.EncodeToBytes(transaction)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hexutil.Encode(rlpTx), rawTx["raw"], "should be equal raw")
}

func getTxReceipt(t *testing.T) {
	r := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"/receipt", 200)
	var receipt *Receipt
	if err := json.Unmarshal(r, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, receipt.GasUsed, transaction.Gas(), "receipt gas used not equal to transaction gas")
}

func sendTx(t *testing.T) {
	var blockRef = tx.NewBlockRef(0)
	var chainTag = repo.ChainTag()
	var expiration = uint32(10)
	var gas = uint64(21000)

	tx := new(tx.Builder).
		BlockRef(blockRef).
		ChainTag(chainTag).
		Expiration(expiration).
		Gas(gas).
		Build()
	sig, err := crypto.Sign(tx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tx = tx.WithSignature(sig)
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		t.Fatal(err)
	}

	res := httpPost(t, ts.URL+"/transactions", RawTx{Raw: hexutil.Encode(rlpTx)})
	var txObj map[string]string
	if err = json.Unmarshal(res, &txObj); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, tx.ID().String(), txObj["id"], "should be the same transaction id")
}

func getTxWithBadId(t *testing.T) {
	txBadId := "0x123"

	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+txBadId, 400)

	assert.Contains(t, string(res), "id:")
}

func txWithBadHeader(t *testing.T) {
	badHeaderURL := []string{
		ts.URL + "/transactions/" + transaction.ID().String() + "?head=badHead",
		ts.URL + "/transactions/" + transaction.ID().String() + "/receipt?head=badHead",
	}

	for _, url := range badHeaderURL {
		res := httpGetAndCheckResponseStatus(t, url, 400)
		assert.Contains(t, string(res), "head:")
	}
}

func getReceiptWithBadId(t *testing.T) {
	txBadId := "0x123"

	httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+txBadId+"/receipt", 400)
}

func checkBlockSummaryExistsInRepoForNonExistingBlock(t *testing.T) {
	head := thor.Bytes32{}

	err := checkBlockSummaryExistsInRepo(repo, head)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func getNonExistingRawTransactionWhenTxStillInMempool(t *testing.T) {
	nonExistingTxId := "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	queryParams := []string{
		"?raw=true",
		"?raw=true&pending=true",
	}

	for _, queryParam := range queryParams {
		res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+nonExistingTxId+queryParam, 200)

		assert.Equal(t, "null\n", string(res))
	}
}

func getNonPendingRawTransactionWhenTxStillInMempool(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+mempoolTx.ID().String()+"?raw=true", 200)
	var rawTx map[string]interface{}
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}

	assert.Empty(t, rawTx)
}

func getRawTransactionWhenTxStillInMempool(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+mempoolTx.ID().String()+"?raw=true&pending=true", 200)
	var rawTx map[string]interface{}
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}
	rlpTx, err := rlp.EncodeToBytes(mempoolTx)
	if err != nil {
		t.Fatal(err)
	}

	assert.NotEmpty(t, rawTx)
	assert.Equal(t, hexutil.Encode(rlpTx), rawTx["raw"], "should be equal raw")
}

func getTransactionByIDTxNotFound(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+mempoolTx.ID().String(), 200)

	assert.Equal(t, "null\n", string(res))
}

func getTransactionByIDPendingTxNotFound(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+mempoolTx.ID().String()+"?pending=true", 200)
	var rtx *Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}

	checkMatchingTx(t, mempoolTx, rtx)
}

func sendTxWithBadFormat(t *testing.T) {
	badRawTx := RawTx{Raw: "badRawTx"}
	rawTxJson, err := json.Marshal(badRawTx)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := http.Post(ts.URL+"/transactions", "application/x-www-form-urlencoded", bytes.NewReader(rawTxJson))

	assert.Equal(t, 400, res.StatusCode, "status code should be 400")
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, string(r), "raw:")
}

func sendTxThatCannotBeAcceptedInLocalMempool(t *testing.T) {
	tx := new(tx.Builder).Build()
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		t.Fatal(err)
	}
	duplicatedRawTx := RawTx{Raw: hexutil.Encode(rlpTx)}
	data, err := json.Marshal(duplicatedRawTx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(ts.URL+"/transactions", "application/x-www-form-urlencoded", bytes.NewReader(data))

	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode, "status code should be 400")
}

func handleGetTransactionByIDWithBadQueryParams(t *testing.T) {
	badQueryParams := []string{
		"?pending=badPending",
		"?pending=true&raw=badRaw",
	}

	for _, badQueryParam := range badQueryParams {
		res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+badQueryParam, 400)
		assert.Contains(t, string(res), "should be boolean")
	}

}

func httpPost(t *testing.T, url string, obj interface{}) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r := parseBytesBody(t, res.Body)
	res.Body.Close()
	return r
}

func initTransactionServer(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ = chain.NewRepository(db, b)
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	transaction = new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	mempoolTx = new(tx.Builder).
		ChainTag(repo.ChainTag()).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Build()

	sig, err := crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	sig2, err := crypto.Sign(mempoolTx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	transaction = transaction.WithSignature(sig)
	mempoolTx = mempoolTx.WithSignature(sig2)

	packer := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, _ := repo.GetBlockSummary(b.Header().ID())
	flow, err := packer.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	err = flow.Adopt(transaction)
	if err != nil {
		t.Fatal(err)
	}
	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBlock(b, receipts, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBestBlockID(b.Header().ID()); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()

	// Add a tx to the mempool to have both pending and non-pending transactions
	mempool := txpool.New(repo, stater, txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute})
	e := mempool.Add(mempoolTx)
	if e != nil {
		t.Fatal("Fatalinooo", e)
	}

	New(repo, mempool).Mount(router, "/transactions")

	ts = httptest.NewServer(router)
}

func checkMatchingTx(t *testing.T, expectedTx *tx.Transaction, actualTx *Transaction) {
	origin, err := expectedTx.Origin()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, origin, actualTx.Origin)
	assert.Equal(t, expectedTx.ID(), actualTx.ID)
	assert.Equal(t, expectedTx.GasPriceCoef(), actualTx.GasPriceCoef)
	assert.Equal(t, expectedTx.Gas(), actualTx.Gas)
	for i, c := range expectedTx.Clauses() {
		assert.Equal(t, hexutil.Encode(c.Data()), actualTx.Clauses[i].Data)
		assert.Equal(t, *c.Value(), big.Int(actualTx.Clauses[i].Value))
		assert.Equal(t, c.To(), actualTx.Clauses[i].To)
	}
}

func httpGetAndCheckResponseStatus(t *testing.T, url string, responseStatusCode int) []byte {
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, responseStatusCode, res.StatusCode, fmt.Sprintf("status code should be %d", responseStatusCode))
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func parseBytesBody(t *testing.T, body io.ReadCloser) []byte {
	r, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
