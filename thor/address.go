package thor

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	// AddressLength length of address in bytes.
	AddressLength = common.AddressLength
)

// Address address of account.
type Address common.Address

// String implements the stringer interface
func (a Address) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

// Bytes returns byte slice form of address.
func (a Address) Bytes() []byte {
	return a[:]
}

// ParseAddress convert string presented address into Address type.
func ParseAddress(s string) (*Address, error) {
	if len(s) == AddressLength*2 {
	} else if len(s) == AddressLength*2+2 {
		if strings.ToLower(s[:2]) != "0x" {
			return nil, errors.New("invalid prefix")
		}
		s = s[2:]
	} else {
		return nil, errors.New("invalid length")
	}

	var addr Address
	_, err := hex.Decode(addr[:], []byte(s))
	if err != nil {
		return nil, err
	}
	return &addr, nil
}

// BytesToAddress converts bytes slice into address.
// If b is larger than address legnth, b will be cropped (from the left).
// If b is smaller than address length, b will be extended (from the left).
func BytesToAddress(b []byte) Address {
	return Address(common.BytesToAddress(b))
}

// CreateContractAddress to generate contract address according to tx hash, clause index and
// contract creation count.
func CreateContractAddress(txHash Hash, clauseIndex uint64, creationCount uint64) Address {
	data, _ := rlp.EncodeToBytes([]interface{}{txHash, clauseIndex, creationCount})
	return BytesToAddress(crypto.Keccak256(data)[12:])
}
