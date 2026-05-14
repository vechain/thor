// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package filters_test

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/filters"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/txpool"
)

type fixture struct {
	chain   *testchain.Chain
	chainID uint64
	pool    *txpool.TxPool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	pool := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	t.Cleanup(pool.Close)

	return &fixture{
		chain:   c,
		chainID: c.Repo().ChainID(),
		pool:    pool,
	}
}

func TestFiltersHandler(t *testing.T) {
	fx := newFixture(t)
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	h := filters.New(fx.chain.Repo(), fx.pool, 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	t.Run("block_filter_empty_then_new_block", func(t *testing.T) {
		idResult := testutil.Call(t, ts, "eth_newBlockFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))
		assert.Regexp(t, `^0x[0-9a-f]+$`, filterID)

		// No new blocks yet — returns empty array (not null).
		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var hashes []common.Hash
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Empty(t, hashes)

		// Mint a block, then poll — returns the new block hash.
		require.NoError(t, fx.chain.MintBlock())
		result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		require.NoError(t, json.Unmarshal(result, &hashes))
		require.Len(t, hashes, 1)

		best, err := fx.chain.BestBlock()
		require.NoError(t, err)
		assert.Equal(t, common.Hash(best.Header().ID()), hashes[0])

		// Second poll with no new blocks — empty again.
		result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Empty(t, hashes)
	})

	t.Run("log_filter_no_events", func(t *testing.T) {
		// Mint a block with a plain ETH transfer (no contract events).
		ethTx := testutil.BuildEthTx(t, fx.chainID, sender, 0, &recipient.Address)
		require.NoError(t, fx.chain.MintBlock(ethTx))

		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		// No events in a plain transfer — returns empty array.
		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Empty(t, logs)
	})

	t.Run("log_filter_invalid_criteria", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_newFilter", []any{
			map[string]any{"address": "not-a-valid-address"},
		})
		assert.Equal(t, jsonrpc.CodeInvalidParams, rpcErr.Code)
	})

	t.Run("eth_getFilterLogs", func(t *testing.T) {
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
		})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		// No contract events in the chain — returns empty array (not null).
		result := testutil.Call(t, ts, "eth_getFilterLogs", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Empty(t, logs)
	})

	t.Run("eth_getFilterLogs_block_filter_error", func(t *testing.T) {
		idResult := testutil.Call(t, ts, "eth_newBlockFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		rpcErr := testutil.CallExpectError(t, ts, "eth_getFilterLogs", []any{filterID})
		assert.Equal(t, jsonrpc.CodeInvalidParams, rpcErr.Code)
	})

	t.Run("pending_tx_filter", func(t *testing.T) {
		idResult := testutil.Call(t, ts, "eth_newPendingTransactionFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		// No pending txs yet — returns empty array.
		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var hashes []common.Hash
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Empty(t, hashes)

		// Add an ETH tx to the pool. SubscribeTxEvent fires synchronously before
		// AddLocal returns, so the hash is immediately available for polling.
		ethTx := testutil.BuildEthTx(t, fx.chainID, sender, 10, &recipient.Address)
		require.NoError(t, fx.pool.AddLocal(ethTx))

		result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		require.NoError(t, json.Unmarshal(result, &hashes))
		require.Len(t, hashes, 1)
		assert.Equal(t, common.Hash(ethTx.ID()), hashes[0])

		// Drained — second poll is empty.
		result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Empty(t, hashes)
	})

	t.Run("eth_uninstallFilter_existing", func(t *testing.T) {
		idResult := testutil.Call(t, ts, "eth_newBlockFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		result := testutil.Call(t, ts, "eth_uninstallFilter", []any{filterID})
		var ok bool
		require.NoError(t, json.Unmarshal(result, &ok))
		assert.True(t, ok)
	})

	t.Run("eth_uninstallFilter_unknown", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_uninstallFilter", []any{"0x9999"})
		var ok bool
		require.NoError(t, json.Unmarshal(result, &ok))
		assert.False(t, ok)
	})

	t.Run("eth_getFilterChanges_unknown", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_getFilterChanges", []any{"0x9999"})
		assert.Equal(t, jsonrpc.CodeInvalidParams, rpcErr.Code)
	})

	t.Run("eth_getFilterLogs_unknown", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_getFilterLogs", []any{"0x9999"})
		assert.Equal(t, jsonrpc.CodeInvalidParams, rpcErr.Code)
	})
}

// newTestPool creates a txpool and registers its cleanup.
func newTestPool(t *testing.T, c *testchain.Chain) *txpool.TxPool {
	t.Helper()
	pool := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	t.Cleanup(pool.Close)
	return pool
}

// TestFiltersHandlerLogChangesWithEvents verifies that eth_getFilterChanges for a log
// filter returns actual ETH-typed transaction events and drains on a second poll.
func TestFiltersHandlerLogChangesWithEvents(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	transferEvent, ok := builtin.Energy.ABI.EventByName("Transfer")
	require.True(t, ok)
	transferTopic := common.Hash(transferEvent.ID()).Hex()

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	// Create a log filter at genesis — no criteria matches all events.
	idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{}})
	var filterID string
	require.NoError(t, json.Unmarshal(idResult, &filterID))

	// No new blocks yet — empty.
	result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
	var initial []any
	require.NoError(t, json.Unmarshal(result, &initial))
	assert.Empty(t, initial)

	// Mint a block with an ETH contract call that emits a Transfer event.
	ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(ethCallTx))

	// Poll — should return the Transfer event.
	result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
	var logs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &logs))
	require.Len(t, logs, 1)

	var addr string
	require.NoError(t, json.Unmarshal(logs[0]["address"], &addr))
	assert.Equal(t, energyAddr.String(), addr)

	var topics []string
	require.NoError(t, json.Unmarshal(logs[0]["topics"], &topics))
	require.NotEmpty(t, topics)
	assert.Equal(t, transferTopic, topics[0])

	// Second poll with no new blocks — empty (changes drain).
	result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
	var drained []any
	require.NoError(t, json.Unmarshal(result, &drained))
	assert.Empty(t, drained)
}

// TestFiltersHandlerLogChangesAddressFilter verifies that address criteria in a log
// filter correctly include matching events and exclude non-matching ones.
func TestFiltersHandlerLogChangesAddressFilter(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	t.Run("matching_address_returns_event", func(t *testing.T) {
		sender := genesis.DevAccounts()[0]
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"address": energyAddr.String(),
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
		require.NoError(t, c.MintBlock(ethCallTx))

		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Len(t, logs, 1)
	})

	t.Run("non_matching_address_returns_empty", func(t *testing.T) {
		// Use a different sender so nonce is 0 regardless of prior subtest.
		sender := genesis.DevAccounts()[2]
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"address": "0x0000000000000000000000000000000000000001",
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		// Energy.transfer emits from energyAddr, not 0x0001 → filter should not match.
		ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
		require.NoError(t, c.MintBlock(ethCallTx))

		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Empty(t, logs)
	})
}

// TestFiltersHandlerGetFilterLogsWithEvents verifies that eth_getFilterLogs returns
// actual events from the stored block range for a log filter.
func TestFiltersHandlerGetFilterLogsWithEvents(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	// Mint the event block first; eth_getFilterLogs re-evaluates the range at query time.
	ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(ethCallTx))

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
		"fromBlock": "0x0",
		"toBlock":   "latest",
	}})
	var filterID string
	require.NoError(t, json.Unmarshal(idResult, &filterID))

	result := testutil.Call(t, ts, "eth_getFilterLogs", []any{filterID})
	var logs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &logs))
	require.Len(t, logs, 1)

	var addr string
	require.NoError(t, json.Unmarshal(logs[0]["address"], &addr))
	assert.Equal(t, energyAddr.String(), addr)
}

// TestFiltersHandlerGetFilterLogsBacktraceLimit verifies that eth_getFilterLogs rejects
// a block range that exceeds the backtrace limit with a server error.
func TestFiltersHandlerGetFilterLogsBacktraceLimit(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	// Build a chain deeper than backtrace=100: fromBlock=0x0, toBlock=latest → range 102 > 100.
	for range 102 {
		require.NoError(t, c.MintBlock())
	}

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
		"fromBlock": "0x0",
		"toBlock":   "latest",
	}})
	var filterID string
	require.NoError(t, json.Unmarshal(idResult, &filterID))

	rpcErr := testutil.CallExpectError(t, ts, "eth_getFilterLogs", []any{filterID})
	assert.Equal(t, jsonrpc.CodeServerError, rpcErr.Code)
}

// TestFiltersHandlerVcTxExcluded verifies that events from TypeLegacy VeChain
// transactions do not appear in log filter changes.
func TestFiltersHandlerVcTxExcluded(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{}})
	var filterID string
	require.NoError(t, json.Unmarshal(idResult, &filterID))

	// Mint a block with a TypeLegacy VeChain tx — emits a Transfer event but is not ETH-typed.
	vcCallTx := testutil.BuildVcCallTx(t, c, sender, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(vcCallTx))

	result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
	var logs []any
	require.NoError(t, json.Unmarshal(result, &logs))
	assert.Empty(t, logs, "TypeLegacy VeChain tx events must not appear in log filter changes")
}

// TestFiltersHandlerBlockFilterMultipleBlocks verifies that polling a block filter
// after minting several blocks returns all new hashes in a single call.
func TestFiltersHandlerBlockFilterMultipleBlocks(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	idResult := testutil.Call(t, ts, "eth_newBlockFilter", []any{})
	var filterID string
	require.NoError(t, json.Unmarshal(idResult, &filterID))

	require.NoError(t, c.MintBlock())
	require.NoError(t, c.MintBlock())
	require.NoError(t, c.MintBlock())

	// Single poll returns all 3 hashes.
	result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
	var hashes []common.Hash
	require.NoError(t, json.Unmarshal(result, &hashes))
	assert.Len(t, hashes, 3)

	// Second poll with no new blocks — empty.
	result = testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
	require.NoError(t, json.Unmarshal(result, &hashes))
	assert.Empty(t, hashes)
}

// TestFiltersHandlerCompactTopicFilter verifies that compact topic hex like "0x0"
// is accepted by eth_newFilter and correctly matches events.
func TestFiltersHandlerCompactTopicFilter(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	transferEvent, ok := builtin.Energy.ABI.EventByName("Transfer")
	require.True(t, ok)
	transferTopic := common.Hash(transferEvent.ID()).Hex()

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	t.Run("compact_zero_topic_creates_filter", func(t *testing.T) {
		// "0x0" is compact hex for zero-topic — should not reject the filter.
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"topics": []any{"0x0"},
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))
		assert.NotEmpty(t, filterID)
	})

	t.Run("compact_topic_matches_correctly", func(t *testing.T) {
		// Create filter with a zero-topic filter — uses compact "0x0" form.
		// This verifies that compact hex is parsed without error.
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"topics": []any{"0x0"},
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		// Mint a block with the Transfer event (topic[0] is the event signature, not zero).
		ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
		require.NoError(t, c.MintBlock(ethCallTx))

		// Should not match because the event topic[0] is the Transfer event ID, not zero.
		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Empty(t, logs)
	})

	t.Run("full_topic_compact_prefix_matches", func(t *testing.T) {
		// Different sender so nonce 0 is fresh.
		sender := genesis.DevAccounts()[2]

		// Use the full topic hex — ParseBytes32Compact handles both compact and full.
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"topics": []any{transferTopic},
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
		require.NoError(t, c.MintBlock(ethCallTx))

		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Len(t, logs, 1)
	})
}

// TestFiltersHandlerORTopicFilter verifies that an array at a topic position is treated as
// OR: topics: [["A","B"]] matches logs whose topic0 equals A OR B.
func TestFiltersHandlerORTopicFilter(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	transferEvent, ok := builtin.Energy.ABI.EventByName("Transfer")
	require.True(t, ok)
	transferTopic := common.Hash(transferEvent.ID()).Hex()
	noMatchTopic := "0x0000000000000000000000000000000000000000000000000000000000000001"

	h := filters.New(c.Repo(), newTestPool(t, c), 100)
	t.Cleanup(h.Close)
	ts := testutil.NewTestServer(t, h)

	t.Run("OR_includes_matching_topic", func(t *testing.T) {
		// [transferTopic, noMatchTopic] at position 0 — should match the Transfer event.
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"topics": []any{[]any{transferTopic, noMatchTopic}},
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
		require.NoError(t, c.MintBlock(ethCallTx))

		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Len(t, logs, 1, "OR filter including transferTopic should return the event")
	})

	t.Run("OR_no_matching_topic", func(t *testing.T) {
		// Two non-matching topics OR-ed at position 0 — should return nothing.
		idResult := testutil.Call(t, ts, "eth_newFilter", []any{map[string]any{
			"topics": []any{[]any{noMatchTopic, "0x0000000000000000000000000000000000000000000000000000000000000002"}},
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		// Mint a block with the Transfer event; the filter should not match it.
		ethCallTx := testutil.BuildEthCallTx(t, chainID, genesis.DevAccounts()[2], 0, &energyAddr, callData, 200_000)
		require.NoError(t, c.MintBlock(ethCallTx))

		result := testutil.Call(t, ts, "eth_getFilterChanges", []any{filterID})
		var logs []any
		require.NoError(t, json.Unmarshal(result, &logs))
		assert.Empty(t, logs, "OR filter with no matching topics should return empty")
	})
}
