// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks_test

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/blocks"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/tx"
)

type fixture struct {
	chain     *testchain.Chain
	chainID   uint64
	ethTxHash string
	blockHash string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	vcTx := testutil.BuildVcTx(t, c, sender, &recipient.Address)
	ethTx := testutil.BuildEthTx(t, chainID, sender, 0, &recipient.Address)
	require.NoError(t, c.MintBlock(vcTx, ethTx))

	bestBlock, err := c.BestBlock()
	require.NoError(t, err)
	return &fixture{
		chain:     c,
		chainID:   chainID,
		ethTxHash: ethTx.ID().String(),
		blockHash: bestBlock.Header().ID().String(),
	}
}

func TestBlocksHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, blocks.New(fx.chain.Repo(), fx.chainID))

	t.Run("eth_getBlockByNumber_latest", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"latest", false})
		var blk map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &blk))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(blk["number"], &num))
		assert.Equal(t, uint64(1), uint64(num))

		// Only the ETH tx hash is present; the VeChain legacy tx is excluded.
		var txHashes []string
		require.NoError(t, json.Unmarshal(blk["transactions"], &txHashes))
		require.Len(t, txHashes, 1)
		assert.Equal(t, fx.ethTxHash, txHashes[0])

		// gasUsed counts only the ETH tx.
		var gasUsed hexutil.Uint64
		require.NoError(t, json.Unmarshal(blk["gasUsed"], &gasUsed))
		assert.Greater(t, uint64(gasUsed), uint64(0))

		// baseFeePerGas is present because GALACTICA is active from block 0.
		_, hasBF := blk["baseFeePerGas"]
		assert.True(t, hasBF, "baseFeePerGas should be present for a GALACTICA block")
	})

	t.Run("eth_getBlockByNumber_earliest", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"earliest", false})
		var blk map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &blk))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(blk["number"], &num))
		assert.Equal(t, uint64(0), uint64(num))
	})

	t.Run("eth_getBlockByNumber_hex", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"0x1", false})
		var blk map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &blk))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(blk["number"], &num))
		assert.Equal(t, uint64(1), uint64(num))
	})

	t.Run("eth_getBlockByNumber_notfound", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"0xffff", false})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getBlockByNumber_fullTxs", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"latest", true})
		var blk map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &blk))

		var txObjs []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(blk["transactions"], &txObjs))
		require.Len(t, txObjs, 1)

		var txHash string
		require.NoError(t, json.Unmarshal(txObjs[0]["hash"], &txHash))
		assert.Equal(t, fx.ethTxHash, txHash)

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(txObjs[0]["type"], &txType))
		assert.Equal(t, uint64(tx.TypeEthDynamicFee), uint64(txType))

		// Projected ETH index: the ETH tx is at canonical position 1 but it is
		// the first (and only) ETH tx, so its projected index is 0.
		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(txObjs[0]["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(0), uint64(txIdx))
	})

	t.Run("eth_getBlockByHash", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByHash", []any{fx.blockHash, false})
		var blk map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &blk))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(blk["number"], &num))
		assert.Equal(t, uint64(1), uint64(num))

		var gotHash string
		require.NoError(t, json.Unmarshal(blk["hash"], &gotHash))
		assert.Equal(t, fx.blockHash, gotHash)
	})

	t.Run("eth_getBlockByHash_notfound", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockByHash", []any{"0x0000000000000000000000000000000000000000000000000000000000000001", false})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getBlockTransactionCountByNumber", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockTransactionCountByNumber", []any{"latest"})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(1), uint64(got)) // one ETH tx in the block
	})

	t.Run("eth_getBlockTransactionCountByNumber_notfound", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockTransactionCountByNumber", []any{"0xffff"})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getBlockTransactionCountByHash", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockTransactionCountByHash", []any{fx.blockHash})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(1), uint64(got))
	})

	t.Run("eth_getBlockTransactionCountByHash_notfound", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockTransactionCountByHash", []any{"0x0000000000000000000000000000000000000000000000000000000000000001"})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getBlockReceipts", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockReceipts", []any{"latest"})
		var receipts []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipts))
		require.Len(t, receipts, 1) // one ETH tx receipt
		var txHash string
		require.NoError(t, json.Unmarshal(receipts[0]["transactionHash"], &txHash))
		assert.Equal(t, fx.ethTxHash, txHash)
	})

	t.Run("eth_getBlockReceipts_notfound", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockReceipts", []any{"0xffff"})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getBlockReceipts_empty", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBlockReceipts", []any{"earliest"})
		var receipts []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipts))
		assert.Empty(t, receipts)
	})

	t.Run("eth_getUncleCountByBlockNumber", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getUncleCountByBlockNumber", []any{"latest"})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(0), uint64(got))
	})

	t.Run("eth_getUncleCountByBlockNumber_notfound", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getUncleCountByBlockNumber", []any{"0xffff"})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getUncleCountByBlockHash", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getUncleCountByBlockHash", []any{fx.blockHash})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(0), uint64(got))
	})

	t.Run("eth_getUncleByBlockHashAndIndex", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getUncleByBlockHashAndIndex", []any{fx.blockHash, "0x0"})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getUncleByBlockNumberAndIndex", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getUncleByBlockNumberAndIndex", []any{"latest", "0x0"})
		assert.Equal(t, "null", string(result))
	})
}
