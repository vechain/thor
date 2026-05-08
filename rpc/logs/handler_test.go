// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logs_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
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
	ts := testutil.NewTestServer(t, logs.New(fx.chain.Repo(), fx.chain.LogDB(), 100, 1000))

	t.Run("eth_getLogs_empty", func(t *testing.T) {
		// The fixture block contains no ETH typed transactions → no events.
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

// TestLogsHandlerWithEvents verifies that eth_getLogs returns events emitted by
// ETH typed transactions — including address and topic filters.
func TestLogsHandlerWithEvents(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	// Mint a block with an ETH call to Energy.transfer, which emits a Transfer event.
	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address
	ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(ethCallTx))

	bestBlock, err := c.BestBlock()
	require.NoError(t, err)
	blockHash := bestBlock.Header().ID().String()
	txHash := ethCallTx.ID().String()

	ts := testutil.NewTestServer(t, logs.New(c.Repo(), c.LogDB(), 100, 1000))

	transferEvent, ok := builtin.Energy.ABI.EventByName("Transfer")
	require.True(t, ok)
	transferTopic := common.Hash(transferEvent.ID()).Hex()

	t.Run("eth_getLogs_range_returns_event", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
		})
		var got []map[string]any
		require.NoError(t, json.Unmarshal(result, &got))
		require.Len(t, got, 1, "Energy.transfer emits one Transfer event")

		addr, _ := got[0]["address"].(string)
		assert.Equal(t, energyAddr.String(), addr)

		topics, _ := got[0]["topics"].([]any)
		require.NotEmpty(t, topics)
		assert.Equal(t, transferTopic, topics[0])
	})

	t.Run("eth_getLogs_log_fields", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
		})
		var got []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &got))
		require.Len(t, got, 1)
		log := got[0]

		var blockNum hexutil.Uint64
		require.NoError(t, json.Unmarshal(log["blockNumber"], &blockNum))
		assert.Equal(t, uint64(1), uint64(blockNum))

		var gotTxHash common.Hash
		require.NoError(t, json.Unmarshal(log["transactionHash"], &gotTxHash))
		assert.Equal(t, txHash, gotTxHash.Hex())

		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(log["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(0), uint64(txIdx))

		var gotBlockHash common.Hash
		require.NoError(t, json.Unmarshal(log["blockHash"], &gotBlockHash))
		assert.Equal(t, blockHash, gotBlockHash.Hex())

		var logIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(log["logIndex"], &logIdx))
		assert.Equal(t, uint64(0), uint64(logIdx))

		var removed bool
		require.NoError(t, json.Unmarshal(log["removed"], &removed))
		assert.False(t, removed)

		var data hexutil.Bytes
		require.NoError(t, json.Unmarshal(log["data"], &data))
		assert.Greater(t, len(data), 0, "ABI-encoded transfer amount should be non-empty")
	})

	t.Run("eth_getLogs_blockHash_with_events", func(t *testing.T) {
		// EIP-234: query by blockHash on a block that contains events.
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"blockHash": blockHash},
		})
		var got []map[string]any
		require.NoError(t, json.Unmarshal(result, &got))
		require.Len(t, got, 1)
		addr, _ := got[0]["address"].(string)
		assert.Equal(t, energyAddr.String(), addr)
	})

	t.Run("eth_getLogs_address_filter", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"address":   energyAddr.String(),
			},
		})
		var got []map[string]any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1)
	})

	t.Run("eth_getLogs_multi_address_filter", func(t *testing.T) {
		// Array-of-addresses form: one matching address, one non-matching.
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"address":   []string{energyAddr.String(), "0x0000000000000000000000000000000000000001"},
			},
		})
		var got []map[string]any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1)
	})

	t.Run("eth_getLogs_topic_filter", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"topics":    []any{transferTopic},
			},
		})
		var got []map[string]any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1)
	})

	t.Run("eth_getLogs_topic_null_wildcard", func(t *testing.T) {
		// Per the Ethereum spec, null at a topic position is a wildcard.
		// The ERC-20 Transfer event has topic1 = the from address (indexed).
		// Filtering [null, senderTopic] means: any topic0, topic1 must match sender.
		senderTopic := common.BytesToHash(sender.Address[:]).Hex()
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"topics":    []any{nil, senderTopic},
			},
		})
		var got []map[string]any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1, "null wildcard at topic0 should still match the Transfer event")
	})

	t.Run("eth_getLogs_address_mismatch_returns_empty", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"address":   "0x0000000000000000000000000000000000000001",
			},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})
}

// TestLogsHandlerVcTxsExcluded verifies that events from TypeLegacy VeChain
// transactions are not returned by eth_getLogs, even though they are stored in logdb.
func TestLogsHandlerVcTxsExcluded(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address

	// Build a TypeLegacy VeChain tx that calls Energy.transfer (emits a Transfer event).
	vcCallTx := testutil.BuildVcCallTx(t, c, sender, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(vcCallTx))

	ts := testutil.NewTestServer(t, logs.New(c.Repo(), c.LogDB(), 100, 1000))

	result := testutil.Call(t, ts, "eth_getLogs", []any{
		map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
	})
	var got []any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Empty(t, got, "events from TypeLegacy VeChain txs must not appear in eth_getLogs")
}

// TestLogsHandlerInvalidParams verifies that malformed filter fields produce RPC errors.
func TestLogsHandlerInvalidParams(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, logs.New(fx.chain.Repo(), fx.chain.LogDB(), 100, 1000))

	t.Run("eth_getLogs_invalid_address", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"address":   "0xinvalid",
			},
		})
		assert.NotNil(t, rpcErr)
	})

	t.Run("eth_getLogs_invalid_topic", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"topics":    []any{"0xinvalid"},
			},
		})
		assert.NotNil(t, rpcErr)
	})
}

// TestLogsHandlerProjectedIndices verifies that transactionIndex and logIndex in
// eth_getLogs responses are projected relative to ETH-typed transactions only, not
// the canonical VeChain block position. In a block with [vcTx, ethTx], the ethTx's
// event must have transactionIndex=0 and logIndex=0, not 1 and 1.
func TestLogsHandlerProjectedIndices(t *testing.T) {
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

	// Mint a mixed block: VeChain tx first (canonical idx 0), ETH tx second (canonical idx 1).
	// The VeChain tx emits a Transfer event (stored in logdb as txIndex=0, logIndex=0).
	// The ETH tx emits a Transfer event (stored in logdb as txIndex=1, logIndex=1).
	vcCallTx := testutil.BuildVcCallTx(t, c, sender, &energyAddr, callData, 200_000)
	ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(vcCallTx, ethCallTx))

	ts := testutil.NewTestServer(t, logs.New(c.Repo(), c.LogDB(), 100, 1000))

	result := testutil.Call(t, ts, "eth_getLogs", []any{
		map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
	})
	var got []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &got))
	require.Len(t, got, 1, "only the ETH tx's event should be returned")

	var txIdx hexutil.Uint64
	require.NoError(t, json.Unmarshal(got[0]["transactionIndex"], &txIdx))
	assert.Equal(t, uint64(0), uint64(txIdx),
		"transactionIndex must be the projected ETH index (0), not the canonical VeChain index (1)")

	var logIdx hexutil.Uint64
	require.NoError(t, json.Unmarshal(got[0]["logIndex"], &logIdx))
	assert.Equal(t, uint64(0), uint64(logIdx),
		"logIndex must be the projected ETH log index (0), not the VeChain block-wide count (1)")
}

// TestLogsHandlerFromBlockDefault verifies that an absent fromBlock defaults to
// "latest" per the Ethereum spec, not to block 0 (genesis).
func TestLogsHandlerFromBlockDefault(t *testing.T) {
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

	// Block 1: contains an ETH event.
	ethCallTx := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(ethCallTx))

	// Block 2: empty — no transactions, no events.
	require.NoError(t, c.MintBlock())

	ts := testutil.NewTestServer(t, logs.New(c.Repo(), c.LogDB(), 100, 1000))

	t.Run("absent_fromBlock_defaults_to_latest", func(t *testing.T) {
		// No fromBlock → defaults to latest (block 2). Block 2 has no events.
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"toBlock": "latest"},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got, "absent fromBlock should default to latest (block 2), which has no events")
	})

	t.Run("explicit_fromBlock_reaches_earlier_event", func(t *testing.T) {
		// Explicit fromBlock=1 includes block 1 where the ETH event lives.
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x1", "toBlock": "latest"},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1, "explicit fromBlock=1 should reach the event in block 1")
	})
}

// TestLogsHandlerLogsLimit verifies that when logdb returns more rows than logsLimit,
// eth_getLogs returns an explicit error instead of silently truncating the result.
func TestLogsHandlerLogsLimit(t *testing.T) {
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

	// Mint two separate ETH call txs so there are 2 events in logdb.
	ethCallTx1 := testutil.BuildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	ethCallTx2 := testutil.BuildEthCallTx(t, chainID, genesis.DevAccounts()[2], 0, &energyAddr, callData, 200_000)
	require.NoError(t, c.MintBlock(ethCallTx1, ethCallTx2))

	// logsLimit=1: querying 2 events exceeds the limit.
	ts := testutil.NewTestServer(t, logs.New(c.Repo(), c.LogDB(), 100, 1))

	rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
		map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
	})
	assert.NotNil(t, rpcErr, "should return an error when result exceeds logsLimit")
}

// TestLogsHandlerEIP234MutualExclusion verifies that passing blockHash together with
// fromBlock or toBlock is rejected with an InvalidParams error, per EIP-234.
func TestLogsHandlerEIP234MutualExclusion(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, logs.New(fx.chain.Repo(), fx.chain.LogDB(), 100, 1000))

	t.Run("blockHash_with_fromBlock", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
			map[string]any{
				"blockHash": fx.blockHash,
				"fromBlock": "0x0",
			},
		})
		assert.Equal(t, -32602, rpcErr.Code, "should be InvalidParams")
	})

	t.Run("blockHash_with_toBlock", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
			map[string]any{
				"blockHash": fx.blockHash,
				"toBlock":   "latest",
			},
		})
		assert.Equal(t, -32602, rpcErr.Code, "should be InvalidParams")
	})
}

// TestLogsHandlerUnknownBlockHash verifies that an unresolvable blockHash returns a
// server error (matching go-ethereum semantics), not a silent empty result.
func TestLogsHandlerUnknownBlockHash(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, logs.New(fx.chain.Repo(), fx.chain.LogDB(), 100, 1000))

	rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
		map[string]any{"blockHash": "0x0000000000000000000000000000000000000000000000000000000000000001"},
	})
	assert.Equal(t, -32000, rpcErr.Code, "unknown blockHash should return server error (-32000)")
}

// TestLogsHandlerReversedRange verifies that fromBlock > toBlock returns an InvalidParams
// error (matching go-ethereum semantics), not a silent empty or DB over-scan.
func TestLogsHandlerReversedRange(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)
	require.NoError(t, c.MintBlock()) // advance to block 1
	ts := testutil.NewTestServer(t, logs.New(c.Repo(), c.LogDB(), 100, 1000))

	rpcErr := testutil.CallExpectError(t, ts, "eth_getLogs", []any{
		map[string]any{"fromBlock": "0x1", "toBlock": "0x0"},
	})
	assert.Equal(t, -32602, rpcErr.Code, "fromBlock > toBlock should return InvalidParams (-32602)")
}
