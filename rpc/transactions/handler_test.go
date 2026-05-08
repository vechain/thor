// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions_test

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/rpc/transactions"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type fixture struct {
	chain     *testchain.Chain
	chainID   uint64
	ethTxHash string
	vcTxHash  string
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
		vcTxHash:  vcTx.ID().String(),
		blockHash: bestBlock.Header().ID().String(),
	}
}

func TestTransactionsHandler(t *testing.T) {
	fx := newFixture(t)
	pool := txpool.New(fx.chain.Repo(), fx.chain.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	ts := testutil.NewTestServer(t, transactions.New(fx.chain.Repo(), fx.chainID, pool))

	// ---- eth_getTransactionByHash ----

	t.Run("eth_getTransactionByHash_eth", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{fx.ethTxHash})
		var txObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &txObj))

		var gotHash string
		require.NoError(t, json.Unmarshal(txObj["hash"], &gotHash))
		assert.Equal(t, fx.ethTxHash, gotHash)

		// The ETH tx sits at canonical index 1 but is the only ETH tx → projected index 0.
		var txIdx hexutil.Uint64
		require.NoError(t, json.Unmarshal(txObj["transactionIndex"], &txIdx))
		assert.Equal(t, uint64(0), uint64(txIdx))
	})

	t.Run("eth_getTransactionByHash_vechain", func(t *testing.T) {
		// VeChain legacy txs are invisible from the ETH endpoint.
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{fx.vcTxHash})
		assert.Equal(t, "null", string(result))
	})

	t.Run("eth_getTransactionByHash_unknown", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByHash", []any{"0x0000000000000000000000000000000000000000000000000000000000000001"})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_getTransactionByBlockHashAndIndex ----

	t.Run("eth_getTransactionByBlockHashAndIndex", func(t *testing.T) {
		// Projected ETH index 0x0 = first (and only) ETH tx in the block.
		result := testutil.Call(t, ts, "eth_getTransactionByBlockHashAndIndex", []any{fx.blockHash, "0x0"})
		var txObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &txObj))

		var gotHash string
		require.NoError(t, json.Unmarshal(txObj["hash"], &gotHash))
		assert.Equal(t, fx.ethTxHash, gotHash)
	})

	t.Run("eth_getTransactionByBlockHashAndIndex_outofrange", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockHashAndIndex", []any{fx.blockHash, "0x1"})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_getTransactionByBlockNumberAndIndex ----

	t.Run("eth_getTransactionByBlockNumberAndIndex", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x1", "0x0"})
		var txObj map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &txObj))

		var gotHash string
		require.NoError(t, json.Unmarshal(txObj["hash"], &gotHash))
		assert.Equal(t, fx.ethTxHash, gotHash)
	})

	t.Run("eth_getTransactionByBlockNumberAndIndex_outofrange", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionByBlockNumberAndIndex", []any{"0x1", "0x1"})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_getTransactionReceipt ----

	t.Run("eth_getTransactionReceipt_eth", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{fx.ethTxHash})
		var receipt map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &receipt))

		var gotHash string
		require.NoError(t, json.Unmarshal(receipt["transactionHash"], &gotHash))
		assert.Equal(t, fx.ethTxHash, gotHash)

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
		assert.Equal(t, uint64(tx.TypeEthDynamicFee), uint64(txType))

		// Simple value transfer emits no events → logsBloom must be all zeros.
		var logsBloom hexutil.Bytes
		require.NoError(t, json.Unmarshal(receipt["logsBloom"], &logsBloom))
		require.Len(t, logsBloom, 256)
		assert.Equal(t, make([]byte, 256), []byte(logsBloom))
	})

	t.Run("eth_getTransactionReceipt_vechain", func(t *testing.T) {
		// VeChain txs have no ETH receipt.
		result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{fx.vcTxHash})
		assert.Equal(t, "null", string(result))
	})

	// ---- eth_sendRawTransaction ----

	t.Run("eth_sendRawTransaction_valid", func(t *testing.T) {
		// Use a fresh account (index 2) that hasn't sent any ETH tx → nonce 0.
		freshSender := genesis.DevAccounts()[2]
		freshRecipient := genesis.DevAccounts()[3].Address

		unsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
			ChainID(fx.chainID).
			Nonce(0).
			MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
			MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
			Gas(21000).
			To(&freshRecipient).
			Value(big.NewInt(1e9)).
			Build()
		freshTx, err := tx.Sign(unsigned, freshSender.PrivateKey)
		require.NoError(t, err)

		rawBytes, err := freshTx.MarshalBinary()
		require.NoError(t, err)

		result := testutil.Call(t, ts, "eth_sendRawTransaction", []any{"0x" + hex.EncodeToString(rawBytes)})
		var gotHash string
		require.NoError(t, json.Unmarshal(result, &gotHash))
		assert.NotEmpty(t, gotHash)
	})

	t.Run("eth_sendRawTransaction_invalid", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_sendRawTransaction", []any{"0xdeadbeef"})
		assert.NotEqual(t, 0, rpcErr.Code)
	})
}

// TestTransactionReceiptBloom verifies that eth_getTransactionReceipt populates
// logsBloom correctly for an ETH typed tx that emits contract events.
func TestTransactionReceiptBloom(t *testing.T) {
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

	pool := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	ts := testutil.NewTestServer(t, transactions.New(c.Repo(), chainID, pool))

	result := testutil.Call(t, ts, "eth_getTransactionReceipt", []any{ethCallTx.ID().String()})
	var receipt map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &receipt))

	// logsBloom must be non-zero: Energy.transfer emits a Transfer event.
	var logsBloom hexutil.Bytes
	require.NoError(t, json.Unmarshal(receipt["logsBloom"], &logsBloom))
	require.Len(t, logsBloom, 256)
	assert.NotEqual(t, make([]byte, 256), []byte(logsBloom), "logsBloom should be non-zero for receipt with events")

	// The bloom must contain the Energy contract address.
	var bloom256 [256]byte
	copy(bloom256[:], logsBloom)
	ethBloom := ethtypes.BytesToBloom(bloom256[:])
	assert.True(t, ethtypes.BloomLookup(ethBloom, common.Address(builtin.Energy.Address)), "receipt bloom should contain Energy contract address")
}
