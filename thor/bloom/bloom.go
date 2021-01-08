// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// inspired by goleveldb's bloom filter

package bloom

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
)

func distribute(hash uint32, k uint8, nBits uint32, cb func(index int, bit byte) bool) bool {
	delta := (hash >> 17) | (hash << 15) // Rotate right 17 bits
	for i := uint8(0); i < k; i++ {
		bitPos := hash % nBits
		if !cb(int(bitPos/8), 1<<(bitPos%8)) {
			return false
		}
		hash += delta
	}
	return true
}

func hash(key []byte) uint32 {
	return binary.BigEndian.Uint32(thor.Blake2b(key).Bytes())
}

// Filter the bloom filter.
type Filter struct {
	Bits []byte
	K    uint8
}

// Contains to test if the given key is contained (false positive).
func (f *Filter) Contains(key []byte) bool {
	return distribute(hash(key), f.K, uint32(len(f.Bits)*8), func(index int, bit byte) bool {
		return f.Bits[index]&bit == bit
	})
}

// Generator to generate variable-length bloom filter.
type Generator struct {
	hashes map[uint32]bool
}

// Add add the key into bloom.
func (g *Generator) Add(key []byte) {
	if g.hashes == nil {
		g.hashes = make(map[uint32]bool)
	}
	g.hashes[hash(key)] = true
}

// Generate generate variable-length bloom filter according to bits per key and count of key.
// The generator will be reset right after Generate.
func (g *Generator) Generate(bitsPerKey int, k uint8) *Filter {
	// compute bloom filter size in bytes
	nBytes := (len(g.hashes)*bitsPerKey + 7) / 8

	// for small n, we can see a very high false positive rate.  Fix it
	// by enforcing a minimum bloom filter length.
	if nBytes < 8 {
		nBytes = 8
	}

	bits := make([]byte, nBytes)
	// filter bit length
	nBits := uint32(nBytes * 8)

	for hash := range g.hashes {
		distribute(hash, k, nBits, func(index int, bit byte) bool {
			bits[index] |= bit
			return true
		})
	}
	g.hashes = nil
	return &Filter{bits, k}
}

// K calculate the best K value.
func K(bitsPerKey int) uint8 {
	// Round down to reduce probing cost a little bit.
	k := uint8(bitsPerKey * 69 / 100) // bitsPerKey * ln(2),  0.69 =~ ln(2)
	if k < 1 {
		k = 1
	} else if k > 30 {
		k = 30
	}
	return k
}
