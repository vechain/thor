// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"

	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/accounts"
	"github.com/vechain/thor/v2/rpc/blocks"
	rpcchain "github.com/vechain/thor/v2/rpc/chain"
	"github.com/vechain/thor/v2/rpc/fees"
	"github.com/vechain/thor/v2/rpc/logs"
	"github.com/vechain/thor/v2/rpc/simulation"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/rpc/transactions"
)

// TestDispatch covers server- and dispatcher-level behaviour that is independent
// of any individual method namespace. Per-method tests live in each sub-package.
func TestDispatch(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := thor.GetEthChainID(c.GenesisBlock().Header().ID())
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	vcTx := testutil.BuildVcTx(t, c, sender, &recipient.Address)
	ethTx := testutil.BuildEthTx(t, chainID, sender, 0, &recipient.Address)

	require.NoError(t, c.MintBlock(vcTx, ethTx))
	require.Equal(t, uint32(1), c.Repo().BestBlockSummary().Header.Number())

	pool := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	srv := rpc.NewServer()
	rpcchain.New(c.Repo(), chainID, "test/1.0").Mount(srv)
	blocks.New(c.Repo(), chainID).Mount(srv)
	transactions.New(c.Repo(), chainID, pool).Mount(srv)
	accounts.New(c.Repo(), c.Stater()).Mount(srv)
	logs.New(c.Repo(), c.LogDB(), 100).Mount(srv)
	fees.New(c.Repo(), 100).Mount(srv)
	simulation.New(c.Repo(), c.Stater(), &testchain.DefaultForkConfig, 1_000_000).Mount(srv)

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	t.Run("unknown_method", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_nonExistentMethod", []any{})
		assert.Equal(t, rpc.CodeMethodNotFound, rpcErr.Code)
	})

	t.Run("batch", func(t *testing.T) {
		batchBody := `[
			{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]},
			{"jsonrpc":"2.0","id":2,"method":"eth_syncing","params":[]}
		]`
		resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader([]byte(batchBody)))
		require.NoError(t, err)
		defer resp.Body.Close()

		var responses []struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *rpc.RPCError   `json:"error"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&responses))
		assert.Len(t, responses, 2)
		for _, r := range responses {
			assert.Nil(t, r.Error, "batch element should not have an error")
		}
	})

	t.Run("batch_exceeds_limit", func(t *testing.T) {
		// Build a batch of 11 requests (maxBatchRequests = 10).
		var batch []map[string]any
		for i := range 11 {
			batch = append(batch, map[string]any{
				"jsonrpc": "2.0",
				"id":      i + 1,
				"method":  "eth_blockNumber",
				"params":  []any{},
			})
		}
		batchBody, _ := json.Marshal(batch)
		resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader(batchBody))
		require.NoError(t, err)
		defer resp.Body.Close()

		var rpcResp struct {
			Error *rpc.RPCError `json:"error"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
		require.NotNil(t, rpcResp.Error)
		assert.Equal(t, rpc.CodeInvalidParams, rpcResp.Error.Code)
	})

	t.Run("invalid_json", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader([]byte("{invalid")))
		require.NoError(t, err)
		defer resp.Body.Close()

		var rpcResp struct {
			Error *rpc.RPCError `json:"error"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
		require.NotNil(t, rpcResp.Error)
		assert.Equal(t, rpc.CodeParseError, rpcResp.Error.Code)
	})

	t.Run("body_too_large", func(t *testing.T) {
		// Send a body that exceeds the 2 MB server limit.
		oversized := make([]byte, 2*1024*1024+1)
		for i := range oversized {
			oversized[i] = 'x'
		}
		resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader(oversized))
		require.NoError(t, err)
		defer resp.Body.Close()

		var rpcResp struct {
			Error *rpc.RPCError `json:"error"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
		require.NotNil(t, rpcResp.Error)
		assert.Equal(t, rpc.CodeInvalidRequest, rpcResp.Error.Code)
	})

	t.Run("wrong_http_method", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}
