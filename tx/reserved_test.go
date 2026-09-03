// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestReservedEncoding(t *testing.T) {
	cases := []struct {
		input    reserved
		expected []byte
	}{
		{reserved{0, nil}, []byte{0xc0}},
		{reserved{8, nil}, []byte{0xc1, 0x08}},
		{reserved{8, []rlp.RawValue{[]byte{0x81}}}, []byte{0xc2, 0x08, 0x81}},
		{reserved{8, []rlp.RawValue{[]byte{0x80}}}, []byte{0xc1, 0x08}}, // trimmed
		{reserved{8, []rlp.RawValue{[]byte{0xc0}}}, []byte{0xc1, 0x08}}, // trimmed
	}

	for i, c := range cases {
		data, err := rlp.EncodeToBytes(&c.input)
		assert.Nil(t, err, "case #%v", i)
		assert.Equal(t, c.expected, data, "case #%v", i)
	}
}

func TestReservedCountLimit(t *testing.T) {
	// MaxUnusedReservedFields+1 unused fields (MaxUnusedReservedFields+2 raws including Features) must be rejected.
	n := MaxUnusedReservedFields + 2
	raws := make([]rlp.RawValue, n)
	for i := range raws {
		raws[i] = rlp.RawValue{0x01}
	}
	data, err := rlp.EncodeToBytes(raws)
	assert.NoError(t, err)

	var r reserved
	err = rlp.DecodeBytes(data, &r)
	assert.ErrorContains(t, err, "reserved field count exceeds limit")

	// Exactly at limit (MaxUnusedReservedFields unused + 1 Features) must pass.
	raws = make([]rlp.RawValue, MaxUnusedReservedFields+1)
	for i := range raws {
		raws[i] = rlp.RawValue{0x01}
	}
	data, err = rlp.EncodeToBytes(raws)
	assert.NoError(t, err)
	err = rlp.DecodeBytes(data, &r)
	assert.NoError(t, err)
}

func TestReservedDecoding(t *testing.T) {
	cases := []struct {
		input    []byte
		expected reserved
	}{
		{[]byte{0xc0}, reserved{0, nil}},
		{[]byte{0xc1, 0x08}, reserved{8, nil}},
		{[]byte{0xc2, 0x08, 0x07}, reserved{8, []rlp.RawValue{[]byte{0x07}}}},
	}

	for i, c := range cases {
		var r reserved
		err := rlp.DecodeBytes(c.input, &r)
		assert.Nil(t, err, "case #%v", i)
		assert.Equal(t, c.expected, r, "case #%v", i)
	}

	var r reserved
	err := rlp.DecodeBytes([]byte{0xc1, 0x80}, &r)
	assert.EqualError(t, err, "invalid reserved fields: not trimmed")

	err = rlp.DecodeBytes([]byte{0xc2, 0x1, 0x80}, &r)
	assert.EqualError(t, err, "invalid reserved fields: not trimmed")
}

// txRLPListHeader builds a canonical RLP long-list header.
func txRLPListHeader(sz int) []byte {
	if sz < 56 {
		return []byte{byte(0xc0 + sz)}
	}
	var lb []byte
	for s := sz; s > 0; s >>= 8 {
		lb = append([]byte{byte(s)}, lb...)
	}
	return append([]byte{0xf7 + byte(len(lb))}, lb...)
}

// makeTxWithReserved builds a legacy transaction whose Reserved field holds n
// empty strings. Reserved is the amplification point.
func makeTxWithReserved(n int) []byte {
	reservedContent := make([]byte, n)
	for i := range reservedContent {
		reservedContent[i] = 0x80
	}
	reserved := append(txRLPListHeader(len(reservedContent)), reservedContent...)

	var body []byte
	body = append(body, 0x80, 0x80, 0x80)       // ChainTag, BlockRef, Expiration
	body = append(body, 0xc0)                   // Clauses = []
	body = append(body, 0x80, 0x80, 0x80, 0x80) // GasPriceCoef, Gas, DependsOn, Nonce
	body = append(body, reserved...)            // Reserved
	body = append(body, 0x80)                   // Signature

	return append(txRLPListHeader(len(body)), body...)
}

func allocsForDecodeTx(data []byte) uint64 {
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	var trx Transaction
	_ = rlp.DecodeBytes(data, &trx) // expected to fail; we only measure allocations

	runtime.ReadMemStats(&m1)
	runtime.KeepAlive(&trx) // &: Transaction embeds sync/atomic fields; copying it trips go vet copylocks
	return m1.TotalAlloc - m0.TotalAlloc
}

func TestReservedDecodeRLP_AllocDecoupledFromInput(t *testing.T) {
	justOver := allocsForDecodeTx(makeTxWithReserved(MaxUnusedReservedFields + 2))
	wayOver := allocsForDecodeTx(makeTxWithReserved(4_000_000))

	t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)

	assert.Less(t, wayOver, uint64(64*1024),
		"allocation tracks input size; the limit is still being enforced after materialization")
}

func BenchmarkReservedDecodeRLP_AtLimit(b *testing.B) {
	data := makeTxWithReserved(MaxUnusedReservedFields + 1)
	b.ReportAllocs()
	for b.Loop() {
		var trx Transaction
		_ = rlp.DecodeBytes(data, &trx)
	}
}

func BenchmarkReservedDecodeRLP_OverLimit(b *testing.B) {
	data := makeTxWithReserved(MaxUnusedReservedFields + 2)
	b.ReportAllocs()
	for b.Loop() {
		var trx Transaction
		_ = rlp.DecodeBytes(data, &trx)
	}
}
