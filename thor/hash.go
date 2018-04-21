package thor

import (
	"hash"

	"golang.org/x/crypto/blake2b"
)

// NewBlake2b return blake2b-256 hash.
func NewBlake2b() hash.Hash {
	hash, _ := blake2b.New256(nil)
	return hash
}

// Blake2b computes blake2b-256 checksum for given data.
func Blake2b(data ...[]byte) (b32 Bytes32) {
	hash := NewBlake2b()
	for _, b := range data {
		hash.Write(b)
	}
	hash.Sum(b32[:0])
	return
}
