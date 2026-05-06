// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/rpc/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestTransactionsHandler(t *testing.T) {
	fx := testutil.NewChainFixture(t)
	pool := testutil.DefaultPool(t, fx.Chain, &fx.Forks)
	ts := testutil.NewMinimalServer(t, transactions.New(fx.Chain.Repo(), fx.ChainID, pool))

	// ---- eth_getTransactionByHash ----

	t.Run("eth_getTransactionByHash_eth", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{fx.EthTxHash})
		var txObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &txObj))

		var gotHash string
		require.NoError(t, json.Unmarshal(txObj["hash"], &gotHash))
		assert.Equal(t, fx.EthTxHash, gotHash)

		// The ETH tx sits at canonical index 1 but is the only ETH tx → projected index 0.
		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(txObj["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(0), uint64(txIdx))
	})

	t.Run("eth_getTransactionByHash_vechain", func(t *testing.T) {
		// VeChain legacy txs are invisible from the ETH endpoint.
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{fx.VcTxHash})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getTransactionByHash_unknown", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{"0x0000000000000000000000000000000000000000000000000000000000000001"})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_getTransactionByBlockHashAndIndex ----

	t.Run("eth_getTransactionByBlockHashAndIndex", func(t *testing.T) {
		// Projected ETH index 0x0 = first (and only) ETH tx in the block.
		result := testutil.Call(t, ts, "eth_getTransactionByBlockHashAndIndex", []any{fx.BlockHash, "0x0"})
		var txObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &txObj))

		var gotHash string
		require.NoError(t, json.Unmarshal(txObj["hash"], &gotHash))
		assert.Equal(t, fx.EthTxHash, gotHash)
	})

	t.Run("eth_getTransactionByBlockHashAndIndex_outofrange", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockHashAndIndex", []any{fx.BlockHash, "0x1"})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_getTransactionByBlockNumberAndIndex ----

	t.Run("eth_getTransactionByBlockNumberAndIndex", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x1", "0x0"})
		var txObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &txObj))

		var gotHash string
		require.NoError(t, json.Unmarshal(txObj["hash"], &gotHash))
		assert.Equal(t, fx.EthTxHash, gotHash)
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_outofrange", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x1", "0x1"})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_getTransactionReceipt ----

	t.Run("eth_getTransactionReceipt_eth", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{fx.EthTxHash})
		var receipt map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipt))

		var gotHash string
		require.NoError(t, json.Unmarshal(receipt["transactionHash"], &gotHash))
		assert.Equal(t, fx.EthTxHash, gotHash)

		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(0), uint64(txIdx))

		var status hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["status"], &status))
		assert.Equal(t, uint64(1), uint64(status), "transfer should succeed")

		var gasUsed hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["gasUsed"], &gasUsed))
		assert.Greater(t, uint64(gasUsed), uint64(0))

		var txType hexutil.Uint64
		require.NoError(t, json.Unmarshal(receipt["type"], &txType))
		assert.Equal(t, uint64(tx.TypeEthTyped1559), uint64(txType))
	})

	t.Run("eth_getTransactionReceipt_vechain", func(t *testing.T) {
		// VeChain txs have no ETH receipt.
		result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{fx.VcTxHash})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_sendRawTransaction ----

	t.Run("eth_sendRawTransaction_valid", func(t *testing.T) {
		// Use a fresh account (index 2) that hasn't sent any ETH tx → nonce 0.
		freshSender := genesis.DevAccounts()[2]
		freshRecipient := genesis.DevAccounts()[3].Address

		freshTx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
			ChainID(fx.ChainID).
			Nonce(0).
			MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
			MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
			GasLimit(21000).
			To(&freshRecipient).
			Value(big.NewInt(1e9)).
			Build(freshSender.PrivateKey)
		require.NoError(t, err)

		rawBytes, err := freshTx.MarshalBinary()
		require.NoError(t, err)

		result := testutil.Call(t, ts, "eth_sendRawTransaction", []any{"0x" + hexBytesToString(rawBytes)})
		var gotHash string
		require.NoError(t, json.Unmarshal(result, &gotHash))
		assert.NotEmpty(t, gotHash)
	})

	t.Run("eth_sendRawTransaction_invalid", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_sendRawTransaction", []any{"0xdeadbeef"})
		assert.NotEqual(t, 0, rpcErr.Code)
	})
}

func hexBytesToString(b []byte) string {
	const hextable = "0123456789abcdef"
	buf := make([]byte, len(b)*2)
	for i, v := range b {
		buf[i*2] = hextable[v>>4]
		buf[i*2+1] = hextable[v&0x0f]
	}
	return string(buf)
}
