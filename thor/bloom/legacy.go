// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bloom

import (
	"math"

	"github.com/vechain/thor/thor"
)

const (
	legacyMaxK       = 16
	legacyBitsLength = 2048
)

// LegacyEstimateBloomK estimate k(num of hash funcs) according to item count.
func LegacyEstimateBloomK(itemCount int) int {
	k := int(math.Round(float64(legacyBitsLength) / float64(itemCount) * math.Ln2))
	if k > legacyMaxK {
		return legacyMaxK
	}
	if k < 1 {
		return 1
	}
	return k
}

// LegacyBloom a simple bloom filter in 2048 bit length.
type LegacyBloom struct {
	Bits [legacyBitsLength / 8]byte
	K    int
}

// NewLegacyBloom new bloom.
func NewLegacyBloom(k int) *LegacyBloom {
	if k > legacyMaxK || k < 1 {
		panic("bloom: invalid K")
	}
	return &LegacyBloom{K: k}
}

// Add add item into bloom.
func (b *LegacyBloom) Add(item []byte) {
	b.distribute(item, func(index int, bit byte) bool {
		b.Bits[index] |= bit
		return true
	})
}

// Test test if item contained. (false positive)
func (b *LegacyBloom) Test(item []byte) bool {
	return b.distribute(item, func(index int, bit byte) bool {
		return b.Bits[index]&bit == bit
	})
}

func (b *LegacyBloom) distribute(item []byte, cb func(index int, bit byte) bool) bool {
	hash := thor.Blake2b(item)
	for i := 0; i < b.K; i++ {
		d := (uint(hash[i*2+1]) + (uint(hash[i*2]) << 8)) % legacyBitsLength
		bit := byte(1) << (d % 8)
		if !cb(int(d/8), bit) {
			return false
		}
	}
	return true
}
