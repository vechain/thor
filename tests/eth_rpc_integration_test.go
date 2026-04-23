// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package tests e2e integration for Spec 2 — the eth_* JSON-RPC namespace.
// Brings up a testchain + txpool + httptest server wrapping rpc.NewServer,
// then exercises the full pipeline: sign 0x02 -> eth_sendRawTransaction ->
// MintBlock -> eth_getTransactionReceipt -> eth_getBlockByNumber(fullTx).
package tests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api/rpc"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// newE2EServer returns a live httptest.Server wrapping rpc.NewServer with a
// fresh testchain + txpool. The returned client closes the server on test
// cleanup.
func newE2EServer(t *testing.T) (*httptest.Server, *testchain.Chain, *txpool.TxPool) {
	t.Helper()
	fork := &thor.ForkConfig{}
	tc, err := testchain.NewWithFork(fork, 180)
	require.NoError(t, err)

	pool := txpool.New(tc.Repo(), tc.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 10,
		MaxLifetime:     time.Hour,
	}, tc.GetForkConfig())

	rpcServer := rpc.NewServer(tc.Repo(), tc.Stater(), pool, tc.LogDB(), tc.GetForkConfig(), tc.Engine(), rpc.Config{
		CallGasLimit:               10_000_000,
		APIBacktraceLimit:          100,
		LogsLimit:                  1000,
		PriorityIncreasePercentage: 5,
	})
	srv := httptest.NewServer(rpcServer)
	t.Cleanup(srv.Close)
	return srv, tc, pool
}

// callRPC posts {method, params} to the e2e server and returns the parsed
// result and error fields of the envelope.
func callRPC(t *testing.T, srv *httptest.Server, method string, params ...any) (result, errField json.RawMessage) {
	t.Helper()

	paramsRaw, err := json.Marshal(params)
	require.NoError(t, err)

	body := struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
		ID      int             `json:"id"`
	}{"2.0", method, paramsRaw, 1}

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &env), "raw: %s", string(respBody))
	return env["result"], env["error"]
}

// --- chain meta ----------------------------------------------------------

func TestEthRPC_E2E_ChainIdAndBlockNumber(t *testing.T) {
	srv, tc, _ := newE2EServer(t)

	result, errField := callRPC(t, srv, "eth_chainId")
	require.Empty(t, errField)
	var chainIDHex string
	require.NoError(t, json.Unmarshal(result, &chainIDHex))
	expected := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	got, ok := new(big.Int).SetString(strings.TrimPrefix(chainIDHex, "0x"), 16)
	require.True(t, ok)
	assert.Equal(t, 0, expected.Cmp(got))

	// blockNumber before any mint is 0x0.
	result, _ = callRPC(t, srv, "eth_blockNumber")
	var n string
	require.NoError(t, json.Unmarshal(result, &n))
	assert.Equal(t, "0x0", n)

	require.NoError(t, tc.MintBlock())
	result, _ = callRPC(t, srv, "eth_blockNumber")
	require.NoError(t, json.Unmarshal(result, &n))
	assert.Equal(t, "0x1", n, "blockNumber increments after mint")
}

// --- full tx lifecycle ---------------------------------------------------

func TestEthRPC_E2E_SendReceivePack(t *testing.T) {
	srv, tc, _ := newE2EServer(t)

	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	to := thor.BytesToAddress([]byte("e2e"))
	builder := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(&to).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).
		Nonce(7)
	trx := tx.MustSign(builder.Build(), genesis.DevAccounts()[0].PrivateKey)
	raw, err := trx.MarshalBinary()
	require.NoError(t, err)

	// eth_sendRawTransaction — pool admission.
	sendResult, errField := callRPC(t, srv, "eth_sendRawTransaction", "0x"+hex.EncodeToString(raw))
	require.Empty(t, errField, "err: %s", string(errField))
	var returnedID thor.Bytes32
	require.NoError(t, json.Unmarshal(sendResult, &returnedID))
	assert.Equal(t, trx.CanonicalTxID(), returnedID)

	// Before packing the tx is pending — getTransactionByHash may return
	// null (txpool isn't connected to the eth_ getByHash lookup — that's by
	// design, spec focuses on mined txs). After MintBlock it resolves.
	require.NoError(t, tc.MintBlock(trx))

	// eth_getTransactionByHash — mined.
	got, errField := callRPC(t, srv, "eth_getTransactionByHash", trx.CanonicalTxID().String())
	require.Empty(t, errField)
	var txObj map[string]any
	require.NoError(t, json.Unmarshal(got, &txObj))
	assert.Equal(t, trx.CanonicalTxID().String(), txObj["hash"])
	assert.Equal(t, "0x2", txObj["type"])

	// eth_getTransactionReceipt.
	got, errField = callRPC(t, srv, "eth_getTransactionReceipt", trx.CanonicalTxID().String())
	require.Empty(t, errField)
	var rcpt map[string]any
	require.NoError(t, json.Unmarshal(got, &rcpt))
	assert.Equal(t, "0x2", rcpt["type"])
	assert.Equal(t, "0x1", rcpt["status"])

	// eth_getBlockByNumber(1, fullTx=true) contains the tx.
	got, errField = callRPC(t, srv, "eth_getBlockByNumber", "0x1", true)
	require.Empty(t, errField)
	var blk map[string]any
	require.NoError(t, json.Unmarshal(got, &blk))
	txs, ok := blk["transactions"].([]any)
	require.True(t, ok)
	require.Len(t, txs, 1)
	first, _ := txs[0].(map[string]any)
	assert.Equal(t, trx.CanonicalTxID().String(), first["hash"])
}

// --- balance at genesis ---------------------------------------------------

func TestEthRPC_E2E_GetBalance(t *testing.T) {
	srv, _, _ := newE2EServer(t)

	devAddr := genesis.DevAccounts()[0].Address
	got, errField := callRPC(t, srv, "eth_getBalance", devAddr.String(), "latest")
	require.Empty(t, errField)
	var hexStr string
	require.NoError(t, json.Unmarshal(got, &hexStr))
	n, ok := new(big.Int).SetString(strings.TrimPrefix(hexStr, "0x"), 16)
	require.True(t, ok)
	assert.Equal(t, 1, n.Sign(), "dev account seeded balance > 0")
}

// --- fee methods -----------------------------------------------------------

func TestEthRPC_E2E_FeeMethods(t *testing.T) {
	srv, tc, _ := newE2EServer(t)
	require.NoError(t, tc.MintBlock())

	for _, m := range []string{"eth_gasPrice", "eth_maxPriorityFeePerGas"} {
		got, errField := callRPC(t, srv, m)
		require.Empty(t, errField, "method %s err: %s", m, string(errField))
		var hexStr string
		require.NoError(t, json.Unmarshal(got, &hexStr))
		n, ok := new(big.Int).SetString(strings.TrimPrefix(hexStr, "0x"), 16)
		require.True(t, ok)
		assert.Equal(t, 1, n.Sign(), "%s must be positive", m)
	}

	got, errField := callRPC(t, srv, "eth_feeHistory", "0x1", "latest")
	require.Empty(t, errField)
	var env map[string]any
	require.NoError(t, json.Unmarshal(got, &env))
	assert.NotNil(t, env["oldestBlock"])
	require.Contains(t, env, "baseFeePerGas")
	bf, _ := env["baseFeePerGas"].([]any)
	assert.Len(t, bf, 2, "baseFeePerGas has blockCount+1 entries")
}

// --- eth_call against a precompile or code-less address -------------------

func TestEthRPC_E2E_Call_EmptyAddress(t *testing.T) {
	srv, _, _ := newE2EServer(t)

	args := map[string]any{
		"from": genesis.DevAccounts()[0].Address.String(),
		"to":   "0x000000000000000000000000000000000000dead",
		"data": "0x",
	}
	got, errField := callRPC(t, srv, "eth_call", args, "latest")
	require.Empty(t, errField, "err: %s", string(errField))
	var s string
	require.NoError(t, json.Unmarshal(got, &s))
	assert.Equal(t, "0x", s)
}

// --- error envelope shape --------------------------------------------------

func TestEthRPC_E2E_UnknownMethod(t *testing.T) {
	srv, _, _ := newE2EServer(t)
	_, errField := callRPC(t, srv, "eth_notAThing")
	require.NotEmpty(t, errField)
	var e map[string]any
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, float64(-32601), e["code"])
}
