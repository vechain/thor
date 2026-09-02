// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package proto

import (
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockByIDResult_DecodeRLP(t *testing.T) {
	t.Run("rejects more than MaxBlockByIDResult", func(t *testing.T) {
		data, err := rlp.EncodeToBytes([]rlp.RawValue{
			[]byte{0x01},
			[]byte{0x02},
		})
		require.NoError(t, err)

		var result blockByIDResult
		err = rlp.DecodeBytes(data, &result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("accepts empty result", func(t *testing.T) {
		data, err := rlp.EncodeToBytes([]rlp.RawValue{})
		require.NoError(t, err)

		var result blockByIDResult
		require.NoError(t, rlp.DecodeBytes(data, &result))
		assert.Empty(t, result)
	})

	t.Run("accepts single block", func(t *testing.T) {
		data, err := rlp.EncodeToBytes([]rlp.RawValue{[]byte{0x01}})
		require.NoError(t, err)

		var result blockByIDResult
		require.NoError(t, rlp.DecodeBytes(data, &result))
		assert.Len(t, result, 1)
	})

	// The point of the fix: rejecting a huge list must not cost in proportion
	// to its size.
	t.Run("allocation is decoupled from input size", func(t *testing.T) {
		measure := func(n int) uint64 {
			oversized := make([]rlp.RawValue, n)
			for i := range oversized {
				oversized[i] = []byte{0x80}
			}
			data, err := rlp.EncodeToBytes(oversized)
			require.NoError(t, err)

			var m0, m1 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m0)

			var result blockByIDResult
			_ = rlp.DecodeBytes(data, &result)

			runtime.ReadMemStats(&m1)
			runtime.KeepAlive(result)
			return m1.TotalAlloc - m0.TotalAlloc
		}

		justOver := measure(2)
		wayOver := measure(2_000_000)
		t.Logf("just-over=%d B  way-over=%d B", justOver, wayOver)
		assert.Less(t, wayOver, uint64(64*1024))
	})
}

func TestBlocksFromNumberResult_DecodeRLP(t *testing.T) {
	t.Run("rejects more than MaxBlocksFromNumber", func(t *testing.T) {
		oversized := make([]rlp.RawValue, MaxBlocksFromNumber+1)
		for i := range oversized {
			// byte(i)&0x7f keeps every element within the single-byte
			// self-encoded RLP range so it's a valid raw value on its own.
			oversized[i] = []byte{byte(i) & 0x7f}
		}
		data, err := rlp.EncodeToBytes(oversized)
		require.NoError(t, err)

		var result blocksFromNumberResult
		err = rlp.DecodeBytes(data, &result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("accepts empty result", func(t *testing.T) {
		data, err := rlp.EncodeToBytes([]rlp.RawValue{})
		require.NoError(t, err)

		var result blocksFromNumberResult
		require.NoError(t, rlp.DecodeBytes(data, &result))
		assert.Empty(t, result)
	})

	t.Run("accepts exactly MaxBlocksFromNumber", func(t *testing.T) {
		blocks := make([]rlp.RawValue, MaxBlocksFromNumber)
		for i := range blocks {
			blocks[i] = []byte{byte(i) & 0x7f}
		}
		data, err := rlp.EncodeToBytes(blocks)
		require.NoError(t, err)

		var result blocksFromNumberResult
		require.NoError(t, rlp.DecodeBytes(data, &result))
		assert.Len(t, result, MaxBlocksFromNumber)
	})
}
