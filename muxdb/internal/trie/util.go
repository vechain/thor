// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"context"
	"sync"
)

// encodePath encodes the path into 4-bytes aligned compact format.
// The encoded paths are in lexicographical order even with suffix appended.
func encodePath(dst []byte, path []byte) []byte {
	if len(path) == 0 {
		return append(dst, 0, 0, 0, 0)
	}
	for i := 0; i < len(path); i += 7 {
		dst = appendUint32(dst, encodePath32(path[i:]))
	}
	return dst
}

// encodePath32 encodes at most 7 path elements into uint32.
func encodePath32(path []byte) uint32 {
	n := len(path)
	if n > 7 {
		n = 8 // means have subsequent path elements.
	}

	var v uint32
	for i := 0; i < 7; i++ {
		if i < n {
			v |= uint32(path[i])
		}
		v <<= 4
	}
	return v | uint32(n)
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
