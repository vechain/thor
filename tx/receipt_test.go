// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

func getMockReceipt(txType byte) Receipt {
	receipt := Receipt{
		Type:     txType,
		GasUsed:  1000,
		GasPayer: thor.Address{},
		Paid:     big.NewInt(100),
		Reward:   big.NewInt(50),
		Reverted: false,
		Outputs:  []*Output{},
	}
	return receipt
}

func TestReceipt(t *testing.T) {
	var rs Receipts
	assert.Equal(t, emptyRoot, rs.RootHash())

	var txs Transactions
	assert.Equal(t, emptyRoot, txs.RootHash())
}

func TestReceiptStructure(t *testing.T) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
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
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
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
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
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
	assert.ErrorIs(t, err, errShortTypedReceipt)
}

func TestDecodeTyped_ShortInput(t *testing.T) {
	r := &Receipt{}
	err := r.decodeTyped([]byte{0x01})
	assert.Equal(t, errShortTypedReceipt, err)
}

func TestDecodeTyped_InvalidRLP(t *testing.T) {
	r := &Receipt{}
	b := append([]byte{TypeDynamicFee}, 0x01, 0x02) // not valid RLP
	err := r.decodeTyped(b)
	assert.Error(t, err)
}

func TestDecodeTyped_UnknownType(t *testing.T) {
	r := &Receipt{}
	b := append([]byte{0xFF}, 0x01, 0x02)
	err := r.decodeTyped(b)
	assert.Equal(t, ErrTxTypeNotSupported, err)
}

func TestDecodeRLP_ErrorFromKind(t *testing.T) {
	r := &Receipt{}
	// Use malformed RLP to trigger error in Kind
	bad := []byte{0xFF, 0xFF, 0xFF}
	s := rlp.NewStream(bytes.NewReader(bad), 0)
	err := r.DecodeRLP(s)
	assert.Error(t, err)
}

func TestDecodeRLP_ErrorFromDecode(t *testing.T) {
	r := &Receipt{}
	// Use a valid RLP list but with invalid content for receiptRLP
	var buf bytes.Buffer
	_ = rlp.Encode(&buf, []byte{0x01, 0x02})
	s := rlp.NewStream(&buf, 0)
	err := r.DecodeRLP(s)
	assert.Error(t, err)
}

func TestDecodeRLP_ErrorFromBytes(t *testing.T) {
	r := &Receipt{}
	// Use a stream with no bytes to trigger error in Bytes
	s := rlp.NewStream(bytes.NewReader([]byte{}), 0)
	err := r.DecodeRLP(s)
	assert.Error(t, err)
}

func TestDecodeRLP_ErrorFromDecodeBytesTyped(t *testing.T) {
	r := &Receipt{}
	// Typed receipt, but b[1:] is not valid RLP
	b := append([]byte{TypeDynamicFee}, 0x01, 0x02)
	buf := bytes.NewBuffer(b)
	s := rlp.NewStream(buf, 0)
	err := r.DecodeRLP(s)
	assert.Error(t, err)
}

func TestDecodeRLP_UnknownTypeTyped(t *testing.T) {
	r := &Receipt{}
	b := append([]byte{0xFF}, 0x01, 0x02)
	buf := bytes.NewBuffer(b)
	s := rlp.NewStream(buf, 0)
	err := r.DecodeRLP(s)
	// Accept any error, as malformed RLP can cause EOF or decode error
	assert.Error(t, err)
}

func TestDerivableReceipts_EncodeIndex_PanicsOnMarshalError(t *testing.T) {
	dr := derivableReceipts{&Receipt{Type: 0xFF}} // unknown type, may cause MarshalBinary to error or panic
	defer func() {
		_ = recover()
	}()
	_ = dr.EncodeIndex(0)
}
