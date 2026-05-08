// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thorclient

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/test/testnode"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// newEthRPCFixture builds the two-block chain and assembles all ETH RPC handlers.
func newEthRPCFixture(t *testing.T) testnode.Node {
	t.Helper()

	c, err := testchain.NewDefault()
	require.NoError(t, err)

	testNode, err := testnode.NewNodeBuilder().WithChain(c).Build()
	require.NoError(t, err)
	require.NoError(t, testNode.Start())

	return testNode
}

func TestEthRPC(t *testing.T) {
	testNode := newEthRPCFixture(t)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	vcTx := testutil.BuildVcTx(t, testNode.Chain(), sender, &recipient.Address)
	ethTx := testutil.BuildEthTx(t, testNode.Chain().ChainID(), sender, 0, &recipient.Address)

	require.NoError(t, testNode.Chain().MintBlock(vcTx, ethTx))
	require.Equal(t, uint32(1), testNode.Chain().Repo().BestBlockSummary().Header.Number())

	// Block 2: two ETH txs from different senders so we get non-trivial
	// transactionIndex and cumulativeGasUsed on the second receipt.
	sender2 := genesis.DevAccounts()[2]

	unsigned2 := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(testNode.Chain().ChainID()).
		Nonce(1). // sender's next nonce: used nonce=0 in block 1
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		Gas(21000).
		To(&recipient.Address).
		Value(big.NewInt(1e9)).
		Build()
	ethTx2, err := tx.Sign(unsigned2, sender.PrivateKey)
	require.NoError(t, err)

	unsigned3 := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(testNode.Chain().ChainID()).
		Nonce(0).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		Gas(21000).
		To(&recipient.Address).
		Value(big.NewInt(1e9)).
		Build()
	ethTx3, err := tx.Sign(unsigned3, sender2.PrivateKey)
	require.NoError(t, err)

	block2, err := testNode.Chain().BestBlock()
	require.NoError(t, err)

	require.NoError(t, testNode.Chain().MintBlock(ethTx2, ethTx3))

	// ── Identity ──────────────────────────────────────────────────────────────

	t.Run("eth_chainId", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_chainId", []any{})
		var chainID hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &chainID))
		assert.Equal(t, testNode.Chain().ChainID(), uint64(chainID))
	})

	t.Run("net_version", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "net_version", []any{})
		var version string
		require.NoError(t, json.Unmarshal(result, &version))
		assert.Equal(t, strconv.FormatUint(testNode.Chain().ChainID(), 10), version)
	})

	t.Run("eth_blockNumber", func(t *testing.T) {
		// Chain has genesis + 2 minted blocks.
		result := testutil.Call(t, testNode.APIServer(), "eth_blockNumber", []any{})
		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &num))
		assert.Equal(t, uint64(2), uint64(num))
	})

	// ── Blocks ────────────────────────────────────────────────────────────────

	t.Run("eth_getBlockByNumber_block1_hashes", func(t *testing.T) {
		// Block 1 contains one VeChain tx (invisible) and one ETH tx.
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockByNumber", []any{"0x1", false})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(block["number"], &num))
		assert.Equal(t, uint64(1), uint64(num))

		var hash string
		require.NoError(t, json.Unmarshal(block["hash"], &hash))
		assert.True(t, strings.EqualFold(block2.Header().ID().String(), hash))

		// Only the EIP-1559 tx is visible; the VeChain-native tx is filtered out.
		var txHashes []string
		require.NoError(t, json.Unmarshal(block["transactions"], &txHashes))
		require.Len(t, txHashes, 1)
		assert.True(t, strings.EqualFold(ethTx.ID().String(), txHashes[0]))
	})

	t.Run("eth_getBlockByNumber_block2_hashes", func(t *testing.T) {
		// Block 2 contains two ETH txs from different senders.
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockByNumber", []any{"latest", false})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(block["number"], &num))
		assert.Equal(t, uint64(2), uint64(num))

		var txHashes []string
		require.NoError(t, json.Unmarshal(block["transactions"], &txHashes))
		require.Len(t, txHashes, 2)
		assert.True(t, strings.EqualFold(ethTx2.ID().String(), txHashes[0]))
		assert.True(t, strings.EqualFold(ethTx3.ID().String(), txHashes[1]))
	})

	t.Run("eth_getBlockByNumber_full_tx", func(t *testing.T) {
		// Full-tx mode: transactions array contains EthTx objectestNode.APIServer() not just hashes.
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockByNumber", []any{"0x1", true})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var txObjects []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(block["transactions"], &txObjects))
		require.Len(t, txObjects, 1)

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(txObjects[0]["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))
	})

	t.Run("eth_getBlockByHash", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockByHash", []any{block2.Header().ID().String(), false})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var hash string
		require.NoError(t, json.Unmarshal(block["hash"], &hash))
		assert.True(t, strings.EqualFold(block2.Header().ID().String(), hash))
	})

	// ── Transactions ──────────────────────────────────────────────────────────

	t.Run("eth_getTransactionByHash_eth", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByHash", []any{ethTx2.ID().String()})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(ethTx["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx2.ID().String(), hash))

		var from string
		require.NoError(t, json.Unmarshal(ethTx["from"], &from))
		assert.True(t, strings.EqualFold(sender.Address.String(), from))
	})

	t.Run("eth_getTransactionByHash_vechain_invisible", func(t *testing.T) {
		// VeChain-native txs must not be visible through the ETH RPC.
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByHash", []any{vcTx.ID().String()})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block1", func(t *testing.T) {
		// Block 1, projected ETH index 0 = the only EIP-1559 tx in that block.
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByBlockNumberAndIndex", []any{"0x1", "0x0"})
		var fetchEthTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fetchEthTx))
		require.NotNil(t, fetchEthTx)

		var hash string
		require.NoError(t, json.Unmarshal(fetchEthTx["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx.ID().String(), hash))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block2_first", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByBlockNumberAndIndex", []any{"0x2", "0x0"})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx2.ID().String(), hash))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block2_second", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByBlockNumberAndIndex", []any{"0x2", "0x1"})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx3.ID().String(), hash))
	})

	t.Run("eth_getTransactionByBlockHashAndIndex", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByBlockHashAndIndex", []any{block2.Header().ID().String(), "0x0"})
		var fetchEthTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fetchEthTx))
		require.NotNil(t, fetchEthTx)

		var hash string
		require.NoError(t, json.Unmarshal(fetchEthTx["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx.ID().String(), hash))
	})

	t.Run("eth_getTransactionReceipt_block1", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionReceipt", []any{ethTx2.ID().String()})
		var receipt map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipt))
		require.NotNil(t, receipt)

		var status hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["status"], &status))
		assert.Equal(t, uint64(1), uint64(status))

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))

		// Only tx in block 1: transactionIndex=0, cumulativeGasUsed=gasUsed=21000.
		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(0), uint64(txIdx))

		var cumGas hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["cumulativeGasUsed"], &cumGas))
		assert.Equal(t, uint64(21000), uint64(cumGas))
	})

	t.Run("eth_getTransactionReceipt_block2_second", func(t *testing.T) {
		// Second ETH tx in block 2: transactionIndex=1, cumulativeGasUsed=42000.
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionReceipt", []any{ethTx3.ID().String()})
		var receipt map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipt))
		require.NotNil(t, receipt)

		var status hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["status"], &status))
		assert.Equal(t, uint64(1), uint64(status))

		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(1), uint64(txIdx))

		var gasUsed hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["gasUsed"], &gasUsed))
		assert.Equal(t, uint64(21000), uint64(gasUsed))

		// Cumulative = gas from both ETH txs in the block.
		var cumGas hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["cumulativeGasUsed"], &cumGas))
		assert.Equal(t, uint64(42000), uint64(cumGas))
	})

	// ── Accounts ──────────────────────────────────────────────────────────────

	t.Run("eth_getBalance", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getBalance", []any{sender.Address.String(), "latest"})
		var bal hexutil.Big
		require.NoError(t, json.Unmarshal(result, &bal))
		assert.True(t, bal.ToInt().Sign() > 0)
	})

	t.Run("eth_getTransactionCount", func(t *testing.T) {
		// Sender executed 2 ETH txs (nonce=0 in block 1, nonce=1 in block 2).
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionCount", []any{sender.Address.String(), "latest"})
		var count hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &count))
		assert.Equal(t, uint64(2), uint64(count))
	})

	t.Run("eth_getCode_eoa", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getCode", []any{sender.Address.String(), "latest"})
		var code hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &code))
		assert.Empty(t, code)
	})

	// ── Fees ──────────────────────────────────────────────────────────────────

	t.Run("eth_gasPrice", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_gasPrice", []any{})
		var price hexutil.Big
		require.NoError(t, json.Unmarshal(result, &price))
		assert.True(t, price.ToInt().Sign() > 0)
	})

	t.Run("eth_maxPriorityFeePerGas", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_maxPriorityFeePerGas", []any{})
		var tip hexutil.Big
		require.NoError(t, json.Unmarshal(result, &tip))
		assert.True(t, tip.ToInt().Sign() > 0)
	})

	t.Run("eth_feeHistory_two_blocks", func(t *testing.T) {
		// blockCount=2, newestBlock="latest" → covers blocks 1 and 2.
		result := testutil.Call(t, testNode.APIServer(), "eth_feeHistory", []any{2, "latest", []any{}})
		var fh map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fh))

		// baseFeePerGas has length blockCount+1 = 3.
		var baseFees []*hexutil.Big
		require.NoError(t, json.Unmarshal(fh["baseFeePerGas"], &baseFees))
		assert.Len(t, baseFees, 3)

		// gasUsedRatio has length blockCount = 2.
		var gasRatios []float64
		require.NoError(t, json.Unmarshal(fh["gasUsedRatio"], &gasRatios))
		assert.Len(t, gasRatios, 2)

		// oldestBlock is block 1.
		var oldest hexutil.Uint64
		require.NoError(t, json.Unmarshal(fh["oldestBlock"], &oldest))
		assert.Equal(t, uint64(1), uint64(oldest))
	})

	// ── Simulation ────────────────────────────────────────────────────────────

	t.Run("eth_call", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_call", []any{
			map[string]any{
				"from":  sender.Address.String(),
				"to":    recipient.Address.String(),
				"value": "0x1",
			},
			"latest",
		})
		var data hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &data))
		assert.Empty(t, data) // plain VET transfer returns no output data
	})

	t.Run("eth_estimateGas", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_estimateGas", []any{
			map[string]any{
				"from":  sender.Address.String(),
				"to":    recipient.Address.String(),
				"value": "0x1",
			},
		})
		var gas hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &gas))
		assert.Equal(t, uint64(21000), uint64(gas))
	})

	// ── Logs ──────────────────────────────────────────────────────────────────

	t.Run("eth_getLogs_empty", func(t *testing.T) {
		// All fixture txs are plain VET transfers — no contract events are emitted.
		// TODO: extend with a contract-deploy tx that emits events to cover non-empty resultestNode.APIServer()
		// address filter, topic filter, and EIP-234 blockHash filter.
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	// ── Send ──────────────────────────────────────────────────────────────────

	t.Run("eth_sendRawTransaction", func(t *testing.T) {
		// Sender has used nonce=0 (block 1) and nonce=1 (block 2); next nonce is 2.
		unsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
			ChainID(testNode.Chain().ChainID()).
			Nonce(2).
			MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
			MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
			Gas(21000).
			To(&recipient.Address).
			Value(big.NewInt(1e9)).
			Build()
		newTx, err := tx.Sign(unsigned, sender.PrivateKey)
		require.NoError(t, err)

		rawBytes, err := newTx.MarshalBinary()
		require.NoError(t, err)

		// 1. Send: the endpoint validates, adds to pool, and returns the tx hash.
		result := testutil.Call(t, testNode.APIServer(), "eth_sendRawTransaction", []any{
			hexutil.Encode(rawBytes),
		})
		var txHash string
		require.NoError(t, json.Unmarshal(result, &txHash))
		assert.True(t, strings.EqualFold(newTx.ID().String(), txHash))

		// 2. Read transaction: must be visible in the new block.
		result = testutil.Call(t, testNode.APIServer(), "eth_getTransactionByHash", []any{txHash})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx, "transaction should be found after mining")

		var readHash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &readHash))
		assert.True(t, strings.EqualFold(txHash, readHash))

		var from string
		require.NoError(t, json.Unmarshal(ethTx["from"], &from))
		assert.True(t, strings.EqualFold(sender.Address.String(), from))

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(ethTx["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))

		// 3. Read receipt: must exist with status=1 (successful transfer).
		result = testutil.Call(t, testNode.APIServer(), "eth_getTransactionReceipt", []any{txHash})
		var receipt map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipt))
		require.NotNil(t, receipt, "receipt should be found after mining")

		var status hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["status"], &status))
		assert.Equal(t, uint64(1), uint64(status))

		var receiptType hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["type"], &receiptType))
		assert.Equal(t, uint64(2), uint64(receiptType))

		var receiptHash string
		require.NoError(t, json.Unmarshal(receipt["transactionHash"], &receiptHash))
		assert.True(t, strings.EqualFold(txHash, receiptHash))
	})
}
