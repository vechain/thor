// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var (
	ts          *httptest.Server
	transaction *tx.Transaction
	mempoolTx   *tx.Transaction
	tclient     *thorclient.Client
	chainTag    byte
)

func TestTransaction(t *testing.T) {
	initTransactionServer(t)
	defer ts.Close()

	// Send tx
	tclient = thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T){
		"sendTx":              sendTx,
		"sendTxWithBadFormat": sendTxWithBadFormat,
		"sendTxThatCannotBeAcceptedInLocalMempool": sendTxThatCannotBeAcceptedInLocalMempool,
	} {
		t.Run(name, tt)
	}

	// Get tx
	for name, tt := range map[string]func(*testing.T){
		"getTx":           getTx,
		"getTxWithBadID":  getTxWithBadID,
		"txWithBadHeader": txWithBadHeader,
		"getNonExistingRawTransactionWhenTxStillInMempool": getNonExistingRawTransactionWhenTxStillInMempool,
		"getNonPendingRawTransactionWhenTxStillInMempool":  getNonPendingRawTransactionWhenTxStillInMempool,
		"getRawTransactionWhenTxStillInMempool":            getRawTransactionWhenTxStillInMempool,
		"getTransactionByIDTxNotFound":                     getTransactionByIDTxNotFound,
		"getTransactionByIDPendingTxNotFound":              getTransactionByIDPendingTxNotFound,
		"handleGetTransactionByIDWithBadQueryParams":       handleGetTransactionByIDWithBadQueryParams,
		"handleGetTransactionByIDWithNonExistingHead":      handleGetTransactionByIDWithNonExistingHead,
	} {
		t.Run(name, tt)
	}

	// Get tx receipt
	for name, tt := range map[string]func(*testing.T){
		"getTxReceipt":        getTxReceipt,
		"getReceiptWithBadID": getReceiptWithBadID,
		"handleGetTransactionReceiptByIDWithNonExistingHead": handleGetTransactionReceiptByIDWithNonExistingHead,
	} {
		t.Run(name, tt)
	}
}

func getTx(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+transaction.ID().String(), 200)
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	checkMatchingTx(t, transaction, rtx)

	res = httpGetAndCheckResponseStatus(t, "/transactions/"+transaction.ID().String()+"?raw=true", 200)
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
	r := httpGetAndCheckResponseStatus(t, "/transactions/"+transaction.ID().String()+"/receipt", 200)
	var receipt *transactions.Receipt
	if err := json.Unmarshal(r, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, receipt.GasUsed, transaction.Gas(), "receipt gas used not equal to transaction gas")
}

func sendTx(t *testing.T) {
	var blockRef = tx.NewBlockRef(0)
	var expiration = uint32(10)
	var gas = uint64(21000)

	trx := tx.MustSign(
		new(tx.Builder).
			BlockRef(blockRef).
			ChainTag(chainTag).
			Expiration(expiration).
			Gas(gas).
			Build(),
		genesis.DevAccounts()[0].PrivateKey,
	)

	rlpTx, err := rlp.EncodeToBytes(trx)
	if err != nil {
		t.Fatal(err)
	}

	res := httpPostAndCheckResponseStatus(t, "/transactions", transactions.RawTx{Raw: hexutil.Encode(rlpTx)}, 200)
	var txObj map[string]string
	if err = json.Unmarshal(res, &txObj); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, trx.ID().String(), txObj["id"], "should be the same transaction id")
}

func getTxWithBadID(t *testing.T) {
	txBadID := "0x123"

	res := httpGetAndCheckResponseStatus(t, "/transactions/"+txBadID, 400)

	assert.Contains(t, string(res), "invalid length")
}

func txWithBadHeader(t *testing.T) {
	badHeaderURL := []string{
		"/transactions/" + transaction.ID().String() + "?head=badHead",
		"/transactions/" + transaction.ID().String() + "/receipt?head=badHead",
	}

	for _, url := range badHeaderURL {
		res := httpGetAndCheckResponseStatus(t, url, 400)
		assert.Contains(t, string(res), "invalid length")
	}
}

func getReceiptWithBadID(t *testing.T) {
	txBadID := "0x123"

	httpGetAndCheckResponseStatus(t, "/transactions/"+txBadID+"/receipt", 400)
}

func getNonExistingRawTransactionWhenTxStillInMempool(t *testing.T) {
	nonExistingTxID := "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	queryParams := []string{
		"?raw=true",
		"?raw=true&pending=true",
	}

	for _, queryParam := range queryParams {
		res := httpGetAndCheckResponseStatus(t, "/transactions/"+nonExistingTxID+queryParam, 200)

		assert.Equal(t, "null\n", string(res))
	}
}

func getNonPendingRawTransactionWhenTxStillInMempool(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+mempoolTx.ID().String()+"?raw=true", 200)
	var rawTx map[string]interface{}
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}

	assert.Empty(t, rawTx)
}

func getRawTransactionWhenTxStillInMempool(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+mempoolTx.ID().String()+"?raw=true&pending=true", 200)
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
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+mempoolTx.ID().String(), 200)

	assert.Equal(t, "null\n", string(res))
}

func getTransactionByIDPendingTxNotFound(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+mempoolTx.ID().String()+"?pending=true", 200)
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}

	checkMatchingTx(t, mempoolTx, rtx)
}

func sendTxWithBadFormat(t *testing.T) {
	badRawTx := transactions.RawTx{Raw: "badRawTx"}

	res := httpPostAndCheckResponseStatus(t, "/transactions", badRawTx, 400)

	assert.Contains(t, string(res), hexutil.ErrMissingPrefix.Error())
}

func sendTxThatCannotBeAcceptedInLocalMempool(t *testing.T) {
	tx := new(tx.Builder).Build()
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		t.Fatal(err)
	}
	duplicatedRawTx := transactions.RawTx{Raw: hexutil.Encode(rlpTx)}

	res := httpPostAndCheckResponseStatus(t, "/transactions", duplicatedRawTx, 400)

	assert.Contains(t, string(res), "bad tx: chain tag mismatch")
}

func handleGetTransactionByIDWithBadQueryParams(t *testing.T) {
	badQueryParams := []string{
		"?pending=badPending",
		"?pending=true&raw=badRaw",
	}

	for _, badQueryParam := range badQueryParams {
		res := httpGetAndCheckResponseStatus(t, "/transactions/"+transaction.ID().String()+badQueryParam, 400)
		assert.Contains(t, string(res), "should be boolean")
	}
}

func handleGetTransactionByIDWithNonExistingHead(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+transaction.ID().String()+"?head=0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 400)
	assert.Equal(t, "head: leveldb: not found", strings.TrimSpace(string(res)))
}

func handleGetTransactionReceiptByIDWithNonExistingHead(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+transaction.ID().String()+"/receipt?head=0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 400)
	assert.Equal(t, "head: leveldb: not found", strings.TrimSpace(string(res)))
}

func httpPostAndCheckResponseStatus(t *testing.T, url string, obj interface{}, responseStatusCode int) []byte {
	body, statusCode, err := tclient.RawHTTPClient().RawHTTPPost(url, obj)
	require.NoError(t, err)
	assert.Equal(t, responseStatusCode, statusCode, fmt.Sprintf("status code should be %d", responseStatusCode))

	return body
}

func initTransactionServer(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	chainTag = thorChain.Repo().ChainTag()

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	transaction = new(tx.Builder).
		ChainTag(chainTag).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	transaction = tx.MustSign(transaction, genesis.DevAccounts()[0].PrivateKey)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], transaction))

	mempool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute})

	mempoolTx = new(tx.Builder).
		ChainTag(chainTag).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Build()
	mempoolTx = tx.MustSign(mempoolTx, genesis.DevAccounts()[0].PrivateKey)

	// Add a tx to the mempool to have both pending and non-pending transactions
	e := mempool.Add(mempoolTx)
	if e != nil {
		t.Fatal(e)
	}

	router := mux.NewRouter()
	transactions.New(thorChain.Repo(), mempool).Mount(router, "/transactions")

	ts = httptest.NewServer(router)
}

func checkMatchingTx(t *testing.T, expectedTx *tx.Transaction, actualTx *transactions.Transaction) {
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
	body, statusCode, err := tclient.RawHTTPClient().RawHTTPGet(url)
	require.NoError(t, err)
	assert.Equal(t, responseStatusCode, statusCode, fmt.Sprintf("status code should be %d", responseStatusCode))

	return body
}
