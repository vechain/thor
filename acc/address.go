package acc

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
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
