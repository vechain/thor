// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package muxdb

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/trie"
)

type mockStoreBackend struct {
	kv.Store
	deleteRangeCalled bool
	startKey          []byte
	limitKey          []byte
}

func (m *mockStoreBackend) DeleteRange(ctx context.Context, r kv.Range) error {
	m.deleteRangeCalled = true
	m.startKey = r.Start
	m.limitKey = r.Limit
	return nil
}

func TestAppendHistNodeKey(t *testing.T) {
	b := backend{
		HistPtnFactor:    1000,
		DedupedPtnFactor: 1000,
		CachedNodeTTL:    100,
	}

	tests := []struct {
		name     string
		trieName string
		path     []byte
		ver      trie.Version
	}{
		{
			"empty path",
			"test",
			[]byte{},
			trie.Version{Major: 1, Minor: 0},
		},
		{
			"with path",
			"test",
			[]byte{1, 2, 3},
			trie.Version{Major: 1000, Minor: 1},
		},
		{
			"max uint32 factor",
			"test",
			[]byte{1},
			trie.Version{Major: 1, Minor: 0},
		},
		{
			"with minor version",
			"test",
			[]byte{1},
			trie.Version{Major: 1, Minor: 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			result := b.AppendHistNodeKey(buf, tt.trieName, tt.path, tt.ver)

			assert.Equal(t, trieHistSpace, result[0])

			if b.HistPtnFactor != math.MaxUint32 {
				ptnBytes := result[1:5]
				ptn := binary.BigEndian.Uint32(ptnBytes)
				assert.Equal(t, tt.ver.Major/b.HistPtnFactor, ptn)
			}
		})
	}
}

func TestAppendDedupedNodeKey(t *testing.T) {
	b := backend{
		HistPtnFactor:    1000,
		DedupedPtnFactor: 1000,
		CachedNodeTTL:    100,
	}

	tests := []struct {
		name     string
		trieName string
		path     []byte
		ver      trie.Version
	}{
		{
			"empty path",
			"test",
			[]byte{},
			trie.Version{Major: 1, Minor: 0},
		},
		{
			"with path",
			"test",
			[]byte{1, 2, 3},
			trie.Version{Major: 1000, Minor: 1},
		},
		{
			"large version",
			"test",
			[]byte{1},
			trie.Version{Major: 999999, Minor: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			result := b.AppendDedupedNodeKey(buf, tt.trieName, tt.path, tt.ver)

			assert.Equal(t, trieDedupedSpace, result[0])

			if b.DedupedPtnFactor != math.MaxUint32 {
				ptnBytes := result[1:5]
				ptn := binary.BigEndian.Uint32(ptnBytes)
				assert.Equal(t, tt.ver.Major/b.DedupedPtnFactor, ptn)
			}
		})
	}
}

func TestDeleteHistoryNodes(t *testing.T) {
	b := backend{
		HistPtnFactor: 1000,
		Store:         &mockStoreBackend{},
	}

	mockStore := b.Store.(*mockStoreBackend)

	err := b.DeleteHistoryNodes(context.Background(), 1000, 2000)
	assert.Nil(t, err)
	assert.True(t, mockStore.deleteRangeCalled)

	expectedStart := binary.BigEndian.AppendUint32([]byte{trieHistSpace}, 1)
	expectedLimit := binary.BigEndian.AppendUint32([]byte{trieHistSpace}, 2)
	assert.True(t, bytes.Equal(expectedStart, mockStore.startKey))
	assert.True(t, bytes.Equal(expectedLimit, mockStore.limitKey))
}

func TestAppendNodePath(t *testing.T) {
	tests := []struct {
		name string
		path []byte
	}{
		{"empty path", []byte{}},
		{"single byte", []byte{0x12}},
		{"two bytes", []byte{0x12, 0x34}},
		{"three bytes", []byte{0x12, 0x34, 0x56}},
		{"four bytes", []byte{0x12, 0x34, 0x56, 0x78}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			result := appendNodePath(buf, tt.path)

			if len(tt.path) == 0 {
				assert.Equal(t, []byte{0, 0}, result)
				return
			}

			if len(tt.path) == 1 {
				assert.Equal(t, tt.path[0], result[0])
				assert.Equal(t, byte(1), result[1])
				return
			}

			assert.True(t, len(result) > 0)

			if len(tt.path) >= 2 {
				if len(tt.path) > 2 {
					assert.Equal(t, tt.path[0]|0x10, result[0])
				} else {
					assert.Equal(t, tt.path[0], result[0])
				}
				assert.Equal(t, (tt.path[1]<<4)|2, result[1])
			}
		})
	}
}
