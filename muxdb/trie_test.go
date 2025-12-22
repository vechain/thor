// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"context"
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/muxdb/engine"
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
		Cache:            &dummyCache{},
		HistPtnFactor:    1,
		DedupedPtnFactor: 1,
		CachedNodeTTL:    100,
	}
}

func TestTrieBasicOperations(t *testing.T) {
	var (
		name = "test-trie"
		back = newTestBackend()
	)

	tr := newTrie(name, back, trie.Root{})

	key := []byte("key1")
	value := []byte("value1")
	meta := []byte("meta1")

	err := tr.Update(key, value, meta)
	assert.Nil(t, err)

	gotValue, gotMeta, err := tr.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, value, gotValue)
	assert.Equal(t, meta, gotMeta)

	err = tr.Update(key, nil, nil)
	assert.Nil(t, err)

	gotValue, gotMeta, err = tr.Get(key)
	assert.Nil(t, err)
	assert.Nil(t, gotValue)
	assert.Nil(t, gotMeta)
}

func TestTrieVersioning(t *testing.T) {
	var (
		name  = "versioned-trie"
		back  = newTestBackend()
		roots []trie.Root
	)

	for i := range uint32(5) {
		var root trie.Root
		if len(roots) > 0 {
			root = roots[len(roots)-1]
		}

		tr := newTrie(name, back, root)

		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, i)
		value := []byte("value-" + string(key))

		err := tr.Update(key, value, nil)
		assert.Nil(t, err)

		err = tr.Commit(trie.Version{Major: i}, false)
		assert.Nil(t, err)

		roots = append(roots, trie.Root{
			Hash: tr.Hash(),
			Ver:  trie.Version{Major: i},
		})
	}

	for i, root := range roots {
		tr := newTrie(name, back, root)
		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, uint32(i))
		value := []byte("value-" + string(key))

		gotValue, _, err := tr.Get(key)
		assert.Nil(t, err)
		assert.Equal(t, value, gotValue)
	}
}

func TestTrieCaching(t *testing.T) {
	var (
		name = "cached-trie"
		back = newTestBackend()
	)

	tr := newTrie(name, back, trie.Root{})

	key := []byte("cached-key")
	value := []byte("cached-value")

	err := tr.Update(key, value, nil)
	assert.Nil(t, err)

	tr.SetNoFillCache(true)
	key2 := []byte("uncached-key")
	value2 := []byte("uncached-value")

	err = tr.Update(key2, value2, nil)
	assert.Nil(t, err)
}

func TestTrieCheckpoint(t *testing.T) {
	var (
		name = "checkpoint-trie"
		back = newTestBackend()
	)

	tr := newTrie(name, back, trie.Root{})

	for i := range uint32(3) {
		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, i)
		value := []byte("checkpoint-" + string(key))

		err := tr.Update(key, value, nil)
		assert.Nil(t, err)

		err = tr.Commit(trie.Version{Major: i}, false)
		assert.Nil(t, err)
	}

	leafCount := 0
	leafHandler := func(leaf *trie.Leaf) {
		leafCount++
	}

	err := tr.Checkpoint(context.Background(), 0, leafHandler)
	assert.Nil(t, err)
	assert.Greater(t, leafCount, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for i := range uint32(10000) {
		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, i)
		value := []byte("checkpoint-extra-" + string(key))

		err := tr.Update(key, value, nil)
		assert.Nil(t, err)
	}
	err = tr.Commit(trie.Version{Major: 10}, false)
	assert.Nil(t, err)

	err = tr.Checkpoint(ctx, 0, nil)
	assert.Equal(t, context.Canceled, err)
}

func TestTrieNodeIterator(t *testing.T) {
	var (
		name = "iterator-trie"
		back = newTestBackend()
	)

	tr := newTrie(name, back, trie.Root{})

	for i := range uint32(5) {
		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, i)
		value := []byte("iter-" + string(key))

		err := tr.Update(key, value, nil)
		assert.Nil(t, err)
	}

	iter := tr.NodeIterator(nil, 0)
	nodeCount := 0
	for iter.Next(true) {
		nodeCount++
	}
	assert.Greater(t, nodeCount, 0)
	assert.Nil(t, iter.Error())
}

func TestTrieCopy(t *testing.T) {
	var (
		name = "copy-trie"
		back = newTestBackend()
	)

	tr := newTrie(name, back, trie.Root{})

	key := []byte("original")
	value := []byte("value")
	err := tr.Update(key, value, nil)
	assert.Nil(t, err)

	trCopy := tr.Copy()

	gotValue, _, err := trCopy.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, value, gotValue)

	err = tr.Update(key, []byte("modified"), nil)
	assert.Nil(t, err)

	gotValue, _, err = trCopy.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, value, gotValue)
}

func TestContextChecker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	checker := newContextChecker(ctx, 1) // Set small debounce for faster testing
	cancel()

	for range 10 {
		err := checker()
		if err != nil {
			assert.Equal(t, context.Canceled, err)
			return
		}
	}
	t.Error("Expected context canceled error")
}

func TestGetAfterPrune(t *testing.T) {
	var (
		name = "prune-test"
		back = newTestBackend()
	)

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	engine := engine.NewLevelEngine(db)

	engine.DeleteRange(context.Background(), kv.Range{
		Start: []byte{trieHistSpace},
		Limit: []byte{trieHistSpace + 1},
	})

	mux := &MuxDB{
		engine:      engine,
		trieBackend: back,
		done:        make(chan struct{}),
	}

	key := []byte("key")
	root := trie.Root{}
	root2 := trie.Root{}
	root3 := trie.Root{}
	for i := range uint32(5) {
		tr := newTrie(name, back, root)

		value := []byte("checkpoint-" + strconv.Itoa(int(i)))

		err := tr.Update(key, value, nil)
		assert.Nil(t, err)

		ver := trie.Version{Major: i + 1}
		err = tr.Commit(ver, false)
		assert.Nil(t, err)
		root = trie.Root{
			Hash: tr.Hash(),
			Ver:  ver,
		}

		if i == 2 {
			root2 = root
		}
		if i == 3 {
			root3 = root
		}
	}

	tr := newTrie(name, back, root3)
	value, _, err := tr.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, "checkpoint-3", string(value))

	// checkpoint(squash) for [0,3], keep 4 in the hist space
	tr.Checkpoint(context.Background(), 0, nil)

	tr = newTrie(name, back, root2)
	value, _, err = tr.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, "checkpoint-2", string(value))

	// delete [0,4) in hist space
	err = mux.DeleteTrieHistoryNodes(context.Background(), 0, 4)
	assert.Nil(t, err)

	// version 4 can be still read
	tr = newTrie(name, back, root3)
	value, _, err = tr.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, "checkpoint-3", string(value))

	// version 2 can not be read
	tr = newTrie(name, back, root2)
	_, _, err = tr.Get(key)
	assert.Contains(t, err.Error(), "missing trie node")
}
