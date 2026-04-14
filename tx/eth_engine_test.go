// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeEthereumTx_TypeDetection covers the type-detection switch in
// NormalizeEthereumTx before any per-type decoding occurs.
func TestNormalizeEthereumTx_TypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		raw      []byte
		wantCode EthTxErrorCode
	}{
		{
			name:     "empty bytes",
			raw:      []byte{},
			wantCode: EthErrMalformedEncoding,
		},
		{
			name:     "oversized transaction",
			raw:      bytes.Repeat([]byte{0xAB}, maxEthTxSize+1),
			wantCode: EthErrOversized,
		},
		{
			name:     "EIP-2930 type 0x01 rejected",
			raw:      []byte{0x01},
			wantCode: EthErrUnsupportedTxType,
		},
		{
			name:     "EIP-4844 type 0x03 rejected",
			raw:      []byte{0x03},
			wantCode: EthErrUnsupportedTxTypeEIP4844,
		},
		{
			name:     "VeChain TypeDynamicFee 0x51 rejected",
			raw:      []byte{TypeDynamicFee},
			wantCode: EthErrUnsupportedTxType,
		},
		{
			name:     "unknown type byte 0x04 rejected",
			raw:      []byte{0x04},
			wantCode: EthErrUnsupportedTxType,
		},
		{
			name:     "TypeEthLegacy 0x52 as wire byte rejected",
			raw:      []byte{TypeEthLegacy},
			wantCode: EthErrUnsupportedTxType,
		},
		{
			name:     "EIP-1559 truncated to type byte only",
			raw:      []byte{TypeEthTyped1559},
			wantCode: EthErrMalformedEncoding,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeEthereumTx(tt.raw, testChainID)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, err.(*EthTxError).Code)
		})
	}
}
