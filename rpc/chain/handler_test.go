// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/rpc/chain"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
)

type fixture struct {
	chain   *testchain.Chain
	chainID uint64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	require.NoError(t, c.MintBlock())
	return &fixture{
		chain:   c,
		chainID: c.Repo().ChainID(),
	}
}

func TestChainHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, chain.New(fx.chain.Repo(), fx.chainID, "test/1.0"))

	t.Run("eth_chainId", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_chainId", []any{})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, fx.chainID, uint64(got))
	})

	t.Run("net_version", func(t *testing.T) {
		// net_version returns the chain ID as a decimal string.
		result := testutil.Call(t, ts, "net_version", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.NotEmpty(t, got)
	})

	t.Run("web3_clientVersion", func(t *testing.T) {
		result := testutil.Call(t, ts, "web3_clientVersion", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "Thor/test/1.0", got)
	})

	t.Run("eth_blockNumber", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_blockNumber", []any{})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(1), uint64(got))
	})

	t.Run("eth_syncing", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_syncing", []any{})
		var got bool
		require.NoError(t, json.Unmarshal(result, &got))
		assert.False(t, got)
	})

	t.Run("eth_accounts", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_accounts", []any{})
		var got []string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	t.Run("eth_mining", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_mining", []any{})
		var got bool
		require.NoError(t, json.Unmarshal(result, &got))
		assert.False(t, got)
	})

	t.Run("eth_hashrate", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_hashrate", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "0x0", got)
	})
}
