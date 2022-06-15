// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package lowrlp provides methods to perform low-level rlp encoding.
// Codes are mostly copied from github.com/ethereum/go-ethereum/rlp.
package lowrlp

import (
	"io"
)

// Encoder is the low-level rlp encoder.
type Encoder struct {
	str     []byte     // string data, contains everything except list headers
	lheads  []listhead // all list headers
	lhsize  int        // sum of sizes of all encoded list headers
	sizebuf [9]byte    // auxiliary buffer for uint encoding
}

// Reset reset the encoder state.
func (w *Encoder) Reset() {
	w.lhsize = 0
	w.str = w.str[:0]
	w.lheads = w.lheads[:0]
}

// EncodeString encodes the string value.
func (w *Encoder) EncodeString(b []byte) {
	if len(b) == 1 && b[0] <= 0x7F {
		// fits single byte, no string header
		w.str = append(w.str, b[0])
	} else {
		w.encodeStringHeader(len(b))
		w.str = append(w.str, b...)
	}
}

// EncodeUint encodes the uint value.
func (w *Encoder) EncodeUint(i uint64) {
	if i == 0 {
		w.str = append(w.str, 0x80)
	} else if i < 128 {
		// fits single byte
		w.str = append(w.str, byte(i))
	} else {
		s := putint(w.sizebuf[1:], i)
		w.sizebuf[0] = 0x80 + byte(s)
		w.str = append(w.str, w.sizebuf[:s+1]...)
	}
}

// EncodeRaw encodes raw value.
func (w *Encoder) EncodeRaw(r []byte) {
	w.str = append(w.str, r...)
}

// EncodeEmptyString encodes an empty string.
// It's equivalent to w.EncodeString(nil), but more efficient.
func (w *Encoder) EncodeEmptyString() {
	w.str = append(w.str, 0x80)
}

// EncodeEmptyList encodes an empty list.
// It's equivalent to w.ListEnd(w.List()), but more efficient.
func (w *Encoder) EncodeEmptyList() {
	w.str = append(w.str, 0xC0)
}

// List starts to encode list elements.
// It returns the offset which is passed to ListEnd when list ended.
func (w *Encoder) List() int {
	w.lheads = append(w.lheads, listhead{offset: len(w.str), size: w.lhsize})
	return len(w.lheads) - 1
}

// ListEnd ends the list. offset is the return value of the corresponded List call.
func (w *Encoder) ListEnd(index int) {
	lh := &w.lheads[index]
	lh.size = w.size() - lh.offset - lh.size
	if lh.size < 56 {
		w.lhsize++ // length encoded into kind tag
	} else {
		w.lhsize += 1 + intsize(uint64(lh.size))
	}
}

// ToBytes outputs the encode result to byte slice.
func (w *Encoder) ToBytes() []byte {
	out := make([]byte, w.size())
	strpos := 0
	pos := 0
	for _, head := range w.lheads {
		// write string data before header
		n := copy(out[pos:], w.str[strpos:head.offset])
		pos += n
		strpos += n
		// write the header
		enc := head.encode(out[pos:])
		pos += len(enc)
	}
	// copy string data after the last list header
	copy(out[pos:], w.str[strpos:])
	return out
}

// ToWriter outputs the encode result to io.Writer.
func (w *Encoder) ToWriter(out io.Writer) (err error) {
	strpos := 0
	for _, head := range w.lheads {
		// write string data before header
		if head.offset-strpos > 0 {
			n, err := out.Write(w.str[strpos:head.offset])
			strpos += n
			if err != nil {
				return err
			}
		}
		// write the header
		enc := head.encode(w.sizebuf[:])
		if _, err = out.Write(enc); err != nil {
			return err
		}
	}
	if strpos < len(w.str) {
		// write string data after the last list header
		_, err = out.Write(w.str[strpos:])
	}
	return err
}

func (w *Encoder) encodeStringHeader(size int) {
	if size < 56 {
		w.str = append(w.str, 0x80+byte(size))
	} else {
		sizesize := putint(w.sizebuf[1:], uint64(size))
		w.sizebuf[0] = 0xB7 + byte(sizesize)
		w.str = append(w.str, w.sizebuf[:sizesize+1]...)
	}
}

func (w *Encoder) size() int {
	return len(w.str) + w.lhsize
}

type listhead struct {
	offset int // index of this header in string data
	size   int // total size of encoded data (including list headers)
}

// encode writes head to the given buffer, which must be at least
// 9 bytes long. It returns the encoded bytes.
func (head *listhead) encode(buf []byte) []byte {
	return buf[:puthead(buf, 0xC0, 0xF7, uint64(head.size))]
}

// intsize computes the minimum number of bytes required to store i.
func intsize(i uint64) (size int) {
	for size = 1; ; size++ {
		if i >>= 8; i == 0 {
			return size
		}
	}
}

// puthead writes a list or string header to buf.
// buf must be at least 9 bytes long.
func puthead(buf []byte, smalltag, largetag byte, size uint64) int {
	if size < 56 {
		buf[0] = smalltag + byte(size)
		return 1
	}
	sizesize := putint(buf[1:], size)
	buf[0] = largetag + byte(sizesize)
	return sizesize + 1
}

// putint writes i to the beginning of b in big endian byte
// order, using the least number of bytes needed to represent i.
func putint(b []byte, i uint64) (size int) {
	switch {
	case i < (1 << 8):
		b[0] = byte(i)
		return 1
	case i < (1 << 16):
		b[0] = byte(i >> 8)
		b[1] = byte(i)
		return 2
	case i < (1 << 24):
		b[0] = byte(i >> 16)
		b[1] = byte(i >> 8)
		b[2] = byte(i)
		return 3
	case i < (1 << 32):
		b[0] = byte(i >> 24)
		b[1] = byte(i >> 16)
		b[2] = byte(i >> 8)
		b[3] = byte(i)
		return 4
	case i < (1 << 40):
		b[0] = byte(i >> 32)
		b[1] = byte(i >> 24)
		b[2] = byte(i >> 16)
		b[3] = byte(i >> 8)
		b[4] = byte(i)
		return 5
	case i < (1 << 48):
		b[0] = byte(i >> 40)
		b[1] = byte(i >> 32)
		b[2] = byte(i >> 24)
		b[3] = byte(i >> 16)
		b[4] = byte(i >> 8)
		b[5] = byte(i)
		return 6
	case i < (1 << 56):
		b[0] = byte(i >> 48)
		b[1] = byte(i >> 40)
		b[2] = byte(i >> 32)
		b[3] = byte(i >> 24)
		b[4] = byte(i >> 16)
		b[5] = byte(i >> 8)
		b[6] = byte(i)
		return 7
	default:
		b[0] = byte(i >> 56)
		b[1] = byte(i >> 48)
		b[2] = byte(i >> 40)
		b[3] = byte(i >> 32)
		b[4] = byte(i >> 24)
		b[5] = byte(i >> 16)
		b[6] = byte(i >> 8)
		b[7] = byte(i)
		return 8
	}
}
