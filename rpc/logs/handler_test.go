// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logs_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/rpc/logs"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
)

type fixture struct {
	chain     *testchain.Chain
	blockHash string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	require.NoError(t, c.MintBlock())
	bestBlock, err := c.BestBlock()
	require.NoError(t, err)
	return &fixture{
		chain:     c,
		blockHash: bestBlock.Header().ID().String(),
	}
}

func TestLogsHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, logs.New(fx.chain.Repo(), fx.chain.LogDB(), 100))

	t.Run("eth_getLogs_empty", func(t *testing.T) {
		// The fixture block contains no contract events.
		// eth_getLogs therefore returns an empty array.
		//
		// TODO: extend with a contract-deploy tx that emits events so we
		// can assert on non-empty log results (address filter, topic filter, EIP-234
		// blockHash filter, etc.).
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	t.Run("eth_getLogs_blockHash_filter", func(t *testing.T) {
		// EIP-234: single-block query via blockHash.
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"blockHash": fx.blockHash},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	t.Run("eth_getLogs_range_exceeds_backtrace", func(t *testing.T) {
		// A range wider than the backtrace limit (100) must be rejected.
		rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "0x65"}, // 0x65 = 101
		})
		assert.NotNil(t, rpcErr)
	})
}
