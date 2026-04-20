// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEthBuilder_NilKeyRejected verifies that BuildRaw returns an error for a nil key.
func TestEthBuilder_NilKeyRejected(t *testing.T) {
	_, err := defaultEth1559Builder().BuildRaw(nil)
	require.Error(t, err)
}

// TestEthBuilder_UnsupportedTypePanics verifies that NewEthBuilder panics for
// types other than TypeEthTyped1559.
func TestEthBuilder_UnsupportedTypePanics(t *testing.T) {
	assert.Panics(t, func() { NewEthBuilder(TypeLegacy) })
	assert.Panics(t, func() { NewEthBuilder(TypeDynamicFee) })
	assert.Panics(t, func() { NewEthBuilder(0xFF) })
}

// TestEthBuilder_MutatingInputAfterSet verifies that mutating a *big.Int after
// passing it to a setter does not affect the built transaction.
func TestEthBuilder_MutatingInputAfterSet(t *testing.T) {
	fee := big.NewInt(20e9)
	b := NewEthBuilder(TypeEthTyped1559).
		ChainID(testChainID).
		Nonce(5).
		MaxFeePerGas(fee).
		MaxPriorityFeePerGas(big.NewInt(1e9)).
		GasLimit(21000)

	// Mutate the caller's pointer after handing it to the builder.
	fee.SetInt64(0)

	rawBytes, err := b.BuildRaw(ethTestKey)
	require.NoError(t, err, "mutating the input *big.Int must not affect the built tx")

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(20e9), ntx.MaxFeePerGas, "maxFeePerGas must reflect the value at Set time, not after mutation")
}

// TestEthBuilder_BuildErrorPropagates verifies that Build() returns an error when
// NormalizeEthereumTx rejects the encoded bytes due to invalid field values.
func TestEthBuilder_BuildErrorPropagates(t *testing.T) {
	tests := []struct {
		name    string
		builder *EthBuilder
	}{
		// EthTyped1559: maxFeePerGas defaults to zero → "maxFeePerGas must be > 0".
		{
			"1559_maxFee_zero",
			NewEthBuilder(TypeEthTyped1559).ChainID(testChainID).GasLimit(21000).MaxPriorityFeePerGas(big.NewInt(1e9)),
		},
		// EthTyped1559: gasLimit zero → "gasLimit must be > 0".
		{
			"1559_gasLimit_zero",
			NewEthBuilder(TypeEthTyped1559).ChainID(testChainID).MaxFeePerGas(big.NewInt(10e9)).MaxPriorityFeePerGas(big.NewInt(1e9)),
		},
		// EthTyped1559: maxPriorityFeePerGas > maxFeePerGas → "maxPriorityFeePerGas must be ≤ maxFeePerGas".
		{
			"1559_priority_exceeds_maxFee",
			NewEthBuilder(TypeEthTyped1559).ChainID(testChainID).GasLimit(21000).MaxFeePerGas(big.NewInt(1e9)).MaxPriorityFeePerGas(big.NewInt(2e9)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.builder.Build(ethTestKey)
			require.Error(t, err, "Build must propagate NormalizeEthereumTx validation errors")
		})
	}
}
