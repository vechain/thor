// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/v2/trie"
)

func TestMuxdb(t *testing.T) {
	var err error
	db := NewMem()
	db.Close()

	dir := os.TempDir()

	opts := Options{
		TrieNodeCacheSizeMB:        128,
		TrieCachedNodeTTL:          30, // 5min
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       true,
		OpenFilesCacheCapacity:     512,
		ReadCacheMB:                256, // rely on os page cache other than huge db read cache.
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    1000,
	}
	path := filepath.Join(dir, "main.db")
	db, err = Open(path, &opts)
	assert.Nil(t, err)

	err = db.Close()
	assert.Nil(t, err)

	os.RemoveAll(path)
}

func TestConfigLoadSave(t *testing.T) {
	db := NewMem()
	defer db.Close()

	store := db.NewStore(propStoreName)

	cfg := config{
		HistPtnFactor:    1000,
		DedupedPtnFactor: 2000,
	}
	err := cfg.LoadOrSave(store)
	assert.Nil(t, err)

	cfg2 := config{}
	err = cfg2.LoadOrSave(store)
	assert.Nil(t, err)
	assert.Equal(t, cfg.HistPtnFactor, cfg2.HistPtnFactor)
	assert.Equal(t, cfg.DedupedPtnFactor, cfg2.DedupedPtnFactor)
}

func TestCompactionMetrics(t *testing.T) {
	stor := storage.NewMemStorage()
	ldb, _ := leveldb.Open(stor, nil)
	defer ldb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		collectCompactionMetrics(ctx, ldb)
		close(done)
	}()

	// Wait for collection to finish
	select {
	case <-done:
		// Collection finished as expected
	case <-time.After(3 * time.Second):
		t.Fatal("Collection didn't finish in time")
	}
}

func TestCorruptedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupted.db")

	opts := &Options{
		TrieNodeCacheSizeMB:        128,
		TrieHistPartitionFactor:    1000,
		TrieDedupedPartitionFactor: 2000,
		OpenFilesCacheCapacity:     16,
		ReadCacheMB:                32,
		WriteBufferMB:              16,
	}

	db, err := Open(dbPath, opts)
	assert.Nil(t, err)
	db.Close()

	// Corrupt the database by writing invalid data
	f, _ := os.OpenFile(filepath.Join(dbPath, "CURRENT"), os.O_WRONLY|os.O_TRUNC, 0644)
	f.WriteString("invalid data")
	f.Close()

	// Try to open corrupted database
	db, err = Open(dbPath, opts)
	if assert.Error(t, err) {
		assert.IsType(t, &storage.ErrCorrupted{}, err)
		return
	}
	db.Close()
}

func TestStore(t *testing.T) {
	db := NewMem()

	store := db.NewStore("test")
	key := []byte("key")
	val := []byte("val")

	store.Put(key, val)
	v, err := store.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val, v)

	store.Delete(key)
	_, err = store.Get(key)
	assert.True(t, db.IsNotFound(err))

	db.Close()
}

func TestAdvancedTrie(t *testing.T) {
	db := NewMem()
	defer db.Close()

	tr := db.NewTrie("advanced-test", trie.Root{})

	updates := map[string][]byte{
		"key1": []byte("val1"),
		"key2": []byte("val2"),
		"key3": []byte("val3"),
	}

	for k, v := range updates {
		err := tr.Update([]byte(k), v, nil)
		assert.Nil(t, err)
	}

	ver := trie.Version{Major: 1, Minor: 0}
	err := tr.Commit(ver, false)
	assert.Nil(t, err)

	// Verify all values
	for k, v := range updates {
		val, _, err := tr.Get([]byte(k))
		assert.Nil(t, err)
		assert.Equal(t, v, val)
	}

	// Test deletion
	for k := range updates {
		err := tr.Update([]byte(k), nil, nil)
		assert.Nil(t, err)
	}

	ver = trie.Version{Major: 2, Minor: 0}
	err = tr.Commit(ver, false)
	assert.Nil(t, err)

	// Verify deletions
	for k := range updates {
		val, _, err := tr.Get([]byte(k))
		assert.Nil(t, err)
		assert.Nil(t, val)
	}
}

func TestMuxdbTrie(t *testing.T) {
	var err error
	db := NewMem()

	tr := db.NewTrie("test", trie.Root{})
	tr.SetNoFillCache(true)
	key := []byte("key")
	val1 := []byte("val")
	val2 := []byte("val2")

	ver1 := trie.Version{Major: 1, Minor: 0}
	ver2 := trie.Version{Major: 100, Minor: 0}
	ver3 := trie.Version{Major: 101, Minor: 0}

	err = tr.Update(key, val1, nil)
	assert.Nil(t, err)
	err = tr.Commit(ver1, false)
	assert.Nil(t, err)

	root1 := tr.Hash()
	tr1 := db.NewTrie("test", trie.Root{Hash: root1, Ver: ver1})
	tr1.SetNoFillCache(true)
	v, _, err := tr1.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val1, v)

	tr1.Update(key, val2, nil)
	err = tr1.Commit(ver2, false)
	assert.Nil(t, err)
	root2 := tr1.Hash()

	tr2 := db.NewTrie("test", trie.Root{Hash: root2, Ver: ver2})
	tr2.SetNoFillCache(true)
	v, _, err = tr2.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val2, v)

	err = tr2.Commit(ver3, false)
	assert.Nil(t, err)
	root3 := tr2.Hash()

	//prune trie [0, ver3)
	xtr := db.NewTrie("test", trie.Root{Hash: root2, Ver: ver2})
	err = xtr.Checkpoint(context.Background(), 0, nil)
	assert.Nil(t, err)
	err = db.DeleteTrieHistoryNodes(context.Background(), 0, ver3.Major)
	assert.Nil(t, err)

	//after delete history nodes，the history nodes should be deleted
	path := []byte{}

	histKey := xtr.back.AppendHistNodeKey(nil, "test", path, ver1)
	_, err = xtr.back.Store.Get(histKey)
	assert.True(t, db.IsNotFound(err))

	histKey = xtr.back.AppendHistNodeKey(nil, "test", path, ver2)
	_, err = xtr.back.Store.Get(histKey)
	assert.True(t, db.IsNotFound(err))

	histKey = xtr.back.AppendHistNodeKey(nil, "test", path, ver3)
	_, err = xtr.back.Store.Get(histKey)
	assert.Nil(t, err)

	dedupedKey := xtr.back.AppendDedupedNodeKey(nil, "test", path, ver2)
	blob, err := xtr.back.Store.Get(dedupedKey)
	assert.Nil(t, err)
	assert.NotNil(t, blob)

	tr4 := db.NewTrie("test", trie.Root{Hash: root2, Ver: ver2})
	v, _, err = tr4.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val2, v)

	tr5 := db.NewTrie("test", trie.Root{Hash: root3, Ver: ver3})
	v, _, err = tr5.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val2, v)

	db.Close()
}
