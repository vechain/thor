package cry

import (
	"hash"

	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/vechain/thor/thor"
)

// NewHasher returns widely used hasher (Keccak256).
func NewHasher() hash.Hash {
	return sha3.NewKeccak256()
}

// HashSum computes hash of data using hasher returned by NewHash.
func HashSum(data ...[]byte) (hash thor.Hash) {
	h := NewHasher()
	for _, b := range data {
		h.Write(b)
	}
	h.Sum(hash[:0])
	return
}
