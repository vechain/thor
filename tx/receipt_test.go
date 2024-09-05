// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func getMockReceipt() tx.Receipt {
	receipt := tx.Receipt{
		GasUsed:  1000,
		GasPayer: thor.Address{},
		Paid:     big.NewInt(100),
		Reward:   big.NewInt(50),
		Reverted: false,
		Outputs:  []*tx.Output{},
	}
	return receipt
}

func TestReceipt(t *testing.T) {
	var rs tx.Receipts
	fmt.Println(rs.RootHash())

	var txs tx.Transactions
	fmt.Println(txs.RootHash())
}

func TestReceiptStructure(t *testing.T) {
	receipt := getMockReceipt()

	assert.Equal(t, uint64(1000), receipt.GasUsed)
	assert.Equal(t, thor.Address{}, receipt.GasPayer)
	assert.Equal(t, big.NewInt(100), receipt.Paid)
	assert.Equal(t, big.NewInt(50), receipt.Reward)
	assert.Equal(t, false, receipt.Reverted)
	assert.Equal(t, []*tx.Output{}, receipt.Outputs)
}

func TestEmptyRootHash(t *testing.T) {
	receipt1 := getMockReceipt()
	receipt2 := getMockReceipt()

	receipts := tx.Receipts{
		&receipt1,
		&receipt2,
	}

	rootHash := receipts.RootHash()
	assert.NotEqual(t, thor.Bytes32{}, rootHash, "Root hash should not be empty")
}
