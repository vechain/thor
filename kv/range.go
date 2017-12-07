package kv

import (
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// Range describes key range of kv store.
type Range struct {
	r *util.Range
}

// NewRange create a range.
func NewRange(from []byte, to []byte) *Range {
	return &Range{
		r: &util.Range{
			Start: from,
			Limit: to,
		},
	}
}

// NewRangeWithBytesPrefix create a range defined by bytes prefix.
func NewRangeWithBytesPrefix(prefix []byte) *Range {
	return &Range{
		r: util.BytesPrefix(prefix),
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
			r: &util.Range{
				Start: start,
				Limit: util.BytesPrefix(end).Limit,
			},
		}, nil
	}
	// even hex
	prefix, err := hex.DecodeString(hexPrefix)
	if err != nil {
		return nil, errors.Wrap(err, "new range")
	}

	return NewRangeWithBytesPrefix(prefix), nil
}

func (r Range) WithPrefix(prefix []byte) *Range {
	r.r.Start = withPrefix(r.r.Start, prefix)
	r.r.Limit = withPrefix(r.r.Limit, prefix)
	return &r
}

func withPrefix(src []byte, prefix []byte) []byte {
	r := make([]byte, len(prefix), len(src))
	copy(r, prefix)
	copy(r[len(prefix):], src)
	return r
}
