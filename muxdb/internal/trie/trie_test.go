// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/muxdb/internal/engine"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

func newEngine() engine.Engine {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return engine.NewLevelEngine(db)
}

func newBackend() *Backend {
	engine := newEngine()
	return &Backend{
		Store:            engine,
		Cache:            nil,
		LeafBank:         NewLeafBank(engine, 2, 100),
		HistSpace:        0,
		DedupedSpace:     1,
		HistPtnFactor:    1,
		DedupedPtnFactor: 1,
		CachedNodeTTL:    100,
	}
}

func TestTrie(t *testing.T) {
	name := "the trie"

	t.Run("basic", func(t *testing.T) {
		back := newBackend()
		tr := New(back, name, thor.Bytes32{}, 0, 0, false)
		assert.Equal(t, name, tr.Name())

		assert.False(t, tr.dirty)

		key := []byte("key")
		val := []byte("value")
		tr.Update(key, val, nil)
		assert.True(t, tr.dirty)

		_val, _, _ := tr.Get(key)
		assert.Equal(t, val, _val)
	})

	t.Run("hash root", func(t *testing.T) {
		back := newBackend()
		tr := New(back, name, thor.Bytes32{}, 0, 0, false)

		_tr := new(trie.Trie)

		for i := 0; i < 100; i++ {
			for j := 0; j < 100; j++ {
				key := []byte(strconv.Itoa(i) + "_" + strconv.Itoa(j))
				val := []byte("v" + strconv.Itoa(j) + "_" + strconv.Itoa(i))
				tr.Update(key, val, nil)
				_tr.Update(key, val)
			}
			h, _ := tr.Stage(0, 0)
			assert.Equal(t, _tr.Hash(), h)
		}
	})

	t.Run("fast get", func(t *testing.T) {
		back := newBackend()
		tr := New(back, name, thor.Bytes32{}, 0, 0, false)

		var roots []thor.Bytes32
		for i := 0; i < 100; i++ {
			for j := 0; j < 100; j++ {
				key := []byte(strconv.Itoa(i) + "_" + strconv.Itoa(j))
				val := []byte("v" + strconv.Itoa(j) + "_" + strconv.Itoa(i))
				tr.Update(key, val, nil)
			}
			root, commit := tr.Stage(uint32(i), 0)
			if err := commit(); err != nil {
				t.Fatal(err)
			}

			roots = append(roots, root)
		}

		tr = New(back, name, roots[10], 10, 0, false)

		if err := tr.DumpLeaves(context.Background(), 0, 10, func(l *trie.Leaf) *trie.Leaf {
			return &trie.Leaf{
				Value: l.Value,
				Meta:  []byte("from lb"),
			}
		}); err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 10; i++ {
			for j := 0; j < 100; j++ {
				key := []byte(strconv.Itoa(i) + "_" + strconv.Itoa(j))
				val := []byte("v" + strconv.Itoa(j) + "_" + strconv.Itoa(i))

				_val, _meta, err := tr.FastGet(key, 10)
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, val, _val)
				assert.Equal(t, []byte("from lb"), _meta)
			}
		}
	})
}
