// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

import (
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// Range describes key range of kv store.
type Range struct {
	From []byte
	To   []byte
}

// NewRange create a range.
func NewRange(from []byte, to []byte) *Range {
	return &Range{
		from,
		to,
	}
}

// NewRangeWithBytesPrefix create a range defined by bytes prefix.
func NewRangeWithBytesPrefix(prefix []byte) *Range {
	r := util.BytesPrefix(prefix)
	return &Range{
		r.Start,
		r.Limit,
	}
}

// NewRangeWithHexPrefix create a range defined by hex prefix.
// The hex can be odd.
func NewRangeWithHexPrefix(hexPrefix string) (*Range, error) {
	if len(hexPrefix)%2 > 0 {
		// odd hex
		start, err := hex.DecodeString(hexPrefix + "0")
		if err != nil {
			return nil, errors.Wrap(err, "new range")
		}
		end, err := hex.DecodeString(hexPrefix + "f")
		if err != nil {
			return nil, errors.Wrap(err, "new range")
		}

		return &Range{
			start,
			util.BytesPrefix(end).Limit,
		}, nil
	}
	// even hex
	prefix, err := hex.DecodeString(hexPrefix)
	if err != nil {
		return nil, errors.Wrap(err, "new range")
	}

	return NewRangeWithBytesPrefix(prefix), nil
}

// WithPrefix create a new range prefixed with prefix.
func (r Range) WithPrefix(prefix []byte) *Range {
	r.From = withPrefix(r.From, prefix)
	r.To = withPrefix(r.To, prefix)
	return &r
}

func withPrefix(src []byte, prefix []byte) []byte {
	r := make([]byte, len(prefix), len(src))
	copy(r, prefix)
	copy(r[len(prefix):], src)
	return r
}
