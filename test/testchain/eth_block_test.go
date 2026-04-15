// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

// eth_block_test.go — integration tests for blocks that mix VeChain-native and Ethereum
// transaction types within the same block.  These tests exercise the full pipeline:
// transaction construction → packer adoption (flow.Adopt) → EVM execution → receipt
// generation → block commitment → chain retrieval.

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ethBlockTestChainID is the Ethereum replay-protection chain ID used to sign Ethereum
// transactions in this file.  It is independent of the VeChain chainTag — NormalizeEthereumTx
// validates the signature against this value, while the VeChain chainTag is set separately
// via NewEthereumTransaction.
const ethBlockTestChainID = uint64(1337)

// TestMintBlock_MixedTxFamilies mints a single block containing:
//  1. A VeChain TypeLegacy tx (VET transfer)
//  2. An Ethereum EthLegacy tx     (VET transfer)
//  3. An Ethereum EthTyped1559 tx  (VET transfer)
//
// Assertions:
//   - MintBlock succeeds (packer adoption, EVM execution, consensus validation all pass).
//   - All three receipts exist and are not reverted.
//   - The recipient's VET balance increased by exactly the sum of the three transferred amounts.
//   - Each tx can be retrieved from the chain repository by its ID.
//
// TODO: add contract-creation clauses (nil To) once the runtime/API layer for that path
// is integrated end-to-end.
func TestMintBlock_MixedTxFamilies(t *testing.T) {
	chain, err := NewDefault()
	require.NoError(t, err)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1].Address

	// Record the recipient's VET balance before the block.
	balanceBefore, err := chain.State().GetBalance(recipient)
	require.NoError(t, err)

	// How much VET each transaction transfers (1 VET = 10^18 smallest units).
	transferPerTx := new(big.Int).Mul(big.NewInt(1e9), big.NewInt(1e9))

	// gasPrice / maxFeePerGas must exceed the initial base fee so validateTxFee passes.
	// InitialBaseFee = 10^13; use 2× for comfortable headroom.
	feeAboveBase := big.NewInt(2 * thor.InitialBaseFee)
	feeForPriority := big.NewInt(thor.InitialBaseFee)

	// --- 1. VeChain TypeLegacy tx ---
	vcTx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(chain.ChainTag()).
		BlockRef(tx.NewBlockRef(chain.Repo().BestBlockSummary().Header.Number())).
		Expiration(1000).
		GasPriceCoef(255).
		Gas(21000).
		Nonce(datagen.RandUint64()).
		Clause(tx.NewClause(&recipient).WithValue(transferPerTx)).
		Build()
	vcTx = tx.MustSign(vcTx, sender.PrivateKey)

	// --- 2. Ethereum EthLegacy tx ---
	// The sender's address is derived from sender.PrivateKey, which is a dev account
	// with VET and VTHO pre-funded in genesis — so gas payment will succeed.
	ethLegacyTx, err := tx.NewEthBuilder(tx.TypeEthLegacy).
		ChainTag(chain.ChainTag()).
		ChainID(ethBlockTestChainID).
		Nonce(1).
		GasPrice(feeAboveBase).
		GasLimit(21000).
		To(&recipient).
		Value(transferPerTx).
		Build(sender.PrivateKey)
	require.NoError(t, err)

	// --- 3. Ethereum EthTyped1559 tx ---
	eth1559Tx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainTag(chain.ChainTag()).
		ChainID(ethBlockTestChainID).
		Nonce(2).
		MaxPriorityFeePerGas(feeForPriority).
		MaxFeePerGas(feeAboveBase).
		GasLimit(21000).
		To(&recipient).
		Value(transferPerTx).
		Build(sender.PrivateKey)
	require.NoError(t, err)

	// Mint a block containing all three transactions.
	require.NoError(t, chain.MintBlock(vcTx, ethLegacyTx, eth1559Tx))

	// --- Verify: receipts exist and are not reverted ---
	for _, trx := range []*tx.Transaction{vcTx, ethLegacyTx, eth1559Tx} {
		receipt, err := chain.GetTxReceipt(trx.ID())
		require.NoError(t, err, "receipt must exist for tx ID %s", trx.ID())
		assert.False(t, receipt.Reverted, "tx %s must not be reverted", trx.ID())
	}

	// --- Verify: VET balance increased by exactly 3 VET ---
	balanceAfter, err := chain.State().GetBalance(recipient)
	require.NoError(t, err)
	expectedIncrease := new(big.Int).Mul(transferPerTx, big.NewInt(3))
	actualIncrease := new(big.Int).Sub(balanceAfter, balanceBefore)
	assert.Equal(t, expectedIncrease, actualIncrease,
		"recipient VET balance must increase by exactly 3 × transferPerTx")

	// --- Verify: each tx is retrievable by ID from the chain repository ---
	for _, trx := range []*tx.Transaction{vcTx, ethLegacyTx, eth1559Tx} {
		retrieved, _, err := chain.Repo().NewBestChain().GetTransaction(trx.ID())
		require.NoError(t, err, "tx %s must be retrievable by ID", trx.ID())
		assert.Equal(t, trx.Type(), retrieved.Type(), "tx type must be preserved in repo")

		wantOrigin, err := trx.Origin()
		require.NoError(t, err)
		gotOrigin, err := retrieved.Origin()
		require.NoError(t, err)
		assert.Equal(t, wantOrigin, gotOrigin, "tx origin must be preserved in repo")
	}
}
