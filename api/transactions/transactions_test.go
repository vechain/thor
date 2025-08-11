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
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var (
	ts        *httptest.Server
	legacyTx  *tx.Transaction
	dynFeeTx  *tx.Transaction
	mempoolTx *tx.Transaction
	tclient   *thorclient.Client
	thorChain *testchain.Chain
	chainTag  byte
)

func TestTransaction(t *testing.T) {
	initTransactionServer(t)
	defer ts.Close()

	// Send tx
	tclient = thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T){
		"sendLegacyTx":                             sendLegacyTx,
		"sendImpossibleBlockRefExpiryTx":           sendImpossibleBlockRefExpiryTx,
		"sendTxWithBadFormat":                      sendTxWithBadFormat,
		"sendTxThatCannotBeAcceptedInLocalMempool": sendTxThatCannotBeAcceptedInLocalMempool,
		"sendDynamicFeeTx":                         sendDynamicFeeTx,
	} {
		t.Run(name, tt)
	}

	// Get tx
	for name, tt := range map[string]func(*testing.T){
		"getLegacyTx":     getLegacyTx,
		"getTxWithBadID":  getTxWithBadID,
		"txWithBadHeader": txWithBadHeader,
		"getNonExistingRawTransactionWhenTxStillInMempool": getNonExistingRawTransactionWhenTxStillInMempool,
		"getNonPendingRawTransactionWhenTxStillInMempool":  getNonPendingRawTransactionWhenTxStillInMempool,
		"getRawTransactionWhenTxStillInMempool":            getRawTransactionWhenTxStillInMempool,
		"getTransactionByIDTxNotFound":                     getTransactionByIDTxNotFound,
		"getTransactionByIDPendingTxNotFound":              getTransactionByIDPendingTxNotFound,
		"handleGetTransactionByIDWithBadQueryParams":       handleGetTransactionByIDWithBadQueryParams,
		"handleGetTransactionByIDWithNonExistingHead":      handleGetTransactionByIDWithNonExistingHead,

		"getDynamicFeeTx": getDynamicFeeTx,
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

func getLegacyTx(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+legacyTx.ID().String(), 200)
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	checkMatchingTx(t, legacyTx, rtx)

	res = httpGetAndCheckResponseStatus(t, "/transactions/"+legacyTx.ID().String()+"?raw=true", 200)
	var rawTx map[string]any
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}
	rlpTx, err := legacyTx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hexutil.Encode(rlpTx), rawTx["raw"], "should be equal raw")
}

func getDynamicFeeTx(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+dynFeeTx.ID().String(), 200)
	var rtx *transactions.Transaction
	if err := json.Unmarshal(res, &rtx); err != nil {
		t.Fatal(err)
	}
	checkMatchingTx(t, dynFeeTx, rtx)

	res = httpGetAndCheckResponseStatus(t, "/transactions/"+dynFeeTx.ID().String()+"?raw=true", 200)
	var rawTx map[string]any
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}
	encTx, err := dynFeeTx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hexutil.Encode(encTx), rawTx["raw"], "should be equal raw")
}

func getTxReceipt(t *testing.T) {
	r := httpGetAndCheckResponseStatus(t, "/transactions/"+legacyTx.ID().String()+"/receipt", 200)
	var receipt *api.Receipt
	if err := json.Unmarshal(r, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, receipt.GasUsed, legacyTx.Gas(), "receipt gas used not equal to transaction gas")
	assert.Equal(t, receipt.Type, legacyTx.Type())

	r = httpGetAndCheckResponseStatus(t, "/transactions/"+dynFeeTx.ID().String()+"/receipt", 200)
	if err := json.Unmarshal(r, &receipt); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, receipt.GasUsed, legacyTx.Gas(), "receipt gas used not equal to transaction gas")
	assert.Equal(t, receipt.Type, dynFeeTx.Type())
}

func sendLegacyTx(t *testing.T) {
	blockRef := tx.NewBlockRef(0)
	expiration := uint32(10)
	gas := uint64(21000)

	trx := tx.NewBuilder(tx.TypeLegacy).
		BlockRef(blockRef).
		ChainTag(chainTag).
		Expiration(expiration).
		Gas(gas).
		Build()
	trx = tx.MustSign(
		trx,
		genesis.DevAccounts()[0].PrivateKey,
	)

	rlpTx, err := trx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	res := httpPostAndCheckResponseStatus(t, "/transactions", api.RawTx{Raw: hexutil.Encode(rlpTx)}, 200)
	var txObj map[string]string
	if err = json.Unmarshal(res, &txObj); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, trx.ID().String(), txObj["id"], "should be the same transaction id")
}

func sendDynamicFeeTx(t *testing.T) {
	blockRef := tx.NewBlockRef(0)
	expiration := uint32(10)
	gas := uint64(21000)

	trx := tx.NewBuilder(tx.TypeDynamicFee).
		BlockRef(blockRef).
		ChainTag(chainTag).
		Expiration(expiration).
		Gas(gas).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxPriorityFeePerGas(big.NewInt(10)).
		Build()
	trx = tx.MustSign(
		trx,
		genesis.DevAccounts()[0].PrivateKey,
	)

	rlpTx, err := trx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	res := httpPostAndCheckResponseStatus(t, "/transactions", api.RawTx{Raw: hexutil.Encode(rlpTx)}, 200)
	var txObj map[string]string
	if err = json.Unmarshal(res, &txObj); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, trx.ID().String(), txObj["id"], "should be the same transaction id")
}

func sendImpossibleBlockRefExpiryTx(t *testing.T) {
	blockRef := tx.NewBlockRef(thorChain.Repo().BestBlockSummary().Header.Number())
	expiration := uint32(0)
	gas := uint64(21000)

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

	res := httpPostAndCheckResponseStatus(t, "/transactions", api.RawTx{Raw: hexutil.Encode(rlpTx)}, 403)
	assert.Equal(t, "tx rejected: expired\n", string(res), "should be expired")
}

func getTxWithBadID(t *testing.T) {
	txBadID := "0x123"

	res := httpGetAndCheckResponseStatus(t, "/transactions/"+txBadID, 400)

	assert.Contains(t, string(res), "invalid length")
}

func txWithBadHeader(t *testing.T) {
	badHeaderURL := []string{
		"/transactions/" + legacyTx.ID().String() + "?head=badHead",
		"/transactions/" + legacyTx.ID().String() + "/receipt?head=badHead",
		"/transactions/" + dynFeeTx.ID().String() + "?head=badHead",
		"/transactions/" + dynFeeTx.ID().String() + "/receipt?head=badHead",
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
	var rawTx map[string]any
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}

	assert.Empty(t, rawTx)
}

func getRawTransactionWhenTxStillInMempool(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/transactions/"+mempoolTx.ID().String()+"?raw=true&pending=true", 200)
	var rawTx map[string]any
	if err := json.Unmarshal(res, &rawTx); err != nil {
		t.Fatal(err)
	}
	rlpTx, err := mempoolTx.MarshalBinary()
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
	badRawTx := api.RawTx{Raw: "badRawTx"}

	res := httpPostAndCheckResponseStatus(t, "/transactions", badRawTx, 400)

	assert.Contains(t, string(res), hexutil.ErrMissingPrefix.Error())
}

func sendTxThatCannotBeAcceptedInLocalMempool(t *testing.T) {
	tx := tx.NewBuilder(tx.TypeLegacy).Build()
	rlpTx, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	duplicatedRawTx := api.RawTx{Raw: hexutil.Encode(rlpTx)}

	res := httpPostAndCheckResponseStatus(t, "/transactions", duplicatedRawTx, 400)

	assert.Contains(t, string(res), "bad tx: chain tag mismatch")
}

func handleGetTransactionByIDWithBadQueryParams(t *testing.T) {
	badQueryParams := []string{
		"?pending=badPending",
		"?pending=true&raw=badRaw",
	}

	for _, badQueryParam := range badQueryParams {
		res := httpGetAndCheckResponseStatus(t, "/transactions/"+legacyTx.ID().String()+badQueryParam, 400)
		assert.Contains(t, string(res), "should be boolean")
	}
}

func handleGetTransactionByIDWithNonExistingHead(t *testing.T) {
	res := httpGetAndCheckResponseStatus(
		t,
		"/transactions/"+legacyTx.ID().String()+"?head=0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		400,
	)
	assert.Equal(t, "head: leveldb: not found", strings.TrimSpace(string(res)))
}

func handleGetTransactionReceiptByIDWithNonExistingHead(t *testing.T) {
	res := httpGetAndCheckResponseStatus(
		t,
		"/transactions/"+legacyTx.ID().String()+"/receipt?head=0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		400,
	)
	assert.Equal(t, "head: leveldb: not found", strings.TrimSpace(string(res)))
}

func httpPostAndCheckResponseStatus(t *testing.T, url string, obj any, responseStatusCode int) []byte {
	body, statusCode, err := tclient.RawHTTPClient().RawHTTPPost(url, obj)
	require.NoError(t, err)
	assert.Equal(t, responseStatusCode, statusCode, fmt.Sprintf("status code should be %d", responseStatusCode))

	return body
}

func initTransactionServer(t *testing.T) {
	forkConfig := testchain.DefaultForkConfig
	forkConfig.GALACTICA = 2

	var err error
	thorChain, err = testchain.NewWithFork(&forkConfig, 180)
	require.NoError(t, err)

	chainTag = thorChain.Repo().ChainTag()

	// Creating first block with legacy tx
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	legacyTx = tx.NewBuilder(tx.TypeLegacy).
		ChainTag(chainTag).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	legacyTx = tx.MustSign(legacyTx, genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], legacyTx))

	dynFeeTx = tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(chainTag).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee * 10)).
		MaxPriorityFeePerGas(big.NewInt(10)).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	dynFeeTx = tx.MustSign(dynFeeTx, genesis.DevAccounts()[0].PrivateKey)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], dynFeeTx))

	mempool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute}, &forkConfig)

	mempoolTx = tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(chainTag).
		Expiration(10).
		Gas(21000).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee * 10)).
		MaxPriorityFeePerGas(big.NewInt(10)).
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
	assert.Equal(t, expectedTx.Gas(), actualTx.Gas)
	for i, c := range expectedTx.Clauses() {
		assert.Equal(t, hexutil.Encode(c.Data()), actualTx.Clauses[i].Data)
		assert.Equal(t, c.Value().String(), (*big.Int)(actualTx.Clauses[i].Value).String())
		assert.Equal(t, c.To(), actualTx.Clauses[i].To)
	}
	switch expectedTx.Type() {
	case tx.TypeLegacy:
		assert.Equal(t, expectedTx.GasPriceCoef(), *actualTx.GasPriceCoef)
		assert.Empty(t, actualTx.MaxFeePerGas)
		assert.Empty(t, actualTx.MaxPriorityFeePerGas)
	case tx.TypeDynamicFee:
		assert.Nil(t, actualTx.GasPriceCoef)
		assert.Equal(t, (*math.HexOrDecimal256)(expectedTx.MaxFeePerGas()), actualTx.MaxFeePerGas)
		assert.Equal(t, (*math.HexOrDecimal256)(expectedTx.MaxPriorityFeePerGas()), actualTx.MaxPriorityFeePerGas)
	}
}

func httpGetAndCheckResponseStatus(t *testing.T, url string, responseStatusCode int) []byte {
	body, statusCode, err := tclient.RawHTTPClient().RawHTTPGet(url)
	require.NoError(t, err)
	assert.Equal(t, responseStatusCode, statusCode, fmt.Sprintf("status code should be %d", responseStatusCode))

	return body
}
