// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Bytes32 array of 32 bytes.
type Bytes32 [32]byte

var (
	_ json.Marshaler   = (*Bytes32)(nil)
	_ json.Unmarshaler = (*Bytes32)(nil)
)

// String implements stringer
func (b Bytes32) String() string {
	return "0x" + hex.EncodeToString(b[:])
}

// AbbrevString returns abbrev string presentation.
func (b Bytes32) AbbrevString() string {
	return fmt.Sprintf("0x%x…%x", b[:4], b[28:])
}

// Bytes returns byte slice form of Bytes32.
func (b Bytes32) Bytes() []byte {
	return b[:]
}

// IsZero returns if Bytes32 has all zero bytes.
func (b Bytes32) IsZero() bool {
	return b == Bytes32{}
}

// MarshalJSON implements json.Marshaler.
func (b *Bytes32) MarshalJSON() ([]byte, error) {
	if b == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(b.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *Bytes32) UnmarshalJSON(data []byte) error {
	var hex string
	if err := json.Unmarshal(data, &hex); err != nil {
		return err
	}
	parsed, err := ParseBytes32(hex)
	if err != nil {
		return err
	}
	*b = parsed
	return nil
}

// ParseBytes32 convert string presented into Bytes32 type
func ParseBytes32(s string) (Bytes32, error) {
	if len(s) == 32*2 {
	} else if len(s) == 32*2+2 {
		if strings.ToLower(s[:2]) != "0x" {
			return Bytes32{}, errors.New("invalid prefix")
		}
		s = s[2:]
	} else {
		return Bytes32{}, errors.New("invalid length")
	}

	var b Bytes32
	_, err := hex.Decode(b[:], []byte(s))
	if err != nil {
		return Bytes32{}, err
	}
	return b, nil
}

// MustParseBytes32 convert string presented into Bytes32 type, panic on error.
func MustParseBytes32(s string) Bytes32 {
	b32, err := ParseBytes32(s)
	if err != nil {
		panic(err)
	}
	return b32
}

// BytesToBytes32 converts bytes slice into Bytes32.
// If b is larger than Bytes32 legnth, b will be cropped (from the left).
// If b is smaller than Bytes32 length, b will be extended (from the left).
func BytesToBytes32(b []byte) Bytes32 {
	return Bytes32(common.BytesToHash(b))
}
