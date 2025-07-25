// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

func TestRootHash(t *testing.T) {
	// Test empty transactions slice
	test := []struct {
		name           string
		txs            Transactions
		expectedResult thor.Bytes32
	}{
		{
			name: "empty transactions slice",
			txs:  Transactions{},
			expectedResult: thor.Bytes32{
				0x45,
				0xb0,
				0xcf,
				0xc2,
				0x20,
				0xce,
				0xec,
				0x5b,
				0x7c,
				0x1c,
				0x62,
				0xc4,
				0xd4,
				0x19,
				0x3d,
				0x38,
				0xe4,
				0xeb,
				0xa4,
				0x8e,
				0x88,
				0x15,
				0x72,
				0x9c,
				0xe7,
				0x5f,
				0x9c,
				0xa,
				0xb0,
				0xe4,
				0xc1,
				0xc0,
			},
		},
		{
			name: "non-empty legacy transactions slice",
			txs:  Transactions{GetMockTx(TypeLegacy), GetMockTx(TypeLegacy)},
			expectedResult: thor.Bytes32{
				0x80,
				0x63,
				0x8e,
				0x9b,
				0x4e,
				0x59,
				0xdc,
				0xc9,
				0x63,
				0xf9,
				0x1e,
				0x23,
				0xc2,
				0x52,
				0xa1,
				0x8d,
				0xfe,
				0x5a,
				0x3d,
				0x3d,
				0xdc,
				0x62,
				0x98,
				0xac,
				0x40,
				0x8d,
				0xbe,
				0x74,
				0x64,
				0xc,
				0x57,
				0xb1,
			},
		},
		{
			name: "non-empty dyn fee transactions slice",
			txs:  Transactions{GetMockTx(TypeDynamicFee), GetMockTx(TypeDynamicFee)},
			expectedResult: thor.Bytes32{
				0xbf,
				0x4d,
				0xe7,
				0x2c,
				0x62,
				0xa5,
				0x3c,
				0xb3,
				0x87,
				0x74,
				0xca,
				0x39,
				0xc2,
				0xd2,
				0x28,
				0x5b,
				0xa8,
				0x6,
				0x9,
				0xe4,
				0x24,
				0x6a,
				0xcb,
				0x91,
				0x5b,
				0x5f,
				0xda,
				0x15,
				0x4,
				0x2a,
				0xa7,
				0xcd,
			},
		},
		{
			name: "non-empty mixed transactions slice",
			txs:  Transactions{GetMockTx(TypeLegacy), GetMockTx(TypeDynamicFee), GetMockTx(TypeDynamicFee), GetMockTx(TypeLegacy)},
			expectedResult: thor.Bytes32{
				0x76,
				0x87,
				0xa,
				0x4f,
				0x2f,
				0x50,
				0xa4,
				0xbb,
				0x94,
				0x5f,
				0x9e,
				0xaf,
				0x8f,
				0x7b,
				0xf4,
				0x34,
				0xa4,
				0xa7,
				0x93,
				0xef,
				0x99,
				0x5b,
				0x93,
				0xcc,
				0xd5,
				0x73,
				0x95,
				0x8d,
				0xd6,
				0xc5,
				0x90,
				0x52,
			},
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedResult, tt.txs.RootHash())
		})
	}
}
