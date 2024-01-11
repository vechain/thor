// Copyright 2016 The go-ethereum Authors
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
	"crypto/rand"
	"testing"
)

func randBytes(n int) []byte {
	r := make([]byte, n)
	rand.Read(r)
	return r
}

func benchmarkEncodeFullNode(b *testing.B, consensus, skipHash bool) {
	var (
		f   = fullNode{}
		buf []byte
	)
	for i := 0; i < 16; i++ {
		f.children[i] = &refNode{hash: randBytes(32)}
	}
	for i := 0; i < b.N; i++ {
		if consensus {
			buf = f.encodeConsensus(buf[:0])
		} else {
			buf = f.encode(buf[:0], skipHash)
		}
	}
}
func benchmarkEncodeShortNode(b *testing.B, consensus bool) {
	var (
		s = shortNode{
			key:   []byte{0x1, 0x2, 0x10},
			child: &valueNode{val: randBytes(32)},
		}
		buf []byte
	)

	for i := 0; i < b.N; i++ {
		if consensus {
			buf = s.encodeConsensus(buf[:0])
		} else {
			buf = s.encode(buf[:0], false)
		}
	}
}

func BenchmarkEncodeFullNode(b *testing.B) {
	benchmarkEncodeFullNode(b, false, false)
}

func BenchmarkEncodeFullNodeSkipHash(b *testing.B) {
	benchmarkEncodeFullNode(b, false, true)
}

func BenchmarkEncodeFullNodeConsensus(b *testing.B) {
	benchmarkEncodeFullNode(b, true, false)
}

func BenchmarkEncodeShortNode(b *testing.B) {
	benchmarkEncodeShortNode(b, false)
}

func BenchmarkEncodeShortNodeConsensus(b *testing.B) {
	benchmarkEncodeShortNode(b, true)
}

func benchmarkDecodeFullNode(b *testing.B, skipHash bool) {
	f := fullNode{}
	for i := 0; i < 16; i++ {
		f.children[i] = &refNode{hash: randBytes(32)}
	}
	enc := f.encode(nil, skipHash)
	for i := 0; i < b.N; i++ {
		mustDecodeNode(nil, enc, 0)
	}
}

func BenchmarkDecodeFullNode(b *testing.B) {
	benchmarkDecodeFullNode(b, false)
}

func BenchmarkDecodeFullNodeSkipHash(b *testing.B) {
	benchmarkDecodeFullNode(b, true)
}

func BenchmarkDecodeShortNode(b *testing.B) {
	s := shortNode{
		key:   []byte{0x1, 0x2, 0x10},
		child: &valueNode{val: randBytes(32)},
	}

	enc := s.encode(nil, false)
	for i := 0; i < b.N; i++ {
		mustDecodeNode(nil, enc, 0)
	}
}
