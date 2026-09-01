// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"math/bits"
	"testing"

	"github.com/vechain/thor/v2/thor"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

func TestJumpDestAnalysis(t *testing.T) {
	tests := []struct {
		code  []byte
		exp   byte
		which int
	}{
		{[]byte{byte(PUSH1), 0x01, 0x01, 0x01}, 0b0000_0010, 0},
		{[]byte{byte(PUSH1), byte(PUSH1), byte(PUSH1), byte(PUSH1)}, 0b0000_1010, 0},
		{[]byte{0x00, byte(PUSH1), 0x00, byte(PUSH1), 0x00, byte(PUSH1), 0x00, byte(PUSH1)}, 0b0101_0100, 0},
		{
			[]byte{byte(PUSH8), byte(PUSH8), byte(PUSH8), byte(PUSH8), byte(PUSH8), byte(PUSH8), byte(PUSH8), byte(PUSH8), 0x01, 0x01, 0x01},
			bits.Reverse8(0x7F),
			0,
		},
		{[]byte{byte(PUSH8), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b0000_0001, 1},
		{[]byte{0x01, 0x01, 0x01, 0x01, 0x01, byte(PUSH2), byte(PUSH2), byte(PUSH2), 0x01, 0x01, 0x01}, 0b1100_0000, 0},
		{[]byte{0x01, 0x01, 0x01, 0x01, 0x01, byte(PUSH2), 0x01, 0x01, 0x01, 0x01, 0x01}, 0b0000_0000, 1},
		{[]byte{byte(PUSH3), 0x01, 0x01, 0x01, byte(PUSH1), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b0010_1110, 0},
		{[]byte{byte(PUSH3), 0x01, 0x01, 0x01, byte(PUSH1), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b0000_0000, 1},
		{[]byte{0x01, byte(PUSH8), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b1111_1100, 0},
		{[]byte{0x01, byte(PUSH8), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b0000_0011, 1},
		{[]byte{byte(PUSH16), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b1111_1110, 0},
		{[]byte{byte(PUSH16), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b1111_1111, 1},
		{[]byte{byte(PUSH16), 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, 0b0000_0001, 2},
		{[]byte{byte(PUSH8), 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, byte(PUSH1), 0x01}, 0b1111_1110, 0},
		{[]byte{byte(PUSH8), 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, byte(PUSH1), 0x01}, 0b0000_0101, 1},
		{[]byte{byte(PUSH32)}, 0b1111_1110, 0},
		{[]byte{byte(PUSH32)}, 0b1111_1111, 1},
		{[]byte{byte(PUSH32)}, 0b1111_1111, 2},
		{[]byte{byte(PUSH32)}, 0b1111_1111, 3},
		{[]byte{byte(PUSH32)}, 0b0000_0001, 4},
	}
	for i, test := range tests {
		ret := codeBitmap(test.code)
		if ret[test.which] != test.exp {
			t.Fatalf("test %d: expected %x, got %02x", i, test.exp, ret[test.which])
		}
	}
}

// A create frame (empty CodeHash) must analyze the
// jumpdest bitmap locally and never populate the process-global bitmapCache;
// a regular deployed contract (real CodeHash) still does.
func TestIsCode_EmptyCodeHashSkipsGlobalCache(t *testing.T) {
	bitmapCache.Purge()
	t.Cleanup(func() { bitmapCache.Purge() })

	// PUSH1 3, JUMP, JUMPDEST, PUSH1 0, PUSH1 0, RETURN. Position 3 holds
	// JUMPDEST as real code; positions 1, 5 and 7 are PUSH1 data bytes that
	// must NOT be classified as valid jump destinations.
	code := []byte{byte(PUSH1), 0x03, byte(JUMP), byte(JUMPDEST), byte(PUSH1), 0x00, byte(PUSH1), 0x00, byte(RETURN)}
	want := codeBitmap(code)

	initcode := &Contract{}
	initcode.SetCallCode(&common.Address{}, common.Hash{}, code)

	for _, pos := range []uint64{1, 3, 5, 7} {
		if got, exp := initcode.isCode(pos), want.codeSegment(pos); got != exp {
			t.Fatalf("isCode(%d) = %v, want %v (matching codeBitmap ground truth)", pos, got, exp)
		}
	}
	if !initcode.validJumpdest(uint256.NewInt(3)) {
		t.Fatal("JUMPDEST at position 3 must be a valid jump destination")
	}
	if initcode.validJumpdest(uint256.NewInt(1)) {
		t.Fatal("PUSH1 data byte at position 1 must not be a valid jump destination")
	}

	hash := common.Hash(thor.Keccak256(code))
	if bitmapCache.Contains(hash) {
		t.Fatal("analyzing a create-frame contract (empty CodeHash) must not populate the global bitmap cache")
	}

	// Positive control: the same code with a real codehash (the 'regular
	// contract' branch) does get cached, proving the two branches actually
	// differ and this test isn't vacuously passing.
	deployed := &Contract{}
	deployed.SetCallCode(&common.Address{}, hash, code)
	deployed.isCode(3)
	if !bitmapCache.Contains(hash) {
		t.Fatal("a regular (deployed) contract with a real codehash should populate the global bitmap cache")
	}
}

const analysisCodeSize = 1200 * 1024

func BenchmarkJumpdestAnalysis_1200k(bench *testing.B) {
	// 1.4 ms
	code := make([]byte, analysisCodeSize)
	bench.SetBytes(analysisCodeSize)

	for bench.Loop() {
		codeBitmap(code)
	}
	bench.StopTimer()
}

func BenchmarkJumpdestHashing_1200k(bench *testing.B) {
	// 4 ms
	code := make([]byte, analysisCodeSize)
	bench.SetBytes(analysisCodeSize)

	for bench.Loop() {
		thor.Keccak256(code)
	}
	bench.StopTimer()
}

func BenchmarkJumpdestOpAnalysis(bench *testing.B) {
	var op OpCode
	bencher := func(b *testing.B) {
		code := make([]byte, analysisCodeSize)
		b.SetBytes(analysisCodeSize)
		for i := range code {
			code[i] = byte(op)
		}
		bits := make(bitvec, len(code)/8+1+4)
		b.ResetTimer()
		for b.Loop() {
			for j := range bits {
				bits[j] = 0
			}
			codeBitmapInternal(code, bits)
		}
	}
	for op = PUSH1; op <= PUSH32; op++ {
		bench.Run(op.String(), bencher)
	}
	op = JUMPDEST
	bench.Run(op.String(), bencher)
	op = STOP
	bench.Run(op.String(), bencher)
}
