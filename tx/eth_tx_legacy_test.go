// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
)

func TestEthLegacy_ValidDecodeHashAndSender(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	assert.Equal(t, TypeEthLegacy, ntx.TxType)
	assert.Equal(t, testSenderAddress, ntx.Sender)
	assert.Equal(t, uint64(5), ntx.Nonce)
	assert.Equal(t, uint64(21000), ntx.GasLimit)
	assert.Equal(t, big.NewInt(20e9), ntx.GasPrice)
	assert.Nil(t, ntx.MaxFeePerGas)
	assert.Nil(t, ntx.MaxPriorityFeePerGas)
	assert.Equal(t, testChainID, ntx.ChainID)

	// Hash must equal Keccak256(rawBytes).
	expectedHash := thor.Keccak256(rawBytes)
	assert.Equal(t, expectedHash, ntx.Hash)

	// Raw must be the original bytes.
	assert.Equal(t, rawBytes, ntx.Raw)
}

func TestEthLegacy_ChainIDExtractedFromV(t *testing.T) {
	// Build with chain ID 100 — different from testChainID.
	const altChainID = uint64(100)
	rawBytes, err := defaultEthLegacyBuilder().ChainID(altChainID).BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, altChainID)
	require.NoError(t, err)
	assert.Equal(t, altChainID, ntx.ChainID)
}

func TestEthLegacy_ChainIDMismatch(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey) // signed for chainID=1337
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, 1) // wrong chain
	require.Error(t, err)
	ethErr := err.(*EthTxError)
	assert.Equal(t, EthErrChainIDMismatch, ethErr.Code)
}

func TestEthLegacy_PreEIP155Rejected(t *testing.T) {
	// Craft a transaction with v=27 (pre-EIP-155).
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	type txBody struct {
		Nonce    uint64
		GasPrice *big.Int
		GasLimit uint64
		To       *thor.Address `rlp:"nil"`
		Value    *big.Int
		Data     []byte
		V        *big.Int
		R        *big.Int
		S        *big.Int
	}
	rawBytes, err := rlp.EncodeToBytes(&txBody{
		Nonce:    1,
		GasPrice: big.NewInt(1e9),
		GasLimit: 21000,
		To:       &to,
		Value:    big.NewInt(0),
		V:        big.NewInt(27),
		R:        big.NewInt(1),
		S:        big.NewInt(1),
	})
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrEIP155Required, err.(*EthTxError).Code)
}

func TestEthLegacy_HighSRejected(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	// Decode, flip s to secp256k1N (which is > HN), re-encode.
	var body ethLegacyTransaction
	require.NoError(t, rlp.DecodeBytes(rawBytes, &body))
	body.S = new(big.Int).Set(secp256k1N) // s == N > HN

	tampered, err := rlp.EncodeToBytes(&body)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrHighSSignature, err.(*EthTxError).Code)
}

func TestEthLegacy_NonCanonicalRLPRejected(t *testing.T) {
	// Build valid raw bytes and inject a leading zero into gasPrice using raw RLP manipulation.
	// We splice in 0x82, 0x00, 0x01 (non-canonical encoding of 1) at the gasPrice position.
	//
	// Instead of byte-level surgery, build a struct with a special type that encodes
	// with leading zeros — simpler and more reliable.
	//
	// The RLP for gasPrice is normally a minimal big-endian uint. We craft bytes that
	// represent gasPrice=1 with a leading zero byte: RLP string of 2 bytes [0x00, 0x01].
	// This is 0x82, 0x00, 0x01.

	// Build valid raw bytes first.
	validRaw, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	// Re-encode with a manually constructed body that has leading zeros in gasPrice.
	// We use rlp.RawValue to inject a non-canonical integer.
	type txBodyRaw struct {
		Nonce    uint64
		GasPrice rlp.RawValue // we inject non-canonical bytes here
		GasLimit uint64
		To       *thor.Address `rlp:"nil"`
		Value    *big.Int
		Data     []byte
		V        *big.Int
		R        *big.Int
		S        *big.Int
	}

	// Decode valid tx to get real V, R, S.
	var valid ethLegacyTransaction
	require.NoError(t, rlp.DecodeBytes(validRaw, &valid))

	nonCanonicalGasPrice := []byte{0x82, 0x00, 0x01} // 2-byte string [0x00, 0x01] = non-canonical 1
	tampered, err := rlp.EncodeToBytes(&txBodyRaw{
		Nonce:    valid.Nonce,
		GasPrice: nonCanonicalGasPrice,
		GasLimit: valid.GasLimit,
		To:       valid.To,
		Value:    valid.Value,
		V:        valid.V,
		R:        valid.R,
		S:        valid.S,
	})
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrNonCanonicalRLP, err.(*EthTxError).Code)
}

func TestEthLegacy_OversizedRejected(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().Data(bytes.Repeat([]byte{0xAB}, maxEthTxSize+1)).BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrOversized, err.(*EthTxError).Code)
}

func TestEthLegacy_GasPriceZeroRejected(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().GasPrice(big.NewInt(0)).BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrInvalidField, err.(*EthTxError).Code)
}

func TestEthLegacy_GasLimitZeroRejected(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().GasLimit(0).BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrInvalidField, err.(*EthTxError).Code)
}

func TestEthLegacy_ContractCreation(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().To(nil).BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	assert.Nil(t, ntx.To)
}

func TestEthLegacy_HashIsStable(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx1, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	ntx2, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	assert.Equal(t, ntx1.Hash, ntx2.Hash, "ethTxHash must be bit-for-bit stable across calls")
}

func TestEthLegacy_HashNotEqualSigningHash(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	// Decode the body to get the signing hash via Transaction wrapper.
	var body ethLegacyTransaction
	require.NoError(t, rlp.DecodeBytes(rawBytes, &body))
	signingHash := body.ethSigningHash()

	assert.NotEqual(t, ntx.Hash, signingHash, "ethTxHash must differ from signing hash")
}

func TestEthLegacy_InvalidR(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	var body ethLegacyTransaction
	require.NoError(t, rlp.DecodeBytes(rawBytes, &body))

	// r = 0
	body.R = big.NewInt(0)
	// s must remain valid so we only test r; re-sign to get valid S won't work easily,
	// so we just check the r=0 path specifically.
	tampered, err := rlp.EncodeToBytes(&body)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrInvalidR, err.(*EthTxError).Code)
}

func TestEthLegacy_InvalidS_Zero(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	var body ethLegacyTransaction
	require.NoError(t, rlp.DecodeBytes(rawBytes, &body))
	body.S = big.NewInt(0)
	tampered, err := rlp.EncodeToBytes(&body)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrInvalidS, err.(*EthTxError).Code)
}
