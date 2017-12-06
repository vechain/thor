package cry

import (
	"encoding/hex"
	"errors"
	"hash"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/sha3"
)

const (
	// HashLength length of hash in bytes
	HashLength = common.HashLength
)

// Hash main hash type
type Hash common.Hash

// String implements stringer
func (h Hash) String() string {
	return "0x" + hex.EncodeToString(h[:])
}

// ParseHash convert string presented hash into Hash type
func ParseHash(s string) (*Hash, error) {
	if len(s) == HashLength*2 {
	} else if len(s) == HashLength*2+2 {
		if strings.ToLower(s[:2]) != "0x" {
			return nil, errors.New("invalid prefix")
		}
		s = s[2:]
	} else {
		return nil, errors.New("invalid length")
	}

	var h Hash
	_, err := hex.Decode(h[:], []byte(s))
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// BytesToHash converts bytes slice into hash.
// If b is larger than hash legnth, b will be cropped (from the left).
// If b is smaller than hash length, b will be extended (from the left).
func BytesToHash(b []byte) Hash {
	return Hash(common.BytesToHash(b))
}

// NewHasher returns widely used hasher (Keccak256).
func NewHasher() hash.Hash {
	return sha3.NewKeccak256()
}

// HashSum computes hash of data using hasher returned by NewHash.
func HashSum(data ...[]byte) Hash {
	h := NewHasher()
	for _, b := range data {
		h.Write(b)
	}
	var hash Hash
	h.Sum(hash[:0])
	return hash
}
