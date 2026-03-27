// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func buildTestBlock(txCount int) *Block {
	builder := new(Builder).
		ParentID(thor.Bytes32{1, 2, 3}).
		Timestamp(1234567890).
		GasLimit(10000).
		GasUsed(500).
		TotalScore(100).
		StateRoot(thor.Bytes32{4, 5, 6}).
		ReceiptsRoot(thor.Bytes32{7, 8, 9}).
		BaseFee(big.NewInt(1000)).
		Alpha([]byte{0xaa, 0xbb})

	for range txCount {
		trx := tx.NewBuilder(tx.TypeLegacy).
			Clause(tx.NewClause(&thor.Address{})).
			Build()
		builder.Transaction(trx)
	}

	return builder.Build()
}

func TestDecodeRawBlock_RoundTrip(t *testing.T) {
	blk := buildTestBlock(3)

	data, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	rb, err := DecodeRawBlock(data)
	require.NoError(t, err)

	assert.Equal(t, blk.Header().ParentID(), rb.Header().ParentID())
	assert.Equal(t, blk.Header().Timestamp(), rb.Header().Timestamp())
	assert.Equal(t, blk.Header().GasLimit(), rb.Header().GasLimit())
	assert.Equal(t, blk.Header().GasUsed(), rb.Header().GasUsed())
	assert.Equal(t, blk.Header().TotalScore(), rb.Header().TotalScore())
	assert.Equal(t, blk.Header().StateRoot(), rb.Header().StateRoot())
	assert.Equal(t, blk.Header().ReceiptsRoot(), rb.Header().ReceiptsRoot())
	assert.Equal(t, blk.Header().Number(), rb.Header().Number())

	decoded, err := rb.Decode()
	require.NoError(t, err)

	assert.Equal(t, len(blk.Transactions()), len(decoded.Transactions()))
	assert.Equal(t, blk.Header().TxsRoot(), decoded.Header().TxsRoot())
	assert.Equal(t, blk.Size(), decoded.Size())
}

func TestDecodeRawBlock_NoTransactions(t *testing.T) {
	blk := buildTestBlock(0)

	data, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	rb, err := DecodeRawBlock(data)
	require.NoError(t, err)

	decoded, err := rb.Decode()
	require.NoError(t, err)
	assert.Empty(t, decoded.Transactions())
}

func TestDecodeRawBlock_HeaderIDMatchesOriginal(t *testing.T) {
	blk := buildTestBlock(2)

	data, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	rb, err := DecodeRawBlock(data)
	require.NoError(t, err)

	assert.Equal(t, blk.Header().SigningHash(), rb.Header().SigningHash())
}

func TestDecodeRawBlock_MalformedNotAList(t *testing.T) {
	// A plain string instead of a list
	data := []byte{0x83, 0x01, 0x02, 0x03}

	_, err := DecodeRawBlock(data)
	assert.Error(t, err)
}

func TestDecodeRawBlock_MalformedTruncated(t *testing.T) {
	blk := buildTestBlock(1)
	data, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	// Truncate the data
	_, err = DecodeRawBlock(data[:len(data)/2])
	assert.Error(t, err)
}

func TestDecodeRawBlock_MalformedExtraFields(t *testing.T) {
	blk := buildTestBlock(1)

	// Encode block as a list with 3 items (header, txs, extra)
	data, err := rlp.EncodeToBytes([]any{
		blk.Header(),
		blk.Transactions(),
		[]byte("extra garbage"),
	})
	require.NoError(t, err)

	_, err = DecodeRawBlock(data)
	assert.Error(t, err, "should reject block with extra fields")
}

func TestDecodeRawBlock_MalformedTxsNotAList(t *testing.T) {
	blk := buildTestBlock(0)

	// Encode with txs as a string instead of a list
	data, err := rlp.EncodeToBytes([]any{
		blk.Header(),
		[]byte("not a list"),
	})
	require.NoError(t, err)

	_, err = DecodeRawBlock(data)
	assert.Error(t, err, "should reject block where txs field is not a list")
}

func TestDecodeRawBlock_ViaStream(t *testing.T) {
	blk := buildTestBlock(2)

	data, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	// Decode via rlp.DecodeBytes which uses Stream internally
	var rb RawBlock
	err = rlp.DecodeBytes(data, &rb)
	require.NoError(t, err)

	assert.Equal(t, blk.Header().ParentID(), rb.Header().ParentID())

	decoded, err := rb.Decode()
	require.NoError(t, err)
	assert.Equal(t, len(blk.Transactions()), len(decoded.Transactions()))
}

func TestDecodeRawBlock_MatchesFullDecode(t *testing.T) {
	blk := buildTestBlock(3)

	data, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	// Full decode (original path)
	var fullBlock Block
	require.NoError(t, rlp.DecodeBytes(data, &fullBlock))

	// Two-phase decode
	rb, err := DecodeRawBlock(data)
	require.NoError(t, err)
	twoPhaseBlock, err := rb.Decode()
	require.NoError(t, err)

	assert.Equal(t, fullBlock.Header().ParentID(), twoPhaseBlock.Header().ParentID())
	assert.Equal(t, fullBlock.Header().Timestamp(), twoPhaseBlock.Header().Timestamp())
	assert.Equal(t, fullBlock.Header().GasLimit(), twoPhaseBlock.Header().GasLimit())
	assert.Equal(t, fullBlock.Header().StateRoot(), twoPhaseBlock.Header().StateRoot())
	assert.Equal(t, fullBlock.Header().TxsRoot(), twoPhaseBlock.Header().TxsRoot())
	assert.Equal(t, len(fullBlock.Transactions()), len(twoPhaseBlock.Transactions()))
	assert.Equal(t, fullBlock.Size(), twoPhaseBlock.Size())

	for i, ftx := range fullBlock.Transactions() {
		assert.Equal(t, ftx.ID(), twoPhaseBlock.Transactions()[i].ID())
	}
}

func benchmarkBlockDecode(b *testing.B, txCount int) {
	blk := buildTestBlock(txCount)
	data, err := rlp.EncodeToBytes(blk)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.Run("SinglePhase", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var block Block
			if err := rlp.DecodeBytes(data, &block); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("TwoPhase_Full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rb, err := DecodeRawBlock(data)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := rb.Decode(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("TwoPhase_HeaderOnly", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rb, err := DecodeRawBlock(data)
			if err != nil {
				b.Fatal(err)
			}
			_ = rb.Header()
		}
	})
}

func BenchmarkBlockDecode_0Txs(b *testing.B)   { benchmarkBlockDecode(b, 0) }
func BenchmarkBlockDecode_10Txs(b *testing.B)  { benchmarkBlockDecode(b, 10) }
func BenchmarkBlockDecode_100Txs(b *testing.B) { benchmarkBlockDecode(b, 100) }
func BenchmarkBlockDecode_500Txs(b *testing.B) { benchmarkBlockDecode(b, 500) }
