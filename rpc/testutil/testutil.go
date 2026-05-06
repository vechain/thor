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
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// ChainFixture holds a fully initialised test chain with one minted block.
// Block layout: position 0 = VeChain TypeLegacy tx, position 1 = Ethereum EIP-1559 tx.
// The EIP-1559 tx has projected ETH index 0 (it is the only ETH tx in the block).
type ChainFixture struct {
	Chain     *testchain.Chain
	Forks     thor.ForkConfig
	ChainID   uint64
	Sender    genesis.DevAccount // DevAccounts()[0] — sent both txs, nonce incremented to 1
	Recipient genesis.DevAccount // DevAccounts()[1] — received VET from both txs
	EthTx     *tx.Transaction    // Ethereum EIP-1559 tx at canonical index 1
	VcTx      *tx.Transaction    // VeChain TypeLegacy tx at canonical index 0
	BlockHash string             // 0x-prefixed 66-char hex of block 1
	EthTxHash string             // ETH tx ID (= Keccak256 of raw wire bytes)
	VcTxHash  string             // VeChain tx ID
}

// NewChainFixture creates a standard test chain ready for RPC tests:
//   - ForkConfig{} — all forks active from block 0 (GALACTICA, INTERSTELLAR, …)
//   - Genesis block + 1 minted block containing one VeChain tx and one ETH tx
func NewChainFixture(t *testing.T) *ChainFixture {
	t.Helper()

	// Disable PoS transition so tests don't have to deal with HayabusaTP complexity.
	hayabusaTP := uint32(math.MaxUint32)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	forks := thor.ForkConfig{}
	thorChain, err := testchain.NewWithFork(&forks, 180)
	require.NoError(t, err)

	chainID := thor.GetEthChainID(thorChain.GenesisBlock().Header().ID())

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	// VeChain TypeLegacy tx — canonical position 0.
	vcTxTo := recipient.Address
	vcTx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		BlockRef(tx.NewBlockRef(thorChain.Repo().BestBlockSummary().Header.Number())).
		Expiration(1000).
		GasPriceCoef(255).
		Gas(21000).
		Nonce(datagen.RandUint64()).
		Clause(tx.NewClause(&vcTxTo).WithValue(big.NewInt(1e9))).
		Build()
	vcTx = tx.MustSign(vcTx, sender.PrivateKey)

	// Ethereum EIP-1559 tx — canonical position 1, projected ETH index 0.
	ethTx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainID(chainID).
		Nonce(0).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		GasLimit(21000).
		To(&vcTxTo).
		Value(big.NewInt(1e9)).
		Build(sender.PrivateKey)
	require.NoError(t, err)

	require.NoError(t, thorChain.MintBlock(vcTx, ethTx))
	require.Equal(t, uint32(1), thorChain.Repo().BestBlockSummary().Header.Number())

	bestBlock, err := thorChain.BestBlock()
	require.NoError(t, err)

	return &ChainFixture{
		Chain:     thorChain,
		Forks:     forks,
		ChainID:   chainID,
		Sender:    sender,
		Recipient: recipient,
		EthTx:     ethTx,
		VcTx:      vcTx,
		BlockHash: bestBlock.Header().ID().String(),
		EthTxHash: ethTx.ID().String(),
		VcTxHash:  vcTx.ID().String(),
	}
}

// Mounter is satisfied by any sub-package handler that exposes Mount.
type Mounter interface {
	Mount(d *rpc.Dispatcher)
}

// NewMinimalServer creates an httptest.Server with only m's methods registered.
// Sub-package tests use this for focused isolation — only the handler under test
// is mounted, so an accidental call to another namespace fails with method-not-found.
func NewMinimalServer(t *testing.T, m Mounter) *httptest.Server {
	t.Helper()
	d := rpc.NewDispatcher()
	m.Mount(d)
	ts := httptest.NewServer(rpc.New(d))
	t.Cleanup(ts.Close)
	return ts
}

// DefaultPool creates a txpool suitable for testing and registers t.Cleanup(pool.Close).
func DefaultPool(t *testing.T, c *testchain.Chain, forks *thor.ForkConfig) txpool.Pool {
	t.Helper()
	p := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, forks)
	t.Cleanup(p.Close)
	return p
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

	resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader(body))
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

	resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader(body))
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
