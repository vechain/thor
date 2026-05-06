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
)

func TestLogsHandler(t *testing.T) {
	fx := testutil.NewChainFixture(t)
	ts := testutil.NewMinimalServer(t, logs.New(fx.Chain.Repo(), fx.Chain.LogDB(), 100))

	t.Run("eth_getLogs_empty", func(t *testing.T) {
		// The fixture ETH tx is a plain VET transfer — it emits no contract events.
		// eth_getLogs therefore returns an empty array.
		//
		// TODO: extend ChainFixture with a contract-deploy tx that emits events so we
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
			map[string]any{"blockHash": fx.BlockHash},
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
