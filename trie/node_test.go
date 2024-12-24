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
	"io"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/test/datagen"
)

func benchmarkEncodeFullNode(b *testing.B, consensus, skipHash bool) {
	var (
		f   = fullNode{}
		buf []byte
	)
	for i := 0; i < 16; i++ {
		f.children[i] = &refNode{hash: datagen.RandomHash().Bytes()}
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
			child: &valueNode{val: datagen.RandBytes(32)},
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
		f.children[i] = &refNode{hash: datagen.RandomHash().Bytes()}
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
		child: &valueNode{val: datagen.RandBytes(32)},
	}

	enc := s.encode(nil, false)
	for i := 0; i < b.N; i++ {
		mustDecodeNode(nil, enc, 0)
	}
}

type fNode struct {
	Children [17]interface{}
}

func (f *fNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, f.Children)
}

type sNode struct {
	Key []byte
	Val interface{}
}
type vNode []byte
type hNode []byte

func TestRefNodeEncodeConsensus(t *testing.T) {
	for i := 0; i < 10; i++ {
		randHash := datagen.RandomHash()

		h := hNode(randHash.Bytes())
		ref := &refNode{hash: randHash.Bytes()}

		expected, err := rlp.EncodeToBytes(h)
		assert.Nil(t, err)
		actual := ref.encodeConsensus(nil)

		assert.Equal(t, expected, actual)
	}
}

func TestValueNodeEncodeConsensus(t *testing.T) {
	for i := 0; i < 10; i++ {
		randValue := datagen.RandBytes(datagen.RandIntN(30))

		v := vNode(randValue)
		value := &valueNode{val: randValue}

		expected, err := rlp.EncodeToBytes(v)
		assert.Nil(t, err)
		actual := value.encodeConsensus(nil)

		assert.Equal(t, expected, actual)
	}
}

func TestShortNodeEncodeConsensus(t *testing.T) {
	for i := 0; i < 10; i++ {
		randKey := datagen.RandBytes(datagen.RandIntN(32))
		randValue := datagen.RandBytes(datagen.RandIntN(30))

		randKey = append(randKey, 16)
		s := &sNode{Key: hexToCompact(randKey), Val: vNode(randValue)}
		short := &shortNode{key: randKey, child: &valueNode{val: randValue}}

		expected, err := rlp.EncodeToBytes(s)
		assert.Nil(t, err)
		actual := short.encodeConsensus(nil)

		assert.Equal(t, expected, actual)
	}

	for i := 0; i < 10; i++ {
		randKey := datagen.RandBytes(datagen.RandIntN(32))
		randHash := datagen.RandomHash()

		s := &sNode{Key: hexToCompact(randKey), Val: hNode(randHash.Bytes())}
		short := &shortNode{key: randKey, child: &refNode{hash: randHash.Bytes()}}

		expected, err := rlp.EncodeToBytes(s)
		assert.Nil(t, err)
		actual := short.encodeConsensus(nil)

		assert.Equal(t, expected, actual)
	}
}

func TestFullNodeEncodeConsensus(t *testing.T) {
	for i := 0; i < 10; i++ {
		randValue := datagen.RandBytes(datagen.RandIntN(30))

		var (
			f    fNode
			full fullNode
		)

		for i := 0; i < 16; i++ {
			if datagen.RandIntN(2) == 1 {
				randHash := datagen.RandomHash()

				f.Children[i] = hNode(randHash.Bytes())
				full.children[i] = &refNode{hash: randHash.Bytes()}
			} else {
				f.Children[i] = vNode(nil)
			}
		}
		f.Children[16] = vNode(randValue)
		full.children[16] = &valueNode{val: randValue}

		expected, err := rlp.EncodeToBytes(&f)
		assert.Nil(t, err)
		actual := full.encodeConsensus(nil)

		assert.Equal(t, expected, actual)
	}

	for i := 0; i < 10; i++ {
		var (
			f    fNode
			full fullNode
		)

		for i := 0; i < 16; i++ {
			if datagen.RandIntN(2) == 1 {
				randHash := datagen.RandomHash()

				f.Children[i] = hNode(randHash.Bytes())
				full.children[i] = &refNode{hash: randHash.Bytes()}
			} else {
				f.Children[i] = vNode(nil)
			}
		}
		f.Children[16] = vNode(nil)

		expected, err := rlp.EncodeToBytes(&f)
		assert.Nil(t, err)
		actual := full.encodeConsensus(nil)

		assert.Equal(t, expected, actual)
	}
}
