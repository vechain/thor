// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/inconshreveable/log15"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func init() {
	log15.Root().SetHandler(log15.DiscardHandler())
}

var repo *chain.Repository
var ts *httptest.Server
var transaction *tx.Transaction
var mempoolTx *tx.Transaction

func TestTransaction(t *testing.T) {
	initTransactionServer(t)
	defer ts.Close()

	// Send tx
	sendTx(t)
	sendTxWithBadFormat(t)
	sendTxThatCannotBeAcceptedInLocalMempool(t)

	// Get tx
	getTx(t)
	getTxWithBadId(t)
	txWithBadHeader(t)
	getNonExistingRawTransactionWhenTxStillInMempool(t)
	getNonPendingRawTransactionWhenTxStillInMempool(t)
	getRawTransactionWhenTxStillInMempool(t)
	getTransactionByIDTxNotFound(t)
	getTransactionByIDPendingTxNotFound(t)
	handleGetTransactionByIDWithBadQueryParams(t)
	handleGetTransactionByIDWithNonExistingHead(t)
	handleGetTransactionByIdWithValidRevisions(t)
	handleGetTransactionByIdWithFinalRevision(t)

	// Get tx receipt
	getTxReceipt(t)
	getReceiptWithBadId(t)
	handleGetTransactionReceiptByIDWithNonExistingHead(t)
	handleGetTransactionReceiptWithValidRevisions(t)
	handleGetTransactionReceiptWithFinalRevision(t)
}

func getTx(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String(), 200)
	var rtx *transactions.Transaction
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
	var receipt *transactions.Receipt
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

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/transactions", transactions.RawTx{Raw: hexutil.Encode(rlpTx)}, 200)
	var txObj map[string]string
	if err = json.Unmarshal(res, &txObj); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, tx.ID().String(), txObj["id"], "should be the same transaction id")
}

func getTxWithBadId(t *testing.T) {
	txBadId := "0x123"

	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+txBadId, 400)

	assert.Contains(t, string(res), "invalid length")
}

func txWithBadHeader(t *testing.T) {
	badHeaderURL := []string{
		ts.URL + "/transactions/" + transaction.ID().String() + "?head=badHead",
		ts.URL + "/transactions/" + transaction.ID().String() + "/receipt?head=badHead",
	}

	for _, url := range badHeaderURL {
		res := httpGetAndCheckResponseStatus(t, url, 400)
		assert.Contains(t, string(res), "invalid syntax")
	}
}

func getReceiptWithBadId(t *testing.T) {
	txBadId := "0x123"

	httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+txBadId+"/receipt", 400)
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
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}

	checkMatchingTx(t, mempoolTx, rtx)
}

func sendTxWithBadFormat(t *testing.T) {
	badRawTx := transactions.RawTx{Raw: "badRawTx"}

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/transactions", badRawTx, 400)

	assert.Contains(t, string(res), hexutil.ErrMissingPrefix.Error())
}

func sendTxThatCannotBeAcceptedInLocalMempool(t *testing.T) {
	tx := new(tx.Builder).Build()
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		t.Fatal(err)
	}
	duplicatedRawTx := transactions.RawTx{Raw: hexutil.Encode(rlpTx)}

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/transactions", duplicatedRawTx, 400)

	assert.Contains(t, string(res), "bad tx: chain tag mismatch")
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

func handleGetTransactionByIDWithNonExistingHead(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"?head=0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 400)
	assert.Equal(t, "head: leveldb: not found", strings.TrimSpace(string(res)))
}

func handleGetTransactionByIdWithValidRevisions(t *testing.T) {
	revisions := []string{
		"1",
		"best",
		repo.BestBlockSummary().Header.ID().String(),
	}

	for _, revision := range revisions {
		res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"?head="+revision, 200)
		var rtx *transactions.Transaction
		if err := json.Unmarshal(res, &rtx); err != nil {
			t.Fatal(err)
		}
		checkMatchingTx(t, transaction, rtx)
	}
}

func handleGetTransactionByIdWithFinalRevision(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"?head=finalized", 200)
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	assert.Nil(t, rtx, "should be nil")
}

func handleGetTransactionReceiptByIDWithNonExistingHead(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"/receipt?head=0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 400)
	assert.Equal(t, "head: leveldb: not found", strings.TrimSpace(string(res)))
}

func handleGetTransactionReceiptWithValidRevisions(t *testing.T) {
	revisions := []string{
		"1",
		"best",
		repo.BestBlockSummary().Header.ID().String(),
	}

	for _, revision := range revisions {
		res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"/receipt?head="+revision, 200)
		var receipt *transactions.Receipt
		if err := json.Unmarshal(res, &receipt); err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, receipt.GasUsed, transaction.Gas(), "receipt gas used not equal to transaction gas")
	}
}

func handleGetTransactionReceiptWithFinalRevision(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, ts.URL+"/transactions/"+transaction.ID().String()+"/receipt?head=finalized", 200)
	var receipt *transactions.Receipt
	if err := json.Unmarshal(res, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Nil(t, receipt, "receipt gas used not equal to transaction gas")
}

func httpPostAndCheckResponseStatus(t *testing.T, url string, obj interface{}, responseStatusCode int) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, responseStatusCode, res.StatusCode, fmt.Sprintf("status code should be %d", responseStatusCode))
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
		t.Fatal(e)
	}

	revisionHandler := utils.NewRevisionHandler(repo, solo.NewBFTEngine(repo))

	transactions.New(repo, mempool, revisionHandler).Mount(router, "/transactions")

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
