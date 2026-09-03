// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"errors"
	"math"
)

type vpScope struct{}

// vp implements varint-prefix coding.
//
// It's much simpler and a bit faster than RLP.
// Trie nodes stored in database are encoded using vp.
var vp vpScope

// AppendUint32 appends vp-encoded i to buf and returns the extended buffer.
func (vpScope) AppendUint32(buf []byte, i uint32) []byte {
	return binary.AppendUvarint(buf, uint64(i))
}

// AppendString appends vp-encoded str to buf and returns the extended buffer.
func (vpScope) AppendString(buf, str []byte) []byte {
	buf = binary.AppendUvarint(buf, uint64(len(str)))
	return append(buf, str...)
}

// SplitString extracts a string and returns rest bytes.
// It'll panic if errored.
func (vpScope) SplitString(buf []byte) (str []byte, rest []byte, err error) {
	i, n := binary.Uvarint(buf)
	if n <= 0 {
		return nil, nil, errors.New("invalid uvarint prefix")
	}
	buf = buf[n:]
	return buf[:i], buf[i:], nil
}

// SplitUint32 extracts uint32 and returns rest bytes.
// It'll panic if errored.
func (vpScope) SplitUint32(buf []byte) (i uint32, rest []byte, err error) {
	i64, n := binary.Uvarint(buf)
	if n <= 0 || i64 > math.MaxUint32 {
		return 0, nil, errors.New("invalid uvarint prefix")
	}
	return uint32(i64), buf[n:], nil
}
