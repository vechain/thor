// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func getMockReceipt(txType byte) Receipt {
	receipt := Receipt{
		Type: txType,
		ReceiptBody: ReceiptBody{GasUsed: 1000,
			GasPayer: thor.Address{},
			Paid:     big.NewInt(100),
			Reward:   big.NewInt(50),
			Reverted: false,
			Outputs:  []*Output{},
		},
	}
	return receipt
}

func TestReceipt(t *testing.T) {
	var rs Receipts
	fmt.Println(rs.RootHash())

	var txs Transactions
	fmt.Println(txs.RootHash())
}

func TestReceiptStructure(t *testing.T) {
	for _, txType := range []TxType{TypeLegacy, TypeDynamicFee} {
		receipt := getMockReceipt(txType)

		// assert.Equal(t, byte(txType), receipt.Type)
		assert.Equal(t, uint64(1000), receipt.GasUsed)
		assert.Equal(t, thor.Address{}, receipt.GasPayer)
		assert.Equal(t, big.NewInt(100), receipt.Paid)
		assert.Equal(t, big.NewInt(50), receipt.Reward)
		assert.Equal(t, false, receipt.Reverted)
		assert.Equal(t, []*Output{}, receipt.Outputs)
	}
}

func TestEmptyRootHash(t *testing.T) {
	tests := []struct {
		name     string
		receipt1 Receipt
		receipt2 Receipt
	}{
		{
			name:     "LegacyReceipts",
			receipt1: getMockReceipt(TypeLegacy),
			receipt2: getMockReceipt(TypeLegacy),
		},
		{
			name:     "DynamicFeeReceipts",
			receipt1: getMockReceipt(TypeDynamicFee),
			receipt2: getMockReceipt(TypeDynamicFee),
		},
		{
			name:     "MixedReceipts",
			receipt1: getMockReceipt(TypeLegacy),
			receipt2: getMockReceipt(TypeDynamicFee),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receipts := Receipts{
				&tt.receipt1,
				&tt.receipt2,
			}

			rootHash := receipts.RootHash()
			assert.NotEmpty(t, rootHash, "Root hash should be empty")
		})
	}
}

func TestMarshalAndUnmarshalBinary(t *testing.T) {
	for _, txType := range []TxType{TypeLegacy, TypeDynamicFee} {
		originalReceipt := getMockReceipt(txType)

		data, err := originalReceipt.MarshalBinary()
		assert.Nil(t, err)

		var unmarshalledReceipt Receipt
		err = unmarshalledReceipt.UnmarshalBinary(data)
		assert.Nil(t, err)

		assert.Equal(t, originalReceipt, unmarshalledReceipt)
	}
}

func TestEncodeAndDecodeReceipt(t *testing.T) {
	for _, txType := range []TxType{TypeLegacy, TypeDynamicFee} {
		originalReceipt := getMockReceipt(txType)
		receiptBuf := new(bytes.Buffer)
		// Encoding
		err := originalReceipt.EncodeRLP(receiptBuf)
		assert.Nil(t, err)

		s := rlp.NewStream(receiptBuf, 0)
		var decodedReceipt Receipt
		// Decoding
		err = decodedReceipt.DecodeRLP(s)
		assert.Nil(t, err)

		assert.Equal(t, originalReceipt, decodedReceipt)
	}
}

func TestDecodeEmptyTypedReceipt(t *testing.T) {
	input := []byte{0x80}
	var r Receipt
	err := rlp.DecodeBytes(input, &r)
	if err != errEmptyTypedReceipt {
		t.Fatal("wrong error:", err)
	}
}
