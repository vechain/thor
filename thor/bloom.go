package thor

import (
	"math"
)

const (
	maxK       = 16
	bitsLength = 2048
)

// EstimateBloomK estimate k(num of hash funcs) according to item count.
func EstimateBloomK(itemCount int) int {
	k := int(math.Round(float64(bitsLength) / float64(itemCount) * math.Ln2))
	if k > maxK {
		return maxK
	}
	if k < 0 {
		return 1
	}
	return k
}

// Bloom a simple bloom filter in 2048 bit length.
type Bloom struct {
	Bits [bitsLength / 8]byte
	K    int
}

// NewBloom new bloom.
func NewBloom(k int) *Bloom {
	if k > maxK || k < 1 {
		panic("bloom: invalid K")
	}
	return &Bloom{K: k}
}

// Add add item into bloom.
func (b *Bloom) Add(item []byte) {
	b.distribute(item, func(index int, bit byte) bool {
		b.Bits[index] |= bit
		return true
	})
}

// Test test if item contained. (false positive)
func (b *Bloom) Test(item []byte) bool {
	return b.distribute(item, func(index int, bit byte) bool {
		return b.Bits[index]&bit == bit
	})
}

func (b *Bloom) distribute(item []byte, cb func(index int, bit byte) bool) bool {
	hash := Blake2b(item)
	for i := 0; i < b.K; i++ {
		d := (uint(hash[i*2+1]) + (uint(hash[i*2]) << 8)) % bitsLength
		bit := byte(1) << (d % 8)
		if !cb(int(d/8), bit) {
			return false
		}
	}
	return true
}
