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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/test/testnode"
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

	block1, err := testNode.Chain().BestBlock()
	require.NoError(t, err)

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

	require.NoError(t, testNode.Chain().MintBlock(ethTx2, ethTx3))

	block2, err := testNode.Chain().BestBlock()
	require.NoError(t, err)

	// Block 3: ETH call to Energy.transfer — emits a Transfer event.
	// Used for eth_getLogs and filter tests.
	sender3 := genesis.DevAccounts()[3]
	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	energyAddr := builtin.Energy.Address
	ethCallTx := testutil.BuildEthCallTx(t, testNode.Chain().ChainID(), sender3, 0, &energyAddr, callData, 200_000)
	require.NoError(t, testNode.Chain().MintBlock(ethCallTx))

	blockWithEvents, err := testNode.Chain().BestBlock()
	require.NoError(t, err)

	transferEvent, ok := builtin.Energy.ABI.EventByName("Transfer")
	require.True(t, ok)
	transferTopic := common.Hash(transferEvent.ID()).Hex()

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
		// Chain has genesis + 3 minted blocks.
		result := testutil.Call(t, testNode.APIServer(), "eth_blockNumber", []any{})
		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &num))
		assert.Equal(t, uint64(3), uint64(num))
	})

	// ── Simple stubs ──────────────────────────────────────────────────────────

	t.Run("net_listening", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "net_listening", []any{})
		var got bool
		require.NoError(t, json.Unmarshal(result, &got))
		assert.True(t, got)
	})

	t.Run("net_peerCount", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "net_peerCount", []any{})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(0), uint64(got))
	})

	t.Run("eth_coinbase", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_coinbase", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "0x0000000000000000000000000000000000000000", got)
	})

	t.Run("eth_syncing", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_syncing", []any{})
		var got bool
		require.NoError(t, json.Unmarshal(result, &got))
		assert.False(t, got)
	})

	t.Run("eth_accounts", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_accounts", []any{})
		var got []string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	t.Run("eth_mining", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_mining", []any{})
		var got bool
		require.NoError(t, json.Unmarshal(result, &got))
		assert.False(t, got)
	})

	t.Run("eth_hashrate", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_hashrate", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "0x0", got)
	})

	t.Run("web3_clientVersion", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "web3_clientVersion", []any{})
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "Thor/test/1.0", got)
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
		assert.True(t, strings.EqualFold(block1.Header().ID().String(), hash))

		// Only the EIP-1559 tx is visible; the VeChain-native tx is filtered out.
		var txHashes []string
		require.NoError(t, json.Unmarshal(block["transactions"], &txHashes))
		require.Len(t, txHashes, 1)
		assert.True(t, strings.EqualFold(ethTx.ID().String(), txHashes[0]))
	})

	t.Run("eth_getBlockByNumber_block2_hashes", func(t *testing.T) {
		// Block 2 contains two ETH txs from different senders.
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockByNumber", []any{"0x2", false})
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
		// Full-tx mode: transactions array contains EthTx objects, not just hashes.
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

	t.Run("eth_getBlockTransactionCountByNumber", func(t *testing.T) {
		// Block 1: 1 ETH tx; block 2: 2 ETH txs; block 3: 1 ETH call tx.
		for _, tc := range []struct {
			tag      string
			expected uint64
		}{
			{"0x1", 1},
			{"0x2", 2},
			{"0x3", 1},
		} {
			result := testutil.Call(t, testNode.APIServer(), "eth_getBlockTransactionCountByNumber", []any{tc.tag})
			var got hexutil.Uint64
			require.NoError(t, json.Unmarshal(result, &got))
			assert.Equal(t, tc.expected, uint64(got), "block %s", tc.tag)
		}
	})

	t.Run("eth_getBlockTransactionCountByHash", func(t *testing.T) {
		// Block 2 has two ETH txs.
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockTransactionCountByHash", []any{block2.Header().ID().String()})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(2), uint64(got))
	})

	t.Run("eth_getBlockReceipts", func(t *testing.T) {
		// Genesis has no ETH txs → empty receipts array.
		result := testutil.Call(t, testNode.APIServer(), "eth_getBlockReceipts", []any{"0x0"})
		var empty []json.RawMessage
		require.NoError(t, json.Unmarshal(result, &empty))
		assert.Empty(t, empty)

		// Block 1: 1 ETH tx receipt.
		result = testutil.Call(t, testNode.APIServer(), "eth_getBlockReceipts", []any{"0x1"})
		var recs1 []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &recs1))
		require.Len(t, recs1, 1)
		var hash string
		require.NoError(t, json.Unmarshal(recs1[0]["transactionHash"], &hash))
		assert.True(t, strings.EqualFold(ethTx.ID().String(), hash))

		// Block 2: 2 ETH tx receipts.
		result = testutil.Call(t, testNode.APIServer(), "eth_getBlockReceipts", []any{"0x2"})
		var recs2 []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &recs2))
		assert.Len(t, recs2, 2)
	})

	t.Run("eth_getUncleCountByBlockNumber", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getUncleCountByBlockNumber", []any{"latest"})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(0), uint64(got))
	})

	t.Run("eth_getUncleCountByBlockHash", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getUncleCountByBlockHash", []any{block2.Header().ID().String()})
		var got hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, uint64(0), uint64(got))
	})

	t.Run("eth_getUncleByBlockHashAndIndex", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getUncleByBlockHashAndIndex", []any{block2.Header().ID().String(), "0x0"})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getUncleByBlockNumberAndIndex", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getUncleByBlockNumberAndIndex", []any{"latest", "0x0"})
		assert.Equal(t, "null", string(result))
	})

	// ── Transactions ──────────────────────────────────────────────────────────

	t.Run("eth_getTransactionByHash_eth", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByHash", []any{ethTx2.ID().String()})
		var ethTxObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTxObj))
		require.NotNil(t, ethTxObj)

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(ethTxObj["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))

		var hash string
		require.NoError(t, json.Unmarshal(ethTxObj["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx2.ID().String(), hash))

		var from string
		require.NoError(t, json.Unmarshal(ethTxObj["from"], &from))
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
		var ethTxObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTxObj))
		require.NotNil(t, ethTxObj)

		var hash string
		require.NoError(t, json.Unmarshal(ethTxObj["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx2.ID().String(), hash))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block2_second", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByBlockNumberAndIndex", []any{"0x2", "0x1"})
		var ethTxObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTxObj))
		require.NotNil(t, ethTxObj)

		var hash string
		require.NoError(t, json.Unmarshal(ethTxObj["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx3.ID().String(), hash))
	})

	t.Run("eth_getTransactionByBlockHashAndIndex", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getTransactionByBlockHashAndIndex", []any{block2.Header().ID().String(), "0x0"})
		var fetchEthTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fetchEthTx))
		require.NotNil(t, fetchEthTx)

		var hash string
		require.NoError(t, json.Unmarshal(fetchEthTx["hash"], &hash))
		assert.True(t, strings.EqualFold(ethTx2.ID().String(), hash))
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

	t.Run("eth_getCode_contract", func(t *testing.T) {
		// The Energy built-in is a deployed contract — its code must be non-empty.
		result := testutil.Call(t, testNode.APIServer(), "eth_getCode", []any{energyAddr.String(), "latest"})
		var code hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &code))
		assert.NotEmpty(t, code)
	})

	t.Run("eth_getStorageAt", func(t *testing.T) {
		// Slot 0 of an EOA is always zero.
		result := testutil.Call(t, testNode.APIServer(), "eth_getStorageAt", []any{sender.Address.String(), "0x0", "latest"})
		var slot common.Hash
		require.NoError(t, json.Unmarshal(result, &slot))
		assert.Equal(t, common.Hash{}, slot)
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
		// blockCount=2, newestBlock="latest" (block 3) → covers blocks 2 and 3.
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

		// oldestBlock is block 2 (newestBlock=3, blockCount=2 → 3-2+1=2).
		var oldest hexutil.Uint64
		require.NoError(t, json.Unmarshal(fh["oldestBlock"], &oldest))
		assert.Equal(t, uint64(2), uint64(oldest))
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

	t.Run("eth_call_contract", func(t *testing.T) {
		// Call Energy.totalSupply() — a pure view function, returns non-zero ABI-encoded uint256.
		totalSupplyMethod, ok := builtin.Energy.ABI.MethodByName("totalSupply")
		require.True(t, ok)
		tsCallData, err := totalSupplyMethod.EncodeInput()
		require.NoError(t, err)

		result := testutil.Call(t, testNode.APIServer(), "eth_call", []any{
			map[string]any{
				"to":   energyAddr.String(),
				"data": hexutil.Encode(tsCallData),
			},
			"latest",
		})
		var data hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &data))
		assert.Len(t, data, 32, "ABI-encoded uint256 is 32 bytes")
	})

	t.Run("eth_estimateGas_contract", func(t *testing.T) {
		// Estimate gas for Energy.transfer — sender has VTHO so it succeeds; gas > 21000.
		result := testutil.Call(t, testNode.APIServer(), "eth_estimateGas", []any{
			map[string]any{
				"from": sender.Address.String(),
				"to":   energyAddr.String(),
				"data": hexutil.Encode(callData),
			},
		})
		var gas hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &gas))
		assert.Greater(t, uint64(gas), uint64(21000))
	})

	// ── Logs ──────────────────────────────────────────────────────────────────

	t.Run("eth_getLogs_pre_event_range", func(t *testing.T) {
		// Blocks 0–2 contain only plain VET transfers — no contract events.
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "0x2"},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	t.Run("eth_getLogs_range_with_event", func(t *testing.T) {
		// Block 3 contains an Energy.transfer → 1 Transfer event.
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x3", "toBlock": "0x3"},
		})
		var got []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &got))
		require.Len(t, got, 1)

		var addr string
		require.NoError(t, json.Unmarshal(got[0]["address"], &addr))
		assert.True(t, strings.EqualFold(energyAddr.String(), addr))

		var topics []string
		require.NoError(t, json.Unmarshal(got[0]["topics"], &topics))
		require.NotEmpty(t, topics)
		assert.True(t, strings.EqualFold(transferTopic, topics[0]))
	})

	t.Run("eth_getLogs_address_filter", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"address":   energyAddr.String(),
			},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1)
	})

	t.Run("eth_getLogs_wrong_address", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
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

	t.Run("eth_getLogs_topic_filter", func(t *testing.T) {
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"topics":    []any{transferTopic},
			},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1)
	})

	t.Run("eth_getLogs_blockHash_filter", func(t *testing.T) {
		// EIP-234: single-block query by blockHash for the block that has the event.
		result := testutil.Call(t, testNode.APIServer(), "eth_getLogs", []any{
			map[string]any{"blockHash": blockWithEvents.Header().ID().String()},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Len(t, got, 1)
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
		var ethTxObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTxObj))
		require.NotNil(t, ethTxObj, "transaction should be found after mining")

		var readHash string
		require.NoError(t, json.Unmarshal(ethTxObj["hash"], &readHash))
		assert.True(t, strings.EqualFold(txHash, readHash))

		var from string
		require.NoError(t, json.Unmarshal(ethTxObj["from"], &from))
		assert.True(t, strings.EqualFold(sender.Address.String(), from))

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(ethTxObj["type"], &txType))
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

	// ── Filters ───────────────────────────────────────────────────────────────
	// The instantMintPool auto-mines a block on every AddLocal, so eth_sendRawTransaction
	// both fires the txFeed (for pending filters) and advances the chain (for block filters)
	// before the HTTP response returns — no sleeps or retries needed.

	t.Run("eth_newBlockFilter", func(t *testing.T) {
		// Create filter, then send a plain VET transfer — auto-mines a new block.
		idResult := testutil.Call(t, testNode.APIServer(), "eth_newBlockFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))
		assert.Regexp(t, `^0x[0-9a-f]+$`, filterID)

		filterSender := genesis.DevAccounts()[4]
		vtxUnsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
			ChainID(testNode.Chain().ChainID()).
			Nonce(0).
			MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
			MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
			Gas(21000).
			To(&recipient.Address).
			Value(big.NewInt(1e9)).
			Build()
		vtx, err := tx.Sign(vtxUnsigned, filterSender.PrivateKey)
		require.NoError(t, err)
		rawBytes, err := vtx.MarshalBinary()
		require.NoError(t, err)
		testutil.Call(t, testNode.APIServer(), "eth_sendRawTransaction", []any{hexutil.Encode(rawBytes)})

		result := testutil.Call(t, testNode.APIServer(), "eth_getFilterChanges", []any{filterID})
		var hashes []common.Hash
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Len(t, hashes, 1)

		// Second poll — no new blocks — empty.
		result = testutil.Call(t, testNode.APIServer(), "eth_getFilterChanges", []any{filterID})
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Empty(t, hashes)
	})

	t.Run("eth_newPendingTransactionFilter", func(t *testing.T) {
		// txFeed fires synchronously inside AddLocal before MintBlock returns,
		// so the hash is in the filter channel when eth_sendRawTransaction responds.
		idResult := testutil.Call(t, testNode.APIServer(), "eth_newPendingTransactionFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		filterSender := genesis.DevAccounts()[5]
		vtxUnsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
			ChainID(testNode.Chain().ChainID()).
			Nonce(0).
			MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
			MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
			Gas(21000).
			To(&recipient.Address).
			Value(big.NewInt(1e9)).
			Build()
		vtx, err := tx.Sign(vtxUnsigned, filterSender.PrivateKey)
		require.NoError(t, err)
		rawBytes, err := vtx.MarshalBinary()
		require.NoError(t, err)
		testutil.Call(t, testNode.APIServer(), "eth_sendRawTransaction", []any{hexutil.Encode(rawBytes)})

		result := testutil.Call(t, testNode.APIServer(), "eth_getFilterChanges", []any{filterID})
		var hashes []common.Hash
		require.NoError(t, json.Unmarshal(result, &hashes))
		require.Len(t, hashes, 1)
		assert.True(t, strings.EqualFold(vtx.ID().String(), hashes[0].Hex()))

		// Second poll — drained — empty.
		result = testutil.Call(t, testNode.APIServer(), "eth_getFilterChanges", []any{filterID})
		require.NoError(t, json.Unmarshal(result, &hashes))
		assert.Empty(t, hashes)
	})

	t.Run("eth_newFilter_getFilterLogs", func(t *testing.T) {
		// Log filter covering block 3 — eth_getFilterLogs returns the Transfer event.
		idResult := testutil.Call(t, testNode.APIServer(), "eth_newFilter", []any{map[string]any{
			"fromBlock": "0x3",
			"toBlock":   "0x3",
		}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		result := testutil.Call(t, testNode.APIServer(), "eth_getFilterLogs", []any{filterID})
		var logs []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &logs))
		require.Len(t, logs, 1)

		var addr string
		require.NoError(t, json.Unmarshal(logs[0]["address"], &addr))
		assert.True(t, strings.EqualFold(energyAddr.String(), addr))
	})

	t.Run("eth_newFilter_getFilterChanges", func(t *testing.T) {
		// Create log filter, then send Energy.transfer (auto-mines) → poll returns 1 event.
		idResult := testutil.Call(t, testNode.APIServer(), "eth_newFilter", []any{map[string]any{}})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		filterSender := genesis.DevAccounts()[6]
		callDataForFilter, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
		require.NoError(t, err)
		filterCallTx := testutil.BuildEthCallTx(t, testNode.Chain().ChainID(), filterSender, 0, &energyAddr, callDataForFilter, 200_000)
		rawBytes, err := filterCallTx.MarshalBinary()
		require.NoError(t, err)
		testutil.Call(t, testNode.APIServer(), "eth_sendRawTransaction", []any{hexutil.Encode(rawBytes)})

		result := testutil.Call(t, testNode.APIServer(), "eth_getFilterChanges", []any{filterID})
		var logs []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &logs))
		require.Len(t, logs, 1)

		var addr string
		require.NoError(t, json.Unmarshal(logs[0]["address"], &addr))
		assert.True(t, strings.EqualFold(energyAddr.String(), addr))

		// Second poll — no new blocks — empty.
		result = testutil.Call(t, testNode.APIServer(), "eth_getFilterChanges", []any{filterID})
		var empty []any
		require.NoError(t, json.Unmarshal(result, &empty))
		assert.Empty(t, empty)
	})

	t.Run("eth_uninstallFilter", func(t *testing.T) {
		idResult := testutil.Call(t, testNode.APIServer(), "eth_newBlockFilter", []any{})
		var filterID string
		require.NoError(t, json.Unmarshal(idResult, &filterID))

		result := testutil.Call(t, testNode.APIServer(), "eth_uninstallFilter", []any{filterID})
		var ok bool
		require.NoError(t, json.Unmarshal(result, &ok))
		assert.True(t, ok)

		result = testutil.Call(t, testNode.APIServer(), "eth_uninstallFilter", []any{"0x9999"})
		require.NoError(t, json.Unmarshal(result, &ok))
		assert.False(t, ok)
	})
}
