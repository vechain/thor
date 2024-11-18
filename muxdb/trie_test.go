// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/v2/muxdb/engine"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func newTestEngine() engine.Engine {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return engine.NewLevelEngine(db)
}

func newTestBackend() *backend {
	engine := newTestEngine()
	return &backend{
		Store:            engine,
		Cache:            &emptyCache{},
		HistPtnFactor:    1,
		DedupedPtnFactor: 1,
		CachedNodeTTL:    100,
	}
}

func TestTrie(t *testing.T) {
	var (
		name  = "the trie"
		back  = newTestBackend()
		round = uint32(200)
		roots []trie.Root
	)

	for i := uint32(0); i < round; i++ {
		var root trie.Root
		if len(roots) > 0 {
			root = roots[len(roots)-1]
		}

		tr := newTrie(name, back, root)
		key := thor.Blake2b(binary.BigEndian.AppendUint32(nil, i)).Bytes()
		val := thor.Blake2b(key).Bytes()
		meta := thor.Blake2b(val).Bytes()
		err := tr.Update(key, val, meta)
		assert.Nil(t, err)

		err = tr.Commit(trie.Version{Major: i}, false)
		assert.Nil(t, err)

		roots = append(roots, trie.Root{
			Hash: tr.Hash(),
			Ver:  trie.Version{Major: i},
		})
	}

	for _i, root := range roots {
		tr := newTrie(name, back, root)
		for i := uint32(0); i <= uint32(_i); i++ {
			key := thor.Blake2b(binary.BigEndian.AppendUint32(nil, i)).Bytes()
			val := thor.Blake2b(key).Bytes()
			meta := thor.Blake2b(val).Bytes()
			_val, _meta, err := tr.Get(key)
			assert.Nil(t, err)
			assert.Equal(t, val, _val)
			assert.Equal(t, meta, _meta)
		}
	}
}
