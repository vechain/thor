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
	// the prefix s is to split the trie into sub tries.
	dst = append(dst, byte(s))

	// the last nibble of the encoded path is d % 4
	for i := 0; i <= d; i += 4 {
		switch d - i {
		case 0:
			dst = append(dst, 0, 0)
		case 1:
			dst = append(dst, path[i]<<4, 1)
		case 2:
			dst = append(dst, (path[i]<<4)|path[i+1], 2)
		case 3:
			dst = append(dst, (path[i]<<4)|path[i+1], (path[i+2]<<4)|3)
		default:
			dst = append(dst, (path[i]<<4)|path[i+1], (path[i+2]<<4)|path[i+3])
		}
	}
	return dst
}

func appendUint32(b []byte, v uint32) []byte {
	return append(b,
		byte(v>>24),
		byte(v>>16),
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
