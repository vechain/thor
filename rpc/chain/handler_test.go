// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/rpc/chain"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
)

// fakeSyncer satisfies chain.Syncer. Default state is "not synced".
type fakeSyncer struct {
	syncedCh chan struct{}
	highest  atomic.Uint32
}

func newFakeSyncer() *fakeSyncer { return &fakeSyncer{syncedCh: make(chan struct{})} }

func (f *fakeSyncer) Synced() <-chan struct{}  { return f.syncedCh }
func (f *fakeSyncer) HighestPeerBlock() uint32 { return f.highest.Load() }
func (f *fakeSyncer) markSynced() {
	select {
	case <-f.syncedCh:
	default:
		close(f.syncedCh)
	}
}
func (f *fakeSyncer) setHighest(n uint32) { f.highest.Store(n) }

type fixture struct {
	chain  *testchain.Chain
	syncer *fakeSyncer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	require.NoError(t, c.MintBlock())
	syncer := newFakeSyncer()
	syncer.markSynced() // default to synced so legacy assertions stay valid
	return &fixture{
		chain:  c,
		syncer: syncer,
	}
}

func TestChainHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, chain.New(fx.chain.Repo(), "test/1.0", fx.syncer))

	t.Run("eth_chainId", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_chainId", []any{})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, fx.chain.ChainID(), uint64(got))
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

	t.Run("net_listening", func(t *testing.T) {
		result := testutil.Call(t, ts, "net_listening", []any{})
		var got bool
		require.NoError(t, json.Unmarshal(result, &got))
		assert.True(t, got)
	})

	t.Run("net_peerCount", func(t *testing.T) {
		result := testutil.Call(t, ts, "net_peerCount", []any{})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(0), uint64(got))
	})

	t.Run("eth_coinbase", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_coinbase", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "0x0000000000000000000000000000000000000000", got)
	})
}

// TestEthSyncingInProgress verifies that eth_syncing returns a sync-progress
// object (not `false`) while the node has not finished initial synchronisation.
// Mirrors go-ethereum's PublicEthereumAPI.Syncing return shape: the inner
// SyncProgress fields, not the {syncing, status} envelope used by the WS
// subscription.
func TestEthSyncingInProgress(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)
	require.NoError(t, c.MintBlock())

	syncer := newFakeSyncer() // not marked synced
	syncer.setHighest(4242)   // a peer claims block 4242

	ts := testutil.NewTestServer(t, chain.New(c.Repo(), "test/1.0", syncer))

	result := testutil.Call(t, ts, "eth_syncing", []any{})

	var got struct {
		StartingBlock string `json:"startingBlock"`
		CurrentBlock  string `json:"currentBlock"`
		HighestBlock  string `json:"highestBlock"`
	}
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x1", got.StartingBlock) // chain minted 1 block before handler creation
	assert.Equal(t, "0x1", got.CurrentBlock)
	assert.Equal(t, "0x1092", got.HighestBlock) // 4242 in hex
}
