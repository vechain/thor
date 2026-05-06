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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// newFullServer assembles all sub-packages onto a single Dispatcher and returns
// an httptest.Server. Used only by integration_test.go for dispatch-level tests
// that need the full method table to be reachable.
func newFullServer(t *testing.T, fx *testutil.ChainFixture) *httptest.Server {
	t.Helper()
	pool := testutil.DefaultPool(t, fx.Chain, &fx.Forks)
	d := rpc.NewDispatcher()
	rpcchain.New(fx.Chain.Repo(), fx.ChainID, "test/1.0").Mount(d)
	blocks.New(fx.Chain.Repo(), fx.ChainID).Mount(d)
	transactions.New(fx.Chain.Repo(), fx.ChainID, pool).Mount(d)
	accounts.New(fx.Chain.Repo(), fx.Chain.Stater()).Mount(d)
	logs.New(fx.Chain.Repo(), fx.Chain.LogDB(), 100).Mount(d)
	fees.New(fx.Chain.Repo(), 100).Mount(d)
	simulation.New(fx.Chain.Repo(), fx.Chain.Stater(), &fx.Forks, 1_000_000).Mount(d)
	ts := httptest.NewServer(rpc.New(d))
	t.Cleanup(ts.Close)
	return ts
}

// TestDispatch covers server- and dispatcher-level behaviour that is independent
// of any individual method namespace. Per-method tests live in each sub-package.
func TestDispatch(t *testing.T) {
	fx := testutil.NewChainFixture(t)
	ts := newFullServer(t, fx)

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
		for i := 0; i < 11; i++ {
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

	t.Run("wrong_http_method", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}
