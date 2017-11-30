package cry

import (
<<<<<<< HEAD
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
=======
	"math/big"

>>>>>>> add crypto for 'address','signature'
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/sha3"
)

func Keccak256(data ...[]byte) []byte {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}

	return d.Sum(nil)
}

func Keccak256Hash(data ...[]byte) (h common.Hash) {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	d.Sum(h[:0])
	return h
}

// Deprecated: For backward compatibility as other packages depend on these
func VSha3(data ...[]byte) []byte          { return Keccak256(data...) }
func VSha3Hash(data ...[]byte) common.Hash { return Keccak256Hash(data...) }

func BytesToHash(b []byte) common.Hash {
	var h common.Hash
	h.SetBytes(b)
	return h
}

func StringToHash(s string) common.Hash { return BytesToHash([]byte(s)) }
func BigToHash(b *big.Int) common.Hash  { return BytesToHash(b.Bytes()) }
func HexToHash(s string) common.Hash    { return BytesToHash(common.FromHex(s)) }
