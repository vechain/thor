// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"reflect"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

func makeClausesRLP(n int) []byte {
	clauses := make([]*Clause, n)
	for i := range clauses {
		clauses[i] = NewClause(nil).WithValue(big.NewInt(int64(i)))
	}
	data, err := rlp.EncodeToBytes(clauses)
	if err != nil {
		panic(err)
	}
	return data
}

// Benchmarks for Clauses (custom type with MaxClausesPerTx enforcement).

func BenchmarkClausesDecodeRLP_Few(b *testing.B) {
	data := makeClausesRLP(3)
	b.ResetTimer()
	for b.Loop() {
		var cs Clauses
		_ = rlp.DecodeBytes(data, &cs)
	}
}

func BenchmarkClausesDecodeRLP_Some20(b *testing.B) {
	data := makeClausesRLP(20)
	b.ResetTimer()
	for b.Loop() {
		var cs Clauses
		_ = rlp.DecodeBytes(data, &cs)
	}
}

func BenchmarkClausesDecodeRLP_AtLimit(b *testing.B) {
	data := makeClausesRLP(MaxClausesPerTx)
	b.ResetTimer()
	for b.Loop() {
		var cs Clauses
		_ = rlp.DecodeBytes(data, &cs)
	}
}

func BenchmarkClausesDecodeRLP_OverLimit(b *testing.B) {
	data := makeClausesRLP(MaxClausesPerTx + 1)
	b.ResetTimer()
	for b.Loop() {
		var cs Clauses
		_ = rlp.DecodeBytes(data, &cs)
	}
}

// Benchmarks for plain []*Clause (no limit enforcement) — baseline for comparison.

func BenchmarkRawClauseSliceDecodeRLP_Few(b *testing.B) {
	data := makeClausesRLP(3)
	b.ResetTimer()
	for b.Loop() {
		var cs []*Clause
		_ = rlp.DecodeBytes(data, &cs)
	}
}

func BenchmarkRawClauseSliceDecodeRLP_Some20(b *testing.B) {
	data := makeClausesRLP(20)
	b.ResetTimer()
	for b.Loop() {
		var cs []*Clause
		_ = rlp.DecodeBytes(data, &cs)
	}
}

func BenchmarkRawClauseSliceDecodeRLP_AtLimit(b *testing.B) {
	data := makeClausesRLP(MaxClausesPerTx)
	b.ResetTimer()
	for b.Loop() {
		var cs []*Clause
		_ = rlp.DecodeBytes(data, &cs)
	}
}

func BenchmarkRawClauseSliceDecodeRLP_OverLimit(b *testing.B) {
	data := makeClausesRLP(MaxClausesPerTx + 1)
	b.ResetTimer()
	for b.Loop() {
		var cs []*Clause
		_ = rlp.DecodeBytes(data, &cs)
	}
}

// makeClauseList encodes a list of n minimal clauses (0xc3 0x80 0x80 0x80).
func makeClauseList(n int) []byte {
	content := make([]byte, 0, n*4)
	for range n {
		content = append(content, 0xc3, 0x80, 0x80, 0x80)
	}
	return append(txRLPListHeader(len(content)), content...)
}

func allocsForDecodeClauses(data []byte) uint64 {
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	var cs Clauses
	_ = rlp.DecodeBytes(data, &cs) // expected to fail; we only measure allocations

	runtime.ReadMemStats(&m1)
	runtime.KeepAlive(cs)
	return m1.TotalAlloc - m0.TotalAlloc
}

func TestClausesDecodeRLP_AllocDecoupledFromInput(t *testing.T) {
	justOver := allocsForDecodeClauses(makeClauseList(MaxClausesPerTx + 1))
	// 2.5M minimal clauses = 10 MB on the wire
	wayOver := allocsForDecodeClauses(makeClauseList(2_500_000))

	t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)

	// Both are rejected at clause 2501. After the fix the 10 MB input must not
	// cost 10 MB; the ceiling is 2501 Clause structs plus stream buffering.
	assert.Less(t, justOver, uint64(1024*1024))
	assert.Less(t, wayOver, uint64(1024*1024),
		"s.Raw() still copies the whole input before the count is checked")
}

func TestClauseTo(t *testing.T) {
	var toAddress thor.Address
	copy(toAddress[:], []byte{0xde, 0xad, 0xbe, 0xef})

	clause := &Clause{
		body: clauseBody{
			To: &toAddress,
		},
	}

	result := clause.To()

	// The result should not be nil and should match the mock address
	assert.NotNil(t, result)
	assert.Equal(t, toAddress, *result)

	// Test the case where 'To' is nil
	clause.body.To = nil
	result = clause.To()

	// The result should be nil
	assert.Nil(t, result)
}

func TestClauseValue(t *testing.T) {
	expectedValue := big.NewInt(100) // Mock value

	clause := &Clause{
		body: clauseBody{
			Value: expectedValue,
		},
	}

	result := clause.Value()

	// The result should not be nil and should match the mock value
	assert.NotNil(t, result)
	assert.Equal(t, 0, expectedValue.Cmp(result))
}

func TestClauseData(t *testing.T) {
	expectedData := []byte{0x01, 0x02, 0x03} // Mock data

	clause := &Clause{
		body: clauseBody{
			Data: expectedData,
		},
	}

	result := clause.Data()

	// The result should not be nil and should match the mock data
	assert.NotNil(t, result)
	assert.True(t, reflect.DeepEqual(expectedData, result))
}
