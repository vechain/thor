// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"fmt"
	"math/big"
	"os"
	"runtime"
	"slices"
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
		require.NoError(t, rlp.DecodeBytes(makeBlockRLP(t, thor.MaxTxsPerBlock), &blk))
		assert.Len(t, blk.Transactions(), thor.MaxTxsPerBlock)
	})

	t.Run("one over the limit is rejected", func(t *testing.T) {
		var blk Block
		err := rlp.DecodeBytes(makeBlockRLP(t, thor.MaxTxsPerBlock+1), &blk)
		require.Error(t, err)
		assert.Contains(t, err.Error(),
			fmt.Sprintf("tx count exceeds limit: > %d", thor.MaxTxsPerBlock))
	})
}

func TestBlockDecodeRLP_ClauseCountLimit(t *testing.T) {
	clauseErr := fmt.Sprintf("block clause count exceeds limit: > %d", thor.MaxClausesPerBlock)

	for _, txType := range []tx.Type{tx.TypeLegacy, tx.TypeDynamicFee} {
		t.Run(fmt.Sprintf("txType-%d", txType), func(t *testing.T) {
			t.Run("at the limit across txs is accepted", func(t *testing.T) {
				var blk Block
				require.NoError(t, rlp.DecodeBytes(encodeBlockWithClauses(t, txType, 2048, 2048, 2048, 2048), &blk))
				assert.Len(t, blk.Transactions(), 4)
			})

			t.Run("one over the limit across txs is rejected", func(t *testing.T) {
				var blk Block
				err := rlp.DecodeBytes(encodeBlockWithClauses(t, txType, 2048, 2048, 2048, 2048, 1), &blk)
				assert.ErrorContains(t, err, clauseErr)
				assert.Nil(t, blk.header, "a rejected body must not produce a block")
			})
		})
	}

	t.Run("both limits at the boundary are accepted", func(t *testing.T) {
		var blk Block
		data := encodeBlockWithClauses(t, tx.TypeLegacy, spreadClauses(t, thor.MaxClausesPerBlock, thor.MaxTxsPerBlock)...)
		require.NoError(t, rlp.DecodeBytes(data, &blk))
		assert.Len(t, blk.Transactions(), thor.MaxTxsPerBlock)
	})

	// The extra tx crosses both limits at once, so only the check order decides
	// which error is returned.
	t.Run("tx count is checked before clause count", func(t *testing.T) {
		var blk Block
		counts := append(spreadClauses(t, thor.MaxClausesPerBlock, thor.MaxTxsPerBlock), 1)
		err := rlp.DecodeBytes(encodeBlockWithClauses(t, tx.TypeLegacy, counts...), &blk)
		assert.ErrorContains(t, err, fmt.Sprintf("tx count exceeds limit: > %d", thor.MaxTxsPerBlock))
	})
}

// VEC-617 shape: clauses fill a 10 MiB body. Rejection must stop after about
// thor.MaxClausesPerBlock clauses rather than decoding all ~2.6M.
func TestBlockDecodeRLP_ClauseAllocDecoupledFromInput(t *testing.T) {
	const wantErr = "block clause count exceeds limit"
	justOver := allocsForDecodeBlock(t, makeBlockRLPWithClauses(t, 2048, 2048, 2048, 2048, 1), wantErr)
	wayOver := allocsForDecodeBlock(t, makeBlockRLPWithClauses(t, slices.Repeat([]int{thor.MaxClausesPerTx}, 1039)...), wantErr)

	t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)

	assert.Less(t, wayOver, justOver*2,
		"allocation tracks input size; the clause limit is not short-circuiting the decode")
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
	const wantErr = "tx count exceeds limit"
	justOver := allocsForDecodeBlock(t, makeBlockRLP(t, thor.MaxTxsPerBlock+1), wantErr)
	// 953_234 minimal txs is 10,485,758 bytes — 2 short of proto.MaxMsgSize
	// (10 MiB), the largest such payload one p2p message can carry. A literal
	// because block cannot import comm/proto.
	wayOver := allocsForDecodeBlock(t, makeBlockRLP(t, 953_234), wantErr)

	t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)

	assert.Less(t, wayOver, justOver*2,
		"allocation tracks input size; the limit is not short-circuiting the decode")
}

// allocsForDecodeBlock returns the bytes allocated decoding data into a Block.
// The decode must fail with wantErr, or the measurement is not of the limit.
func allocsForDecodeBlock(t *testing.T, data []byte, wantErr string) uint64 {
	t.Helper()
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	var blk Block
	err := rlp.DecodeBytes(data, &blk)

	runtime.ReadMemStats(&m1)
	runtime.KeepAlive(&blk)
	require.ErrorContains(t, err, wantErr)
	return m1.TotalAlloc - m0.TotalAlloc
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
	benchmarkBlockDecodeRLPBound(b, thor.MaxTxsPerBlock)
}

func BenchmarkBlockDecodeRLP_Bounded_JustOver(b *testing.B) {
	benchmarkBlockDecodeRLPBound(b, thor.MaxTxsPerBlock+1)
}

func BenchmarkBlockDecodeRLP_Bounded_WayOver(b *testing.B) {
	benchmarkBlockDecodeRLPBound(b, 953_234)
}

func BenchmarkBlockDecodeRLP_Unbounded_AtLimit(b *testing.B) {
	benchmarkBlockDecodeRLPUnbounded(b, thor.MaxTxsPerBlock)
}

func BenchmarkBlockDecodeRLP_Unbounded_JustOver(b *testing.B) {
	benchmarkBlockDecodeRLPUnbounded(b, thor.MaxTxsPerBlock+1)
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
		assert.Less(t, len(blk.Transactions()), thor.MaxTxsPerBlock,
			"the heaviest historical block is at or over the limit; thor.MaxTxsPerBlock must be raised")
		assert.EqualValues(t, len(raw), blk.Size())
		assert.Equal(t, blk.Header().TxsRoot(), blk.Transactions().RootHash())
	})
}

// Fixture: testnet block 9239749, 199 txs / 7761 clauses — the highest clause
// count in mainnet and testnet history as of 2026-10, at 94.7% of
// thor.MaxClausesPerBlock.
// Verify: GET https://testnet.vechain.org/blocks/9239749
// (id 0x008cfcc5016bb87fb0e919e7c26efeb64a095b286bdbb961fc5e887d3786b5d4)
func TestDecode_HeaviestClauseBlock(t *testing.T) {
	raw, err := os.ReadFile("testdata/maxclause-block-testnet.rlp")
	require.NoError(t, err)

	var blk Block
	require.NoError(t, rlp.DecodeBytes(raw, &blk))

	clauses := 0
	for _, trx := range blk.Transactions() {
		clauses += len(trx.Clauses())
	}
	assert.Equal(t, thor.MustParseBytes32("0x008cfcc5016bb87fb0e919e7c26efeb64a095b286bdbb961fc5e887d3786b5d4"), blk.Header().ID())
	assert.Len(t, blk.Transactions(), 199)
	assert.Equal(t, 7761, clauses)
	assert.Less(t, clauses, thor.MaxClausesPerBlock,
		"the heaviest historical block is at or over the limit; thor.MaxClausesPerBlock must be raised")
	assert.EqualValues(t, len(raw), blk.Size())
	assert.Equal(t, blk.Header().TxsRoot(), blk.Transactions().RootHash())
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

// minimalClauseRLP is an empty clause: [to=nil, value=0, data=""].
var minimalClauseRLP = []byte{0xc3, 0x80, 0x80, 0x80}

// makeBlockRLPWithClauses builds a wire-format block whose body holds one
// minimal legacy tx per entry, each carrying that many empty clauses. Raw bytes
// keep multi-megabyte bodies cheap to build.
func makeBlockRLPWithClauses(t testing.TB, clausesPerTx ...int) []byte {
	t.Helper()
	var txs []byte
	for _, n := range clausesPerTx {
		clauses := slices.Repeat(minimalClauseRLP, n)
		// chainTag, blockRef, expiration | clauses | gasPriceCoef, gas, dependsOn, nonce, reserved, signature
		fields := slices.Concat([]byte{0x80, 0x80, 0x80}, extRLPListHeader(len(clauses)), clauses,
			[]byte{0x80, 0x80, 0x80, 0x80, 0xc0, 0x80})
		txs = append(txs, extRLPListHeader(len(fields))...)
		txs = append(txs, fields...)
	}
	headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
	require.NoError(t, err)
	data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw, append(extRLPListHeader(len(txs)), txs...)})
	require.NoError(t, err)
	return data
}

// encodeBlockWithClauses encodes a block with buildTestBlock's header and one
// unsigned tx of txType per entry, each carrying that many clauses.
func encodeBlockWithClauses(t testing.TB, txType tx.Type, clausesPerTx ...int) []byte {
	t.Helper()
	txs := make(tx.Transactions, 0, len(clausesPerTx))
	for _, n := range clausesPerTx {
		b := tx.NewBuilder(txType)
		for range n {
			b.Clause(tx.NewClause(&thor.Address{}))
		}
		txs = append(txs, b.Build())
	}
	headerRaw, err := rlp.EncodeToBytes(buildTestBlock(0).Header())
	require.NoError(t, err)
	txsRaw, err := rlp.EncodeToBytes(txs)
	require.NoError(t, err)
	data, err := rlp.EncodeToBytes([]rlp.RawValue{headerRaw, txsRaw})
	require.NoError(t, err)
	return data
}

// spreadClauses returns per-tx clause counts that put total clauses into
// txCount txs, filling each tx up to thor.MaxClausesPerTx; the rest carry none.
func spreadClauses(t testing.TB, total, txCount int) []int {
	t.Helper()
	counts := make([]int, txCount)
	for i := range counts {
		counts[i] = min(total, thor.MaxClausesPerTx)
		total -= counts[i]
	}
	require.Zero(t, total, "total clauses do not fit in txCount txs")
	return counts
}
