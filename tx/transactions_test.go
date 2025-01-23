// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func MockTransactions(n int) tx.Transactions {
	txs := make(tx.Transactions, n)
	for i := range txs {
		mockTx := GetMockTx(tx.LegacyTxType)
		txs[i] = mockTx
	}
	return txs
}

func TestRootHash(t *testing.T) {
	// Test empty transactions slice
	emptyTxs := tx.Transactions{}
	emptyRoot := emptyTxs.RootHash()
	assert.Equal(t, emptyTxs.RootHash(), emptyRoot)

	nonEmptyTxs := MockTransactions(2)
	assert.Equal(t, nonEmptyTxs.RootHash(), thor.Bytes32{0x30, 0x9a, 0xd5, 0x4b, 0x28, 0x76, 0x65, 0x52, 0x66, 0x89, 0x7b, 0x19, 0x22, 0x24, 0x63, 0xd8, 0x27, 0xc8, 0x2a, 0xd6, 0x20, 0x17, 0x7a, 0xcf, 0x9a, 0xfa, 0xc, 0xce, 0xff, 0x12, 0x24, 0x48})
}
