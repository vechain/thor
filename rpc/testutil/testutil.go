// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package testutil provides test helpers for the rpc package and its sub-packages.
// It deliberately does NOT import any rpc sub-package so that sub-package tests
// can import testutil without creating a circular dependency.
package testutil

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// BuildEthTx creates a signed EIP-1559 tx from sender (at the given nonce) to to.
func BuildEthTx(t *testing.T, chainID uint64, sender genesis.DevAccount, nonce uint64, to *thor.Address) *tx.Transaction {
	t.Helper()
	unsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		Nonce(nonce).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		Gas(21000).
		To(to).
		Value(big.NewInt(1e9)).
		Build()
	ethTx, err := tx.Sign(unsigned, sender.PrivateKey)
	require.NoError(t, err)
	return ethTx
}

// BuildVcTx creates a signed TypeLegacy VeChain tx from sender to to.
func BuildVcTx(t *testing.T, c *testchain.Chain, sender genesis.DevAccount, to *thor.Address) *tx.Transaction {
	t.Helper()
	vcTx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(c.Repo().ChainTag()).
		BlockRef(tx.NewBlockRef(c.Repo().BestBlockSummary().Header.Number())).
		Expiration(1000).
		GasPriceCoef(255).
		Gas(21000).
		Nonce(datagen.RandUint64()).
		Clause(tx.NewClause(to).WithValue(big.NewInt(1e9))).
		Build()
	return tx.MustSign(vcTx, sender.PrivateKey)
}

// Mounter is satisfied by any sub-package handler that exposes Mount.
type Mounter interface {
	Mount(s *rpc.Server)
}

// NewTestServer creates an httptest.Server with only m's methods registered.
// Sub-package tests use this for focused isolation — only the handler under test
// is mounted, so an accidental call to another namespace fails with method-not-found.
func NewTestServer(t *testing.T, m Mounter) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	m.Mount(srv)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts
}

// Call posts a JSON-RPC 2.0 request and returns the result field.
// The test fails immediately if the server returns an RPC error.
func Call(t *testing.T, ts *httptest.Server, method string, params any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/rpc", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *rpc.RPCError   `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	if rpcResp.Error != nil {
		t.Fatalf("unexpected RPC error for %s: code=%d msg=%s", method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result
}

// CallExpectError posts a JSON-RPC 2.0 request and returns the RPC error.
// The test fails if no error is returned.
func CallExpectError(t *testing.T, ts *httptest.Server, method string, params any) *rpc.RPCError {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/rpc", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *rpc.RPCError   `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.NotNil(t, rpcResp.Error, "expected RPC error for method %s but got result: %s", method, rpcResp.Result)
	return rpcResp.Error
}
