// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"fmt"
	"math/big"
	"os"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vrf"
)

func TestBlockProof(t *testing.T) {
	tx1 := tx.NewBuilder(tx.TypeLegacy).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := tx.NewBuilder(tx.TypeDynamicFee).Clause(tx.NewClause(nil)).Build()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")
	alpha := thor.MustParseBytes32("0x68abc4fe6b911dd388eac9252513071dd4edea83e183c4b477dc65dd59359c2c")

	var (
		emptyRoot   = thor.BytesToBytes32([]byte("0"))
		beneficiary = thor.BytesToAddress([]byte("abc"))
	)

	blk := new(Builder).
		GasUsed(1000).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(14000).
		TotalScore(101).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(1761554386318816000).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		ParentID(emptyRoot).
		Alpha(alpha.Bytes()).
		Beneficiary(beneficiary).
		Build()

	key, _ := crypto.HexToECDSA(privKey)
	ec, err := crypto.Sign(blk.Header().SigningHash().Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}

	_, proof, err := vrf.Prove(key, alpha.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	sig, err := NewComplexSignature(ec, proof)
	if err != nil {
		t.Fatal(err)
	}
	blk = blk.WithSignature(sig)

	_, err = blk.Header().Beta()
	assert.Nil(t, err)

	newProof := make([]byte, len(proof))

	copy(newProof, proof[0:33])
	sig, err = NewComplexSignature(ec, newProof)
	if err != nil {
		t.Fatal(err)
	}
	blk = blk.WithSignature(sig)

	_, err = blk.Header().Beta()
	assert.ErrorContains(t, err, "invalid proof: value c is zero")

	copy(newProof, proof[0:47])
	sig, err = NewComplexSignature(ec, newProof)
	if err != nil {
		t.Fatal(err)
	}
	blk = blk.WithSignature(sig)

	_, err = blk.Header().Beta()
	assert.ErrorContains(t, err, "invalid proof: value s is zero")
}

func TestBlockDecodeRLP(t *testing.T) {
	header := &Header{}

	var blk Block

	encoded, err := rlp.EncodeToBytes([]any{})
	if err != nil {
		t.Fatal(err)
	}
	err = rlp.DecodeBytes(encoded, &blk)
	assert.ErrorContains(t, err, "rlp: too few elements for block")

	encoded, err = rlp.EncodeToBytes([]any{header})
	if err != nil {
		t.Fatal(err)
	}
	err = rlp.DecodeBytes(encoded, &blk)
	assert.ErrorContains(t, err, "rlp: too few elements for block")

	tx1 := tx.NewBuilder(tx.TypeLegacy).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()

	encoded, err = rlp.EncodeToBytes([]any{header, tx.Transactions{tx1}, tx.Transactions{}})
	if err != nil {
		t.Fatal(err)
	}
	err = rlp.DecodeBytes(encoded, &blk)
	assert.ErrorContains(t, err, "rlp: too many elements for block")
}

// Block.DecodeRLP is the only path that decodes a block body: MsgNewBlock,
// announcement fetches and sync all route through it.
func TestBlockDecodeRLP_TxCountLimit(t *testing.T) {
	t.Run("empty tx list is accepted", func(t *testing.T) {
		var blk Block
		require.NoError(t, rlp.DecodeBytes(makeBlockRLP(t, 0), &blk))
		assert.Empty(t, blk.Transactions())
	})

	t.Run("single tx is accepted", func(t *testing.T) {
		var blk Block
		require.NoError(t, rlp.DecodeBytes(makeBlockRLP(t, 1), &blk))
		assert.Len(t, blk.Transactions(), 1)
	})

	t.Run("at the limit is accepted", func(t *testing.T) {
		var blk Block
		require.NoError(t, rlp.DecodeBytes(makeBlockRLP(t, MaxTxsPerBlock), &blk))
		assert.Len(t, blk.Transactions(), MaxTxsPerBlock)
	})

	t.Run("one over the limit is rejected", func(t *testing.T) {
		var blk Block
		err := rlp.DecodeBytes(makeBlockRLP(t, MaxTxsPerBlock+1), &blk)
		require.Error(t, err)
		assert.Contains(t, err.Error(),
			fmt.Sprintf("tx count exceeds limit: > %d", MaxTxsPerBlock))
	})
}

// Block.DecodeRLP hand-rolls the outer [header, txs] list, so "exactly two
// elements" is no longer free. The first two cases also guard against
// returning the rlp.EOL sentinel, which an enclosing list decoder misreads
// as end-of-list — see the last subtest.
func TestBlockDecodeRLP_OuterListElementCount(t *testing.T) {
	cases := []struct {
		name        string
		build       func(t *testing.T) []byte
		notEOL      bool
		errContains string
	}{
		{
			name: "outer list has only header (missing txs)",
			build: func(t *testing.T) []byte {
				headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
				require.NoError(t, err)
				data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw})
				require.NoError(t, err)
				return data
			},
			notEOL: true,
		},
		{
			name:   "empty outer list",
			build:  func(t *testing.T) []byte { return []byte{0xc0} },
			notEOL: true,
		},
		{
			name: "outer list has three elements",
			build: func(t *testing.T) []byte {
				headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
				require.NoError(t, err)
				extraRaw, err := rlp.EncodeToBytes([]byte("extra garbage"))
				require.NoError(t, err)
				data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw, makeTxsRLP(1), extraRaw})
				require.NoError(t, err)
				return data
			},
			errContains: "rlp: too many elements for block",
		},
		{
			name:  "top-level payload is not a list",
			build: func(t *testing.T) []byte { return []byte{0x83, 0x01, 0x02, 0x03} },
		},
		{
			name: "header field is not a list",
			build: func(t *testing.T) []byte {
				notListRaw, err := rlp.EncodeToBytes([]byte("not a header"))
				require.NoError(t, err)
				data, err := rlp.EncodeToBytes([]rlp.RawValue{notListRaw, makeTxsRLP(0)})
				require.NoError(t, err)
				return data
			},
		},
		{
			name: "header field is truncated",
			build: func(t *testing.T) []byte {
				headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
				require.NoError(t, err)
				truncated := rlp.RawValue(headerRaw[:len(headerRaw)/2])
				data, err := rlp.EncodeToBytes([]rlp.RawValue{truncated, makeTxsRLP(0)})
				require.NoError(t, err)
				return data
			},
		},
		{
			name: "txs field is not a list",
			build: func(t *testing.T) []byte {
				headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
				require.NoError(t, err)
				notListRaw, err := rlp.EncodeToBytes([]byte("not a list"))
				require.NoError(t, err)
				data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw, notListRaw})
				require.NoError(t, err)
				return data
			},
		},
		{
			name: "a tx element inside the txs list is malformed",
			build: func(t *testing.T) []byte {
				headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
				require.NoError(t, err)
				malformedTx, err := rlp.EncodeToBytes([]byte("not a tx"))
				require.NoError(t, err)
				content := append(append([]byte{}, minimalTxRLP...), malformedTx...)
				txsRaw := append(extRLPListHeader(len(content)), content...)
				data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw, txsRaw})
				require.NoError(t, err)
				return data
			},
		},
		{
			name: "trailing bytes after the block",
			build: func(t *testing.T) []byte {
				data := makeBlockRLP(t, 1)
				return append(data, 0x00)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := c.build(t)
			var blk Block
			err := rlp.DecodeBytes(data, &blk)
			require.Error(t, err)
			if c.notEOL {
				assert.NotErrorIs(t, err, rlp.EOL)
			}
			if c.errContains != "" {
				assert.ErrorContains(t, err, c.errContains)
			}
		})
	}

	// A Block decoded as an element of an outer list must be rejected, not
	// silently dropped: rlp's slice decoder treats EOL as end-of-list.
	t.Run("malformed block inside an outer list is rejected", func(t *testing.T) {
		good := makeBlockRLP(t, 1)
		headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
		require.NoError(t, err)
		headerOnly, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw})
		require.NoError(t, err)
		data, err := rlp.EncodeToBytes([]rlp.RawValue{good, headerOnly})
		require.NoError(t, err)

		var blks []*Block
		err = rlp.DecodeBytes(data, &blks)
		require.Error(t, err)
	})
}

// Once the limit is exceeded, allocation must not track input size: rejecting
// a list far beyond the limit costs about what rejecting one just over it does.
func TestBlockDecodeRLP_AllocDecoupledFromInput(t *testing.T) {
	measure := func(data []byte) uint64 {
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)

		var blk Block
		_ = rlp.DecodeBytes(data, &blk) // expected to fail; we only measure allocations

		runtime.ReadMemStats(&m1)
		runtime.KeepAlive(&blk)
		return m1.TotalAlloc - m0.TotalAlloc
	}

	justOver := measure(makeBlockRLP(t, MaxTxsPerBlock+1))
	// 953_234 minimal txs is 10,485,758 bytes — 2 short of proto.MaxMsgSize
	// (10 MiB), the largest such payload one p2p message can carry. A literal
	// because block cannot import comm/proto.
	wayOver := measure(makeBlockRLP(t, 953_234))

	t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)

	assert.Less(t, wayOver, justOver*2,
		"allocation tracks input size; the limit is not short-circuiting the decode")
}

// unboundedBlockPayload is the struct shape Block.DecodeRLP used to decode
// into, whose []*tx.Transaction falls back to the default slice decoder.
type unboundedBlockPayload struct {
	Header Header
	Txs    []*tx.Transaction
}

func benchmarkBlockDecodeRLPBound(b *testing.B, txCount int) {
	b.Helper()
	data := makeBlockRLP(b, txCount)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var blk Block
		_ = rlp.DecodeBytes(data, &blk)
	}
}

func benchmarkBlockDecodeRLPUnbounded(b *testing.B, txCount int) {
	b.Helper()
	data := makeBlockRLP(b, txCount)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var payload unboundedBlockPayload
		_ = rlp.DecodeBytes(data, &payload)
	}
}

func BenchmarkBlockDecodeRLP_Bounded_AtLimit(b *testing.B) {
	benchmarkBlockDecodeRLPBound(b, MaxTxsPerBlock)
}

func BenchmarkBlockDecodeRLP_Bounded_JustOver(b *testing.B) {
	benchmarkBlockDecodeRLPBound(b, MaxTxsPerBlock+1)
}

func BenchmarkBlockDecodeRLP_Bounded_WayOver(b *testing.B) {
	benchmarkBlockDecodeRLPBound(b, 953_234)
}

func BenchmarkBlockDecodeRLP_Unbounded_AtLimit(b *testing.B) {
	benchmarkBlockDecodeRLPUnbounded(b, MaxTxsPerBlock)
}

func BenchmarkBlockDecodeRLP_Unbounded_JustOver(b *testing.B) {
	benchmarkBlockDecodeRLPUnbounded(b, MaxTxsPerBlock+1)
}

func BenchmarkBlockDecodeRLP_Unbounded_WayOver(b *testing.B) {
	benchmarkBlockDecodeRLPUnbounded(b, 953_234)
}

// Fixture: mainnet block 16577101, 809 txs — the highest tx count in mainnet
// history as of 2026-08. Real txs exercise signature, clause and reserved
// decoding that synthetic minimal-tx lists cannot.
// Verify: GET https://mainnet.vechain.org/blocks/16577101
// (id 0x00fcf24d9e655af6763093adc2c7366a6663db511a7647cca7ddd1fb300be73c)
func TestDecode_HeaviestMainnetBlock(t *testing.T) {
	raw, err := os.ReadFile("testdata/maxtx-block-mainnet.rlp")
	require.NoError(t, err)

	t.Run("Block.DecodeRLP", func(t *testing.T) {
		var blk Block
		require.NoError(t, rlp.DecodeBytes(raw, &blk))
		t.Logf("height=%d txs=%d", blk.Header().Number(), len(blk.Transactions()))
		assert.Less(t, len(blk.Transactions()), MaxTxsPerBlock,
			"the heaviest historical block is at or over the limit; MaxTxsPerBlock must be raised")
		assert.EqualValues(t, len(raw), blk.Size())
		assert.Equal(t, blk.Header().TxsRoot(), blk.Transactions().RootHash())
	})
}

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

// benchmarkBlockDecode covers the success path with realistic tx shapes;
// BenchmarkBlockDecodeRLP_* uses synthetic minimal txs to probe the bound.
func benchmarkBlockDecode(b *testing.B, txCount int) {
	blk := buildTestBlock(txCount)
	data, err := rlp.EncodeToBytes(blk)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var blk Block
		if err := rlp.DecodeBytes(data, &blk); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockDecode_0Txs(b *testing.B)   { benchmarkBlockDecode(b, 0) }
func BenchmarkBlockDecode_10Txs(b *testing.B)  { benchmarkBlockDecode(b, 10) }
func BenchmarkBlockDecode_100Txs(b *testing.B) { benchmarkBlockDecode(b, 100) }
func BenchmarkBlockDecode_500Txs(b *testing.B) { benchmarkBlockDecode(b, 500) }

// minimalTxRLP is the smallest RLP that decodes into a legacy Transaction: all
// fields zero, 11 bytes. tx builds the same shape in makeTxWithReserved(0), but
// unexported helpers cannot cross package boundaries.
var minimalTxRLP = []byte{0xca, 0x80, 0x80, 0x80, 0xc0, 0x80, 0x80, 0x80, 0x80, 0xc0, 0x80}

// Reuse extRLPListHeader from block/extension_test.go — same package, do NOT add
// another list-header helper here.

func makeTxsRLP(n int) []byte {
	content := make([]byte, 0, n*len(minimalTxRLP))
	for range n {
		content = append(content, minimalTxRLP...)
	}
	return append(extRLPListHeader(len(content)), content...)
}

// makeBlockRLP builds a wire-format block [header, txs] with a valid header
// from buildTestBlock and a synthetic tx list of the given length.
// Takes testing.TB so it can also be called from benchmarks.
func makeBlockRLP(t testing.TB, txCount int) []byte {
	t.Helper()
	headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
	require.NoError(t, err)
	data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw, makeTxsRLP(txCount)})
	require.NoError(t, err)
	return data
}
