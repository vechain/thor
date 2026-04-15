// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEthereumTransaction_Legacy covers construction from a NormalizedEthereumTx of
// type EthLegacy and verifies that the resulting tx.Transaction exposes the correct fields.
func TestNewEthereumTransaction_Legacy(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	norm, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	trx := NewEthereumTransaction(norm, testChainTagVal)

	// Type
	assert.Equal(t, TypeEthLegacy, trx.Type())

	// ID must equal the ethTxHash (Keccak256 of raw bytes), not Blake2b(signingHash,origin).
	assert.Equal(t, norm.Hash, trx.ID())

	// Origin must recover to the test sender address.
	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, norm.Sender, origin)

	// Gas must equal the Ethereum gasLimit.
	assert.Equal(t, norm.GasLimit, trx.Gas())

	// Clauses: exactly one clause with correct To, Value, Data.
	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.Equal(t, norm.To, clauses[0].To())
	assert.Equal(t, norm.Value, clauses[0].Value())
	assert.Empty(t, clauses[0].Data()) // default params carry no data; nil and [] are both empty

	// VeChain stubs.
	assert.Equal(t, testChainTagVal, trx.ChainTag())
	assert.Equal(t, uint32(0), trx.BlockRef().Number()) // blockRef stub = 0
	assert.Equal(t, uint32(maxUint32), trx.Expiration())
	assert.False(t, trx.IsExpired(uint32(maxUint32)), "Ethereum txs must not expire at any uint32 block number")
	assert.Nil(t, trx.DependsOn())
	assert.Equal(t, Features(0), trx.Features(), "no VeChain feature flags on Ethereum txs")
}

// TestNewEthereumTransaction_1559 covers the EIP-1559 path.
func TestNewEthereumTransaction_1559(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	norm, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	trx := NewEthereumTransaction(norm, testChainTagVal)

	assert.Equal(t, TypeEthTyped1559, trx.Type())
	assert.Equal(t, norm.Hash, trx.ID())

	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, norm.Sender, origin)

	assert.Equal(t, norm.GasLimit, trx.Gas())
	assert.Equal(t, norm.MaxFeePerGas, trx.MaxFeePerGas())
	assert.Equal(t, norm.MaxPriorityFeePerGas, trx.MaxPriorityFeePerGas())

	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.Equal(t, norm.To, clauses[0].To())
	assert.Equal(t, norm.Value, clauses[0].Value())
	assert.Empty(t, clauses[0].Data())

	// VeChain stubs — must match EthLegacy values.
	assert.Equal(t, testChainTagVal, trx.ChainTag())
	assert.Equal(t, uint32(0), trx.BlockRef().Number())
	assert.Equal(t, uint32(maxUint32), trx.Expiration())
	assert.False(t, trx.IsExpired(uint32(maxUint32)), "Ethereum txs must not expire at any uint32 block number")
	assert.Nil(t, trx.DependsOn())
	assert.Equal(t, Features(0), trx.Features(), "no VeChain feature flags on Ethereum txs")
}

// TestNewEthereumTransaction_LegacyContractCreation verifies that a nil To (contract
// creation) is preserved through the conversion and produces an IsCreatingContract clause.
func TestNewEthereumTransaction_LegacyContractCreation(t *testing.T) {
	trx, err := defaultEthLegacyBuilder().To(nil).ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)

	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.True(t, clauses[0].IsCreatingContract())
	assert.Nil(t, clauses[0].To())
}

// TestNewEthereumTransaction_1559ContractCreation same for EIP-1559.
func TestNewEthereumTransaction_1559ContractCreation(t *testing.T) {
	trx, err := defaultEth1559Builder().To(nil).ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)

	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.True(t, clauses[0].IsCreatingContract())
}

// TestNewEthereumTransaction_LegacyMarshalRoundtrip verifies that MarshalBinary followed by
// UnmarshalBinary produces a Transaction with identical observable properties.
func TestNewEthereumTransaction_LegacyMarshalRoundtrip(t *testing.T) {
	testData := []byte{0xde, 0xad, 0xbe, 0xef}
	original, err := defaultEthLegacyBuilder().Data(testData).ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)

	encoded, err := original.MarshalBinary()
	require.NoError(t, err)

	// Block-body encoding: first byte must be the 0x52 type marker.
	require.NotEmpty(t, encoded)
	assert.Equal(t, TypeEthLegacy, encoded[0])

	var decoded Transaction
	require.NoError(t, decoded.UnmarshalBinary(encoded))

	assert.Equal(t, TypeEthLegacy, decoded.Type())
	assert.Equal(t, original.ID(), decoded.ID())

	originOrig, err := original.Origin()
	require.NoError(t, err)
	originDecoded, err := decoded.Origin()
	require.NoError(t, err)
	assert.Equal(t, originOrig, originDecoded)

	origClauses := original.Clauses()
	decodedClauses := decoded.Clauses()
	require.Len(t, decodedClauses, 1)
	assert.Equal(t, origClauses[0].To(), decodedClauses[0].To())
	assert.Equal(t, origClauses[0].Value(), decodedClauses[0].Value())
	assert.Equal(t, origClauses[0].Data(), decodedClauses[0].Data())
	assert.Equal(t, original.Gas(), decoded.Gas())
}

// TestNewEthereumTransaction_1559MarshalRoundtrip same for EIP-1559.
func TestNewEthereumTransaction_1559MarshalRoundtrip(t *testing.T) {
	testData := []byte{0xca, 0xfe, 0xba, 0xbe}
	original, err := defaultEth1559Builder().Data(testData).ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)

	encoded, err := original.MarshalBinary()
	require.NoError(t, err)

	// Block-body encoding: first byte must be the 0x02 EIP-1559 type byte.
	require.NotEmpty(t, encoded)
	assert.Equal(t, TypeEthTyped1559, encoded[0])

	var decoded Transaction
	require.NoError(t, decoded.UnmarshalBinary(encoded))

	assert.Equal(t, TypeEthTyped1559, decoded.Type())
	assert.Equal(t, original.ID(), decoded.ID())

	originOrig, err := original.Origin()
	require.NoError(t, err)
	originDecoded, err := decoded.Origin()
	require.NoError(t, err)
	assert.Equal(t, originOrig, originDecoded)

	assert.Equal(t, original.MaxFeePerGas(), decoded.MaxFeePerGas())
	assert.Equal(t, original.MaxPriorityFeePerGas(), decoded.MaxPriorityFeePerGas())
	assert.Equal(t, original.Gas(), decoded.Gas())
}

// TestNewEthereumTransaction_LegacyEffectiveGasPrice verifies that the EIP-1559 formula
// in EffectiveGasPrice returns gasPrice for EthLegacy txs (regardless of baseFee).
func TestNewEthereumTransaction_LegacyEffectiveGasPrice(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	norm, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	trx := NewEthereumTransaction(norm, testChainTagVal)

	// EffectiveGasPrice for EthLegacy should equal gasPrice regardless of baseFee,
	// because min(gasPrice, gasPrice + baseFee) = gasPrice when baseFee ≥ 0.
	baseFee := big.NewInt(1e9) // 1 Gwei
	effective := trx.EffectiveGasPrice(baseFee, nil)
	assert.Equal(t, norm.GasPrice, effective)
}

// TestNewEthereumTransaction_UnsupportedTypePanics verifies the panic guard for unknown types.
func TestNewEthereumTransaction_UnsupportedTypePanics(t *testing.T) {
	norm := &NormalizedEthereumTx{TxType: 0xFF}
	assert.Panics(t, func() { NewEthereumTransaction(norm, 0) })
}

// TestNewEthereumTransaction_Hash verifies that Hash() == ID() == ethTxHash for Ethereum txs,
// preventing the latent bug where unexported fields cause rlp.Encode to produce an empty
// encoding and every Ethereum tx to share the same Hash().
func TestNewEthereumTransaction_Hash(t *testing.T) {
	legacy, err := defaultEthLegacyBuilder().ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)
	assert.Equal(t, legacy.ID(), legacy.Hash(), "EthLegacy: Hash must equal ID (ethTxHash)")

	tx1559, err := defaultEth1559Builder().ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)
	assert.Equal(t, tx1559.ID(), tx1559.Hash(), "Eth1559: Hash must equal ID (ethTxHash)")

	// Two different txs must not share the same Hash.
	assert.NotEqual(t, legacy.Hash(), tx1559.Hash())
}

// TestNewEthereumTransaction_LegacyCopy verifies that copy() deep-copies an EthLegacy tx body:
// mutating the original's underlying data does not affect the copy.
func TestNewEthereumTransaction_LegacyCopy(t *testing.T) {
	original, err := defaultEthLegacyBuilder().ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)

	copied := &Transaction{body: original.body.copy()}

	assert.Equal(t, original.ID(), copied.ID())
	assert.Equal(t, original.Gas(), copied.Gas())

	origClauses := original.Clauses()
	copiedClauses := copied.Clauses()
	require.Len(t, copiedClauses, 1)
	assert.Equal(t, origClauses[0].Value(), copiedClauses[0].Value())

	// Mutation isolation: mutating the original body must not affect the copy.
	original.body.(*ethLegacyTxData).gasPrice.SetInt64(0)
	assert.Equal(t, big.NewInt(20e9), copied.MaxFeePerGas(),
		"copied body must not share gasPrice pointer with original")
}

// TestNewEthereumTransaction_1559Copy same for EIP-1559.
func TestNewEthereumTransaction_1559Copy(t *testing.T) {
	original, err := defaultEth1559Builder().ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)

	copied := &Transaction{body: original.body.copy()}

	assert.Equal(t, original.ID(), copied.ID())
	assert.Equal(t, original.Gas(), copied.Gas())
	assert.Equal(t, original.MaxFeePerGas(), copied.MaxFeePerGas())
	assert.Equal(t, original.MaxPriorityFeePerGas(), copied.MaxPriorityFeePerGas())

	// Mutation isolation: mutating the original body must not affect the copy.
	original.body.(*eth1559TxData).maxFee.SetInt64(0)
	assert.Equal(t, big.NewInt(10e9), copied.MaxFeePerGas(),
		"copied body must not share maxFee pointer with original")
}

// TestNewEthereumTransaction_SetSignaturePanics verifies that calling WithSignature on an
// Ethereum tx (which routes through setSignature) panics with a clear message.
func TestNewEthereumTransaction_SetSignaturePanics(t *testing.T) {
	legacy, err := defaultEthLegacyBuilder().ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)
	assert.Panics(t, func() { legacy.WithSignature(make([]byte, 65)) })

	tx1559, err := defaultEth1559Builder().ChainTag(testChainTagVal).Build(ethTestKey)
	require.NoError(t, err)
	assert.Panics(t, func() { tx1559.WithSignature(make([]byte, 65)) })
}

// TestNewEthereumTransaction_LegacyFeeAliasing verifies that MaxFeePerGas and
// MaxPriorityFeePerGas both return gasPrice for EthLegacy transactions, and that
// successive calls return independent copies (no shared pointer).
func TestNewEthereumTransaction_LegacyFeeAliasing(t *testing.T) {
	rawBytes, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	norm, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	trx := NewEthereumTransaction(norm, testChainTagVal)

	gasPrice := big.NewInt(20e9)
	assert.Equal(t, gasPrice, trx.MaxFeePerGas(), "MaxFeePerGas must equal gasPrice for EthLegacy")
	assert.Equal(t, gasPrice, trx.MaxPriorityFeePerGas(), "MaxPriorityFeePerGas must equal gasPrice for EthLegacy")

	// Each call must allocate an independent copy.
	mfpg1 := trx.MaxFeePerGas()
	mfpg2 := trx.MaxFeePerGas()
	assert.NotSame(t, mfpg1, mfpg2, "MaxFeePerGas must return independent copies per call")
}

// TestNewEthereumTransaction_1559EffectiveGasPrice verifies the EIP-1559 formula
// min(maxFeePerGas, maxPriorityFeePerGas + baseFee) for EthTyped1559.
// Default builder: maxFeePerGas=10 Gwei, maxPriorityFeePerGas=1 Gwei.
func TestNewEthereumTransaction_1559EffectiveGasPrice(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	norm, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	trx := NewEthereumTransaction(norm, testChainTagVal)

	tests := []struct {
		name     string
		baseFee  *big.Int
		expected *big.Int
	}{
		// priority(1e9) + baseFee(2e9) = 3e9 < maxFee(10e9) → uncapped
		{"uncapped", big.NewInt(2e9), big.NewInt(3e9)},
		// priority(1e9) + baseFee(12e9) = 13e9 > maxFee(10e9) → fee-capped
		{"fee_capped", big.NewInt(12e9), big.NewInt(10e9)},
		// priority(1e9) + baseFee(9e9) = 10e9 == maxFee(10e9) → boundary (capped)
		{"boundary", big.NewInt(9e9), big.NewInt(10e9)},
		// priority(1e9) + baseFee(0) = 1e9 → zero base fee
		{"zero_basefee", big.NewInt(0), big.NewInt(1e9)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, trx.EffectiveGasPrice(tc.baseFee, nil))
		})
	}
}

// TestNewEthereumTransaction_EffectivePriorityFeePerGas verifies
// min(maxFeePerGas − baseFee, maxPriorityFeePerGas) for both Ethereum tx types.
func TestNewEthereumTransaction_EffectivePriorityFeePerGas(t *testing.T) {
	legacyRaw, err := defaultEthLegacyBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	legacyNorm, err := NormalizeEthereumTx(legacyRaw, testChainID)
	require.NoError(t, err)
	legacyTrx := NewEthereumTransaction(legacyNorm, testChainTagVal)

	raw1559, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)
	norm1559, err := NormalizeEthereumTx(raw1559, testChainID)
	require.NoError(t, err)
	trx1559 := NewEthereumTransaction(norm1559, testChainTagVal)

	tests := []struct {
		name     string
		trx      *Transaction
		baseFee  *big.Int
		expected *big.Int
	}{
		// EthLegacy: maxFee = maxPriority = gasPrice = 20 Gwei
		// min(20e9 − 5e9, 20e9) = 15e9
		{"legacy_below_gasprice", legacyTrx, big.NewInt(5e9), big.NewInt(15e9)},
		// min(20e9 − 20e9, 20e9) = 0
		{"legacy_at_gasprice", legacyTrx, big.NewInt(20e9), big.NewInt(0)},
		// EthTyped1559: maxFee=10 Gwei, maxPriority=1 Gwei
		// min(10e9 − 2e9, 1e9) = 1e9 (priority-capped)
		{"1559_priority_capped", trx1559, big.NewInt(2e9), big.NewInt(1e9)},
		// min(10e9 − 9e9, 1e9) = 1e9 (boundary, priority-capped)
		{"1559_boundary", trx1559, big.NewInt(9e9), big.NewInt(1e9)},
		// min(10e9 − 10e9, 1e9) = 0 (maxFee consumed by baseFee)
		{"1559_zero_priority", trx1559, big.NewInt(10e9), big.NewInt(0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// legacyTxBaseGasPrice and provedWork are only used for TypeLegacy (VeChain native),
			// not for Ethereum tx types; pass nil for both.
			// Use Cmp instead of Equal: big.Int subtraction producing zero yields
			// abs:{} (empty slice) rather than abs:nil, which trips reflect.DeepEqual.
			got := tc.trx.EffectivePriorityFeePerGas(tc.baseFee, nil, nil)
			assert.Equal(t, 0, tc.expected.Cmp(got), "expected %s, got %s", tc.expected, got)
		})
	}
}

// TestEthLegacyTxData_DecodePreEIP155 verifies that decode rejects a pre-EIP-155
// transaction whose V value is below 35.
func TestEthLegacyTxData_DecodePreEIP155(t *testing.T) {
	// Encode a pre-EIP-155 legacy tx (V=27; no chain-ID protection).
	preEIP155 := &ethLegacyTransaction{
		Nonce:    1,
		GasPrice: big.NewInt(20e9),
		GasLimit: 21000,
		To:       nil,
		Value:    big.NewInt(0),
		Data:     nil,
		V:        big.NewInt(27), // pre-EIP-155 yParity=0
		R:        big.NewInt(1),
		S:        big.NewInt(1),
	}
	encoded, err := rlp.EncodeToBytes(preEIP155)
	require.NoError(t, err)

	var d ethLegacyTxData
	err = d.decode(encoded)
	require.Error(t, err, "decode must reject pre-EIP-155 transactions (V < 35)")
}

// maxUint32 mirrors math.MaxUint32 for use in assertions without an import.
const maxUint32 = 1<<32 - 1

// testChainTagVal is the stub chainTag used across eth_tx_data tests.
const testChainTagVal byte = 0x27
