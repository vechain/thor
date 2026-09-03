// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
)

// Benchmark decoding operations for typical timestamp values
func BenchmarkDecode_Timestamp_Standard(b *testing.B) {
	val := uint64(1698000000) // Typical timestamp value
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for b.Loop() {
		var result uint64
		err := rlp.DecodeBytes(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_Timestamp_Optimized(b *testing.B) {
	val := uint64(1698000000) // Typical timestamp value
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for b.Loop() {
		_, err := decodeUint64(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark decoding operations for zero values
func BenchmarkDecode_Zero_Standard(b *testing.B) {
	val := uint64(0)
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for b.Loop() {
		var result uint64
		err := rlp.DecodeBytes(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_Zero_Optimized(b *testing.B) {
	val := uint64(0)
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for b.Loop() {
		_, err := decodeUint64(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark decoding operations for max uint64 values
func BenchmarkDecode_MaxUint64_Standard(b *testing.B) {
	val := uint64(18446744073709551615) // MaxUint64
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for b.Loop() {
		var result uint64
		err := rlp.DecodeBytes(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_MaxUint64_Optimized(b *testing.B) {
	val := uint64(18446744073709551615) // MaxUint64
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for b.Loop() {
		_, err := decodeUint64(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
