// Copyright 2014 The go-ethereum Authors
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

package trie

import (
	"bytes"
	"testing"
)

func TestHexCompact(t *testing.T) {
	tests := []struct{ hex, compact []byte }{
		// empty keys, with and without terminator.
		{hex: []byte{}, compact: []byte{0x00}},
		{hex: []byte{16}, compact: []byte{0x20}},
		// odd length, no terminator
		{hex: []byte{1, 2, 3, 4, 5}, compact: []byte{0x11, 0x23, 0x45}},
		// even length, no terminator
		{hex: []byte{0, 1, 2, 3, 4, 5}, compact: []byte{0x00, 0x01, 0x23, 0x45}},
		// odd length, terminator
		{hex: []byte{15, 1, 12, 11, 8, 16 /*term*/}, compact: []byte{0x3f, 0x1c, 0xb8}},
		// even length, terminator
		{hex: []byte{0, 15, 1, 12, 11, 8, 16 /*term*/}, compact: []byte{0x20, 0x0f, 0x1c, 0xb8}},
	}
	for _, test := range tests {
		if c := hexToCompact(test.hex); !bytes.Equal(c, test.compact) {
			t.Errorf("hexToCompact(%x) -> %x, want %x", test.hex, c, test.compact)
		}
		if c := appendHexToCompact(nil, test.hex); !bytes.Equal(c, test.compact) {
			t.Errorf("appendHexToCompact(%x) -> %x, want %x", test.hex, c, test.compact)
		}
		if l := compactLen(test.hex); l != len(test.compact) {
			t.Errorf("compactLen(%x) -> %v, want %v", test.hex, l, len(test.compact))
		}
		if h := compactToHex(test.compact); !bytes.Equal(h, test.hex) {
			t.Errorf("compactToHex(%x) -> %x, want %x", test.compact, h, test.hex)
		}
	}
}

func TestHexKeybytes(t *testing.T) {
	tests := []struct{ key, hexIn, hexOut []byte }{
		{key: []byte{}, hexIn: []byte{16}, hexOut: []byte{16}},
		{key: []byte{}, hexIn: []byte{}, hexOut: []byte{16}},
		{
			key:    []byte{0x12, 0x34, 0x56},
			hexIn:  []byte{1, 2, 3, 4, 5, 6, 16},
			hexOut: []byte{1, 2, 3, 4, 5, 6, 16},
		},
		{
			key:    []byte{0x12, 0x34, 0x5},
			hexIn:  []byte{1, 2, 3, 4, 0, 5, 16},
			hexOut: []byte{1, 2, 3, 4, 0, 5, 16},
		},
		{
			key:    []byte{0x12, 0x34, 0x56},
			hexIn:  []byte{1, 2, 3, 4, 5, 6},
			hexOut: []byte{1, 2, 3, 4, 5, 6, 16},
		},
	}
	for _, test := range tests {
		if h := keybytesToHex(test.key); !bytes.Equal(h, test.hexOut) {
			t.Errorf("keybytesToHex(%x) -> %x, want %x", test.key, h, test.hexOut)
		}
		if k := hexToKeybytes(test.hexIn); !bytes.Equal(k, test.key) {
			t.Errorf("hexToKeybytes(%x) -> %x, want %x", test.hexIn, k, test.key)
		}
	}
}

func BenchmarkHexToCompact(b *testing.B) {
	testBytes := []byte{0, 15, 1, 12, 11, 8, 16 /*term*/}
	for b.Loop() {
		hexToCompact(testBytes)
	}
}

func BenchmarkAppendHexToCompact(b *testing.B) {
	testBytes := []byte{0, 15, 1, 12, 11, 8, 16 /*term*/}
	var buf []byte
	for b.Loop() {
		buf = appendHexToCompact(buf[:0], testBytes)
	}
}

func BenchmarkCompactToHex(b *testing.B) {
	testBytes := []byte{0, 15, 1, 12, 11, 8, 16 /*term*/}
	for b.Loop() {
		compactToHex(testBytes)
	}
}

func BenchmarkKeybytesToHex(b *testing.B) {
	testBytes := []byte{7, 6, 6, 5, 7, 2, 6, 2, 16}
	for b.Loop() {
		keybytesToHex(testBytes)
	}
}

func BenchmarkHexToKeybytes(b *testing.B) {
	testBytes := []byte{7, 6, 6, 5, 7, 2, 6, 2, 16}
	for b.Loop() {
		hexToKeybytes(testBytes)
	}
}
