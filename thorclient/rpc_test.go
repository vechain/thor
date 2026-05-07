// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thorclient

import (
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/accounts"
	"github.com/vechain/thor/v2/rpc/blocks"
	rpcchain "github.com/vechain/thor/v2/rpc/chain"
	"github.com/vechain/thor/v2/rpc/fees"
	"github.com/vechain/thor/v2/rpc/logs"
	"github.com/vechain/thor/v2/rpc/simulation"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/rpc/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ethRPCTestEnv holds the test server and all pre-minted transaction context.
//
// Chain layout:
//
//	Block 0: genesis
//	Block 1: VcTx (TypeLegacy) + EthTx (EIP-1559, nonce=0, from Sender)
//	Block 2: EthTx2 (EIP-1559, nonce=1, from Sender) + EthTx3 (EIP-1559, nonce=0, from DevAccounts[2])
type ethRPCTestEnv struct {
	ts         *httptest.Server
	fx         *testutil.ChainFixture
	block2Hash string // block 2 ID (0x-prefixed hex)
	ethTx2Hash string // block 2, projected ETH index 0 (Sender, nonce=1)
	ethTx3Hash string // block 2, projected ETH index 1 (DevAccounts[2], nonce=0)
}

// newEthRPCFixture builds the two-block chain and assembles all ETH RPC handlers.
func newEthRPCFixture(t *testing.T) *ethRPCTestEnv {
	t.Helper()

	// Block 0 + block 1 come from the shared ChainFixture.
	fx := testutil.NewChainFixture(t)

	// Block 2: two ETH txs from different senders so we get non-trivial
	// transactionIndex and cumulativeGasUsed on the second receipt.
	sender2 := genesis.DevAccounts()[2]

	ethTx2, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainID(fx.ChainID).
		Nonce(1). // sender's next nonce: used nonce=0 in block 1
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		GasLimit(21000).
		To(&fx.Recipient.Address).
		Value(big.NewInt(1e9)).
		Build(fx.Sender.PrivateKey)
	require.NoError(t, err)

	ethTx3, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainID(fx.ChainID).
		Nonce(0).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		GasLimit(21000).
		To(&fx.Recipient.Address).
		Value(big.NewInt(1e9)).
		Build(sender2.PrivateKey)
	require.NoError(t, err)

	require.NoError(t, fx.Chain.MintBlock(ethTx2, ethTx3))

	block2, err := fx.Chain.BestBlock()
	require.NoError(t, err)

	pool := testutil.DefaultPool(t, fx.Chain, &fx.Forks)
	srv := rpc.NewServer()
	rpcchain.New(fx.Chain.Repo(), fx.ChainID, "test/1.0").Mount(srv)
	blocks.New(fx.Chain.Repo(), fx.ChainID).Mount(srv)
	transactions.New(fx.Chain.Repo(), fx.ChainID, pool).Mount(srv)
	accounts.New(fx.Chain.Repo(), fx.Chain.Stater()).Mount(srv)
	logs.New(fx.Chain.Repo(), fx.Chain.LogDB(), 100).Mount(srv)
	fees.New(fx.Chain.Repo(), 100).Mount(srv)
	simulation.New(fx.Chain.Repo(), fx.Chain.Stater(), &fx.Forks, 1_000_000).Mount(srv)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &ethRPCTestEnv{
		ts:         ts,
		fx:         fx,
		block2Hash: block2.Header().ID().String(),
		ethTx2Hash: ethTx2.ID().String(),
		ethTx3Hash: ethTx3.ID().String(),
	}
}

func TestEthRPC(t *testing.T) {
	env := newEthRPCFixture(t)
	ts, fx := env.ts, env.fx

	// ── Identity ──────────────────────────────────────────────────────────────

	t.Run("eth_chainId", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_chainId", []any{})
		var chainID hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &chainID))
		assert.Equal(t, fx.ChainID, uint64(chainID))
	})

	t.Run("net_version", func(t *testing.T) {
		result := testutil.Call(t, ts, "net_version", []any{})
		var version string
		require.NoError(t, json.Unmarshal(result, &version))
		assert.Equal(t, strconv.FormatUint(fx.ChainID, 10), version)
	})

	t.Run("eth_blockNumber", func(t *testing.T) {
		// Chain has genesis + 2 minted blocks.
		result := testutil.Call(t, ts, "eth_blockNumber", []any{})
		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &num))
		assert.Equal(t, uint64(2), uint64(num))
	})

	// ── Blocks ────────────────────────────────────────────────────────────────

	t.Run("eth_getBlockByNumber_block1_hashes", func(t *testing.T) {
		// Block 1 contains one VeChain tx (invisible) and one ETH tx.
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"0x1", false})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(block["number"], &num))
		assert.Equal(t, uint64(1), uint64(num))

		var hash string
		require.NoError(t, json.Unmarshal(block["hash"], &hash))
		assert.True(t, strings.EqualFold(fx.BlockHash, hash))

		// Only the EIP-1559 tx is visible; the VeChain-native tx is filtered out.
		var txHashes []string
		require.NoError(t, json.Unmarshal(block["transactions"], &txHashes))
		require.Len(t, txHashes, 1)
		assert.True(t, strings.EqualFold(fx.EthTxHash, txHashes[0]))
	})

	t.Run("eth_getBlockByNumber_block2_hashes", func(t *testing.T) {
		// Block 2 contains two ETH txs from different senders.
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"latest", false})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var num hexutil.Uint64
		require.NoError(t, json.Unmarshal(block["number"], &num))
		assert.Equal(t, uint64(2), uint64(num))

		var txHashes []string
		require.NoError(t, json.Unmarshal(block["transactions"], &txHashes))
		require.Len(t, txHashes, 2)
		assert.True(t, strings.EqualFold(env.ethTx2Hash, txHashes[0]))
		assert.True(t, strings.EqualFold(env.ethTx3Hash, txHashes[1]))
	})

	t.Run("eth_getBlockByNumber_full_tx", func(t *testing.T) {
		// Full-tx mode: transactions array contains EthTx objects, not just hashes.
		result := testutil.Call(t, ts, "eth_getBlockByNumber", []any{"0x1", true})
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
		result := testutil.Call(t, ts, "eth_getBlockByHash", []any{fx.BlockHash, false})
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &block))

		var hash string
		require.NoError(t, json.Unmarshal(block["hash"], &hash))
		assert.True(t, strings.EqualFold(fx.BlockHash, hash))
	})

	// ── Transactions ──────────────────────────────────────────────────────────

	t.Run("eth_getTransactionByHash_eth", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{fx.EthTxHash})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(ethTx["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(fx.EthTxHash, hash))

		var from string
		require.NoError(t, json.Unmarshal(ethTx["from"], &from))
		assert.True(t, strings.EqualFold(fx.Sender.Address.String(), from))
	})

	t.Run("eth_getTransactionByHash_vechain_invisible", func(t *testing.T) {
		// VeChain-native txs must not be visible through the ETH RPC.
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{fx.VcTxHash})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block1", func(t *testing.T) {
		// Block 1, projected ETH index 0 = the only EIP-1559 tx in that block.
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x1", "0x0"})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(fx.EthTxHash, hash))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block2_first", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x2", "0x0"})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(env.ethTx2Hash, hash))
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_block2_second", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x2", "0x1"})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(env.ethTx3Hash, hash))
	})

	t.Run("eth_getTransactionByBlockHashAndIndex", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockHashAndIndex", []any{fx.BlockHash, "0x0"})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx)

		var hash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &hash))
		assert.True(t, strings.EqualFold(fx.EthTxHash, hash))
	})

	t.Run("eth_getTransactionReceipt_block1", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{fx.EthTxHash})
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
		result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{env.ethTx3Hash})
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
		result := testutil.Call(t, ts, "eth_getBalance", []any{fx.Sender.Address.String(), "latest"})
		var bal hexutil.Big
		require.NoError(t, json.Unmarshal(result, &bal))
		assert.True(t, bal.ToInt().Sign() > 0)
	})

	t.Run("eth_getTransactionCount", func(t *testing.T) {
		// Sender executed 2 ETH txs (nonce=0 in block 1, nonce=1 in block 2).
		result := testutil.Call(t, ts, "eth_getTransactionCount", []any{fx.Sender.Address.String(), "latest"})
		var count hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &count))
		assert.Equal(t, uint64(2), uint64(count))
	})

	t.Run("eth_getCode_eoa", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getCode", []any{fx.Sender.Address.String(), "latest"})
		var code hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &code))
		assert.Empty(t, code)
	})

	// ── Fees ──────────────────────────────────────────────────────────────────

	t.Run("eth_gasPrice", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_gasPrice", []any{})
		var price hexutil.Big
		require.NoError(t, json.Unmarshal(result, &price))
		assert.True(t, price.ToInt().Sign() > 0)
	})

	t.Run("eth_maxPriorityFeePerGas", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_maxPriorityFeePerGas", []any{})
		var tip hexutil.Big
		require.NoError(t, json.Unmarshal(result, &tip))
		assert.True(t, tip.ToInt().Sign() > 0)
	})

	t.Run("eth_feeHistory_two_blocks", func(t *testing.T) {
		// blockCount=2, newestBlock="latest" → covers blocks 1 and 2.
		result := testutil.Call(t, ts, "eth_feeHistory", []any{2, "latest", []any{}})
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
		result := testutil.Call(t, ts, "eth_call", []any{
			map[string]any{
				"from":  fx.Sender.Address.String(),
				"to":    fx.Recipient.Address.String(),
				"value": "0x1",
			},
			"latest",
		})
		var data hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &data))
		assert.Empty(t, data) // plain VET transfer returns no output data
	})

	t.Run("eth_estimateGas", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_estimateGas", []any{
			map[string]any{
				"from":  fx.Sender.Address.String(),
				"to":    fx.Recipient.Address.String(),
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
		// TODO: extend with a contract-deploy tx that emits events to cover non-empty results,
		// address filter, topic filter, and EIP-234 blockHash filter.
		result := testutil.Call(t, ts, "eth_getLogs", []any{
			map[string]any{"fromBlock": "0x0", "toBlock": "latest"},
		})
		var got []any
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Empty(t, got)
	})

	// ── Send ──────────────────────────────────────────────────────────────────

	t.Run("eth_sendRawTransaction", func(t *testing.T) {
		// Sender has used nonce=0 (block 1) and nonce=1 (block 2); next nonce is 2.
		newTx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
			ChainID(fx.ChainID).
			Nonce(2).
			MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
			MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
			GasLimit(21000).
			To(&fx.Recipient.Address).
			Value(big.NewInt(1e9)).
			Build(fx.Sender.PrivateKey)
		require.NoError(t, err)

		rawBytes, err := newTx.MarshalBinary()
		require.NoError(t, err)

		// 1. Send: the endpoint validates, adds to pool, and returns the tx hash.
		result := testutil.Call(t, ts, "eth_sendRawTransaction", []any{
			hexutil.Encode(rawBytes),
		})
		var txHash string
		require.NoError(t, json.Unmarshal(result, &txHash))
		assert.True(t, strings.EqualFold(newTx.ID().String(), txHash))

		// 2. Mine: seal a block containing the transaction so it becomes queryable.
		require.NoError(t, fx.Chain.MintBlock(newTx))

		// 3. Read transaction: must be visible in the new block.
		result = testutil.Call(t, ts, "eth_getTransactionByHash", []any{txHash})
		var ethTx map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &ethTx))
		require.NotNil(t, ethTx, "transaction should be found after mining")

		var readHash string
		require.NoError(t, json.Unmarshal(ethTx["hash"], &readHash))
		assert.True(t, strings.EqualFold(txHash, readHash))

		var from string
		require.NoError(t, json.Unmarshal(ethTx["from"], &from))
		assert.True(t, strings.EqualFold(fx.Sender.Address.String(), from))

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(ethTx["type"], &txType))
		assert.Equal(t, uint64(2), uint64(txType))

		// 4. Read receipt: must exist with status=1 (successful transfer).
		result = testutil.Call(t, ts, "eth_getTransactionReceipt", []any{txHash})
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
