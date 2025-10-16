package energy

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
)

// Benchmark encoding operations for typical timestamp values
func BenchmarkEncode_Timestamp_Standard(b *testing.B) {
	val := uint64(1698000000) // Typical timestamp value
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rlp.EncodeToBytes(val)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode_Timestamp_Optimized(b *testing.B) {
	val := uint64(1698000000) // Typical timestamp value
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encodeUint64RLP(val)
	}
}

// Benchmark decoding operations for typical timestamp values
func BenchmarkDecode_Timestamp_Standard(b *testing.B) {
	val := uint64(1698000000) // Typical timestamp value
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result uint64
		err := rlp.DecodeBytes(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_Timestamp_Optimized(b *testing.B) {
	val := uint64(1698000000) // Typical timestamp value
	data := encodeUint64RLP(val)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decodeUint64RLP(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark encoding operations for zero values
func BenchmarkEncode_Zero_Standard(b *testing.B) {
	val := uint64(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rlp.EncodeToBytes(val)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode_Zero_Optimized(b *testing.B) {
	val := uint64(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encodeUint64RLP(val)
	}
}

// Benchmark decoding operations for zero values
func BenchmarkDecode_Zero_Standard(b *testing.B) {
	val := uint64(0)
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result uint64
		err := rlp.DecodeBytes(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_Zero_Optimized(b *testing.B) {
	val := uint64(0)
	data := encodeUint64RLP(val)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decodeUint64RLP(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark encoding operations for max uint64 values
func BenchmarkEncode_MaxUint64_Standard(b *testing.B) {
	val := uint64(18446744073709551615) // MaxUint64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rlp.EncodeToBytes(val)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode_MaxUint64_Optimized(b *testing.B) {
	val := uint64(18446744073709551615) // MaxUint64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encodeUint64RLP(val)
	}
}

// Benchmark decoding operations for max uint64 values
func BenchmarkDecode_MaxUint64_Standard(b *testing.B) {
	val := uint64(18446744073709551615) // MaxUint64
	data, _ := rlp.EncodeToBytes(val)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result uint64
		err := rlp.DecodeBytes(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_MaxUint64_Optimized(b *testing.B) {
	val := uint64(18446744073709551615) // MaxUint64
	data := encodeUint64RLP(val)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decodeUint64RLP(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
