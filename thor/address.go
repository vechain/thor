// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	// AddressLength length of address in bytes.
	AddressLength = common.AddressLength
)

// Address address of account.
type Address common.Address

var (
	_ json.Marshaler   = (*Address)(nil)
	_ json.Unmarshaler = (*Address)(nil)
)

// String implements the stringer interface
func (a Address) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

// Bytes returns byte slice form of address.
func (a Address) Bytes() []byte {
	return a[:]
}

// IsZero returns if address is all zero bytes.
func (a Address) IsZero() bool {
	return a == Address{}
}

// MarshalJSON implements json.Marshaler.
func (a *Address) MarshalJSON() ([]byte, error) {
	if a == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(a.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *Address) UnmarshalJSON(data []byte) error {
	var hex string
	if err := json.Unmarshal(data, &hex); err != nil {
		return err
	}
	parsed, err := ParseAddress(hex)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// ParseAddress convert string presented address into Address type.
func ParseAddress(s string) (Address, error) {
	if len(s) == AddressLength*2 {
	} else if len(s) == AddressLength*2+2 {
		if strings.ToLower(s[:2]) != "0x" {
			return Address{}, errors.New("invalid prefix")
		}
		s = s[2:]
	} else {
		return Address{}, errors.New("invalid length")
	}

	var addr Address
	_, err := hex.Decode(addr[:], []byte(s))
	if err != nil {
		return Address{}, err
	}
	return addr, nil
}

// MustParseAddress convert string presented address into Address type, panic on error.
func MustParseAddress(s string) Address {
	addr, err := ParseAddress(s)
	if err != nil {
		panic(err)
	}
	return addr
}

// BytesToAddress converts bytes slice into address.
// If b is larger than address legnth, b will be cropped (from the left).
// If b is smaller than address length, b will be extended (from the left).
func BytesToAddress(b []byte) Address {
	return Address(common.BytesToAddress(b))
}

// CreateContractAddress to generate contract address according to tx id, clause index and
// contract creation count.
func CreateContractAddress(txID Bytes32, clauseIndex uint32, creationCount uint32) Address {
	var b4_1, b4_2 [4]byte
	binary.BigEndian.PutUint32(b4_1[:], clauseIndex)
	binary.BigEndian.PutUint32(b4_2[:], creationCount)
	return BytesToAddress(crypto.Keccak256(txID[:], b4_1[:], b4_2[:]))
}
