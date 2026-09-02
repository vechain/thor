// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"math/big"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extRLPListHeader builds a canonical RLP long-list header: the length must be
// expressed in the minimum number of bytes.
func extRLPListHeader(sz int) []byte {
	if sz < 56 {
		return []byte{byte(0xc0 + sz)}
	}
	var lb []byte
	for s := sz; s > 0; s >>= 8 {
		lb = append([]byte{byte(s)}, lb...)
	}
	return append([]byte{0xf7 + byte(len(lb))}, lb...)
}

// makeExtension encodes a list of n empty strings (0x80 each).
func makeExtension(n int) []byte {
	content := make([]byte, n)
	for i := range content {
		content[i] = 0x80
	}
	return append(extRLPListHeader(len(content)), content...)
}

func allocsForDecodeExtension(data []byte) uint64 {
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	var ex extension
	_ = rlp.DecodeBytes(data, &ex) // expected to fail; we only measure allocations

	runtime.ReadMemStats(&m1)
	runtime.KeepAlive(ex)
	return m1.TotalAlloc - m0.TotalAlloc
}

// The core acceptance criterion: an over-limit input must not allocate in
// proportion to its size. Both inputs below are rejected, but they differ in
// size by a factor of one million.
func TestExtensionDecodeRLP_AllocDecoupledFromInput(t *testing.T) {
	justOver := allocsForDecodeExtension(makeExtension(4))
	wayOver := allocsForDecodeExtension(makeExtension(4_000_000))

	t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)

	// After the fix both stop at the 4th element, so the far larger input must
	// not cost meaningfully more. 64 KiB leaves room for stream buffering.
	assert.Less(t, wayOver, uint64(64*1024),
		"allocation tracks input size; the limit is still being enforced after materialization")
}

// Locks the accept/reject boundary: the set of accepted inputs must be
// identical to the pre-fix implementation.
func TestExtensionDecodeRLP_Boundary(t *testing.T) {
	t.Run("empty list is rejected", func(t *testing.T) {
		var ex extension
		err := rlp.DecodeBytes([]byte{0xc0}, &ex)
		assert.EqualError(t, err, "rlp: unexpected extension")
	})

	t.Run("one element, alpha empty, is rejected as untrimmed", func(t *testing.T) {
		var ex extension
		err := rlp.DecodeBytes([]byte{0xc1, 0x80}, &ex)
		assert.EqualError(t, err, "rlp: extension must be trimmed")
	})

	t.Run("one element, alpha non-empty, is accepted", func(t *testing.T) {
		var ex extension
		// [0xff]
		require.NoError(t, rlp.DecodeBytes([]byte{0xc2, 0x81, 0xff}, &ex))
		assert.Equal(t, []byte{0xff}, ex.Alpha)
		assert.False(t, ex.COM)
		assert.Nil(t, ex.BaseFee)
	})

	t.Run("two elements with com=false is rejected as untrimmed", func(t *testing.T) {
		var ex extension
		// [0xff, false] -- false encodes as 0x80
		err := rlp.DecodeBytes([]byte{0xc3, 0x81, 0xff, 0x80}, &ex)
		assert.EqualError(t, err, "rlp: extension must be trimmed")
	})

	t.Run("two elements with com=true is accepted", func(t *testing.T) {
		var ex extension
		// [0xff, true] -- true encodes as 0x01
		require.NoError(t, rlp.DecodeBytes([]byte{0xc3, 0x81, 0xff, 0x01}, &ex))
		assert.Equal(t, []byte{0xff}, ex.Alpha)
		assert.True(t, ex.COM)
		assert.Nil(t, ex.BaseFee)
	})

	t.Run("three elements is accepted", func(t *testing.T) {
		var ex extension
		// [0xff, true, 0x2a]
		require.NoError(t, rlp.DecodeBytes([]byte{0xc4, 0x81, 0xff, 0x01, 0x2a}, &ex))
		assert.Equal(t, []byte{0xff}, ex.Alpha)
		assert.True(t, ex.COM)
		assert.Equal(t, big.NewInt(0x2a), ex.BaseFee)
	})

	t.Run("four elements is rejected", func(t *testing.T) {
		var ex extension
		err := rlp.DecodeBytes([]byte{0xc5, 0x81, 0xff, 0x01, 0x2a, 0x80}, &ex)
		assert.EqualError(t, err, "rlp: unexpected extension")
	})

	// Behaviour change: the old implementation swallowed this error and fell
	// through to "unexpected extension". It now surfaces the real decode error.
	t.Run("non-list value returns the real decode error", func(t *testing.T) {
		var ex extension
		err := rlp.DecodeBytes([]byte{0x81, 0xff}, &ex)
		require.Error(t, err)
		assert.NotEqual(t, "rlp: unexpected extension", err.Error())
	})
}
