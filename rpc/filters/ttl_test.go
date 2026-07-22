// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package filters

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/txpool"
)

// TestTTLExpiration verifies that filter entries are cleaned up after the TTL
// period expires via evictExpired.
func TestTTLExpiration(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	pool := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	defer pool.Close()

	// Save the original TTL constant and restore after test.
	// Since filterTTL is a const we can't modify it, so we create
	// the handler and manually trigger evictExpired after sleeping.
	h := New(c.Repo(), pool, 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	// Create filters of each type.
	idResult := testutil.Call(t, ts, "eth_newBlockFilter", []any{})
	var blockFilterID string
	require.NoError(t, json.Unmarshal(idResult, &blockFilterID))

	idResult = testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{}})
	var logFilterID string
	require.NoError(t, json.Unmarshal(idResult, &logFilterID))

	idResult = testutil.Call(t, ts, "eth_newPendingTransactionFilter", []any{})
	var pendingFilterID string
	require.NoError(t, json.Unmarshal(idResult, &pendingFilterID))

	// All three should exist in the entries map.
	require.Contains(t, h.entries, blockFilterID)
	require.Contains(t, h.entries, logFilterID)
	require.Contains(t, h.entries, pendingFilterID)

	// Manually age the entries by backdating lastPoll to exceed the TTL.
	h.mu.Lock()
	for _, e := range h.entries {
		e.lastPoll = time.Now().Add(-(filterTTL + time.Second))
	}
	h.mu.Unlock()

	// Trigger eviction directly (avoids waiting for runTTL's ticker).
	h.evictExpired()

	// All three should have been removed.
	require.NotContains(t, h.entries, blockFilterID)
	require.NotContains(t, h.entries, logFilterID)
	require.NotContains(t, h.entries, pendingFilterID)
}
