// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// encodePath encodes the path into compact form.
func encodePath(dst []byte, path []byte) []byte {
	d := len(path)
	s := d / 4
	if s > math.MaxUint8 {
		panic(fmt.Errorf("unexpected length of path: %v", d))
	}
	// the prefix s is to split the trie into sub tries with depth 4.
	dst = append(dst, byte(s))

	// further on, a sub trie is divided to depth-2 sub tries.
	for i := 0; ; i += 4 {
		switch d - i {
		case 0:
			return append(dst, 0)
		case 1:
			return append(dst, (path[i]<<3)|1)
		case 2:
			t := (uint16(path[i]) << 4) | uint16(path[i+1])
			return appendUint16(dst, 0x8000|(t<<7))
		case 3:
			t := (uint16(path[i]) << 8) | (uint16(path[i+1]) << 4) | uint16(path[i+2])
			return appendUint16(dst, 0x8000|(t<<3)|1)
		default:
			dst = append(dst, (path[i]<<4)|path[i+1], (path[i+2]<<4)|path[i+3])
		}
	}
}

func appendUint32(b []byte, v uint32) []byte {
	return append(b,
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v),
	)
}

func appendUint16(b []byte, v uint16) []byte {
	return append(b,
		byte(v>>8),
		byte(v),
	)
}

// newContextChecker creates a debounced context checker.
func newContextChecker(ctx context.Context, debounce int) func() error {
	count := 0
	return func() error {
		count++
		if count > debounce {
			count = 0
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		return nil
	}
}

type buffer struct {
	buf []byte
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return &buffer{}
	},
}
