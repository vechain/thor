// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

// mixed_tx_block_test.go — verifies that a block body containing transactions from all
// four type families (TypeLegacy, TypeDynamicFee, TypeEthLegacy, TypeEthTyped1559) encodes
// and decodes correctly via RLP, with every transaction preserving its type, identity
// (including ethTxHash for Ethereum types), gas, and signer address.

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// mixedBlockChainID is an arbitrary Ethereum replay-protection chain ID for these tests.
// It is independent of the VeChain chainTag — both serve replay protection at different
// layers of the stack.
const mixedBlockChainID = uint64(1337)

// mixedBlockChainTag is the VeChain genesis stub used in the roundtrip test.
const mixedBlockChainTag = byte(0x27)

// mixedBlockTestKey is a deterministic secp256k1 key used to sign both VeChain-native
// and Ethereum transactions in this file.
var mixedBlockTestKey = func() *ecdsa.PrivateKey {
	k, err := crypto.HexToECDSA("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	if err != nil {
		panic(err)
	}
	return k
}()

// TestMixedTxFamilyBlock_Roundtrip builds a block that contains one transaction from each
// of the four supported type families, RLP-encodes it, decodes it, and asserts that every
// transaction survives the round-trip with its observable properties intact.
//
// Critical invariants verified for Ethereum tx types:
//   - ID() == ethTxHash (Keccak256 of raw wire bytes) is preserved through block body encoding.
//   - Origin() (ECDSA recovery) succeeds on the decoded bytes, returning the original signer.
func TestMixedTxFamilyBlock_Roundtrip(t *testing.T) {
	addrA := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	value := big.NewInt(1e9) // arbitrary for serialisation tests
	emptyRoot := thor.BytesToBytes32([]byte("root"))

	// --- VeChain-native transactions (signed so ID() is deterministic) ---

	vcLegacy := tx.MustSign(
		tx.NewBuilder(tx.TypeLegacy).
			ChainTag(mixedBlockChainTag).
			Clause(tx.NewClause(&addrA).WithValue(value)).
			Gas(21000).
			Nonce(1).
			Build(),
		mixedBlockTestKey,
	)

	vcDynFee := tx.MustSign(
		tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(mixedBlockChainTag).
			Clause(tx.NewClause(&addrA).WithValue(value)).
			Gas(21000).
			MaxFeePerGas(big.NewInt(20e9)).
			MaxPriorityFeePerGas(big.NewInt(1e9)).
			Nonce(2).
			Build(),
		mixedBlockTestKey,
	)

	// --- Ethereum transactions ---
	ethLegacy, err := tx.NewEthBuilder(tx.TypeEthLegacy).
		ChainTag(mixedBlockChainTag).
		ChainID(mixedBlockChainID).
		Nonce(10).
		GasPrice(big.NewInt(20e9)).
		GasLimit(21000).
		To(&addrA).
		Value(value).
		Build(mixedBlockTestKey)
	require.NoError(t, err)

	eth1559, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainTag(mixedBlockChainTag).
		ChainID(mixedBlockChainID).
		Nonce(20).
		MaxPriorityFeePerGas(big.NewInt(1e9)).
		MaxFeePerGas(big.NewInt(20e9)).
		GasLimit(21000).
		To(&addrA).
		Value(value).
		Build(mixedBlockTestKey)
	require.NoError(t, err)

	// Build the block.
	blk := new(Builder).
		GasUsed(0).
		Transaction(vcLegacy).
		Transaction(vcDynFee).
		Transaction(ethLegacy).
		Transaction(eth1559).
		GasLimit(2_000_000).
		TotalScore(1).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(1_000_000).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		ParentID(emptyRoot).
		Build()

	// Encode → decode.
	encoded, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	var decoded Block
	require.NoError(t, rlp.DecodeBytes(encoded, &decoded))

	got := decoded.Transactions()
	require.Len(t, got, 4, "decoded block must contain exactly 4 transactions")

	// 0: TypeLegacy
	assert.Equal(t, tx.TypeLegacy, got[0].Type())
	assert.Equal(t, vcLegacy.ID(), got[0].ID())
	assert.Equal(t, vcLegacy.Gas(), got[0].Gas())
	mixedBlockAssertOrigin(t, vcLegacy, got[0])

	// 1: TypeDynamicFee
	assert.Equal(t, tx.TypeDynamicFee, got[1].Type())
	assert.Equal(t, vcDynFee.ID(), got[1].ID())
	assert.Equal(t, vcDynFee.Gas(), got[1].Gas())
	mixedBlockAssertOrigin(t, vcDynFee, got[1])

	// 2: TypeEthLegacy — ethTxHash (ID) must survive the block body round-trip intact.
	assert.Equal(t, tx.TypeEthLegacy, got[2].Type())
	assert.Equal(t, ethLegacy.ID(), got[2].ID(), "ethTxHash must survive block body round-trip")
	assert.Equal(t, ethLegacy.Gas(), got[2].Gas())
	mixedBlockAssertOrigin(t, ethLegacy, got[2])

	// 3: TypeEthTyped1559
	assert.Equal(t, tx.TypeEthTyped1559, got[3].Type())
	assert.Equal(t, eth1559.ID(), got[3].ID(), "ethTxHash must survive block body round-trip")
	assert.Equal(t, eth1559.Gas(), got[3].Gas())
	mixedBlockAssertOrigin(t, eth1559, got[3])
	assert.Equal(t, eth1559.MaxFeePerGas(), got[3].MaxFeePerGas())
	assert.Equal(t, eth1559.MaxPriorityFeePerGas(), got[3].MaxPriorityFeePerGas())
}

// --- helpers ----------------------------------------------------------------

// mixedBlockAssertOrigin recovers Origin() from both transactions and asserts equality.
func mixedBlockAssertOrigin(t *testing.T, want, got *tx.Transaction) {
	t.Helper()
	wantOrigin, err := want.Origin()
	require.NoError(t, err, "want.Origin()")
	gotOrigin, err := got.Origin()
	require.NoError(t, err, "got.Origin()")
	assert.Equal(t, wantOrigin, gotOrigin, "Origin must match after round-trip")
}
