package thor

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
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

// Bytes returns byte slice form of hash.
func (h Hash) Bytes() []byte {
	return h[:]
}

// IsZero returns if hash is all zero bytes.
func (h Hash) IsZero() bool {
	return h == Hash{}
}

// ParseHash convert string presented hash into Hash type
func ParseHash(s string) (Hash, error) {
	if len(s) == HashLength*2 {
	} else if len(s) == HashLength*2+2 {
		if strings.ToLower(s[:2]) != "0x" {
			return Hash{}, errors.New("invalid prefix")
		}
		s = s[2:]
	} else {
		return Hash{}, errors.New("invalid length")
	}

	var h Hash
	_, err := hex.Decode(h[:], []byte(s))
	if err != nil {
		return Hash{}, err
	}
	return h, nil
}

// BytesToHash converts bytes slice into hash.
// If b is larger than hash legnth, b will be cropped (from the left).
// If b is smaller than hash length, b will be extended (from the left).
func BytesToHash(b []byte) Hash {
	return Hash(common.BytesToHash(b))
}
