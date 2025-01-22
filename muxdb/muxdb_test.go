// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/trie"
)

func TestNewMuxDB(t *testing.T) {
	opts := Options{
		TrieNodeCacheSizeMB:        128,
		TrieCachedNodeTTL:          30,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       true,
		OpenFilesCacheCapacity:     512,
		ReadCacheMB:                256,
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    1000,
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path, &opts)
	assert.Nil(t, err)
	defer db.Close()

	assert.NotNil(t, db.engine)
	assert.NotNil(t, db.trieBackend)
	assert.NotNil(t, db.done)
}

func TestNewMemDB(t *testing.T) {
	db := NewMem()
	defer db.Close()

	assert.NotNil(t, db.engine)
	assert.NotNil(t, db.trieBackend)
	assert.Equal(t, uint32(1), db.trieBackend.HistPtnFactor)
	assert.Equal(t, uint32(1), db.trieBackend.DedupedPtnFactor)
	assert.Equal(t, uint16(32), db.trieBackend.CachedNodeTTL)
}

func TestDBStore(t *testing.T) {
	db := NewMem()
	defer db.Close()

	store := db.NewStore("test")

	tests := []struct {
		key   []byte
		value []byte
	}{
		{[]byte("key1"), []byte("value1")},
		{[]byte{0x00}, []byte{0x01}},
		{[]byte("large-key"), make([]byte, 1024)},
		{nil, []byte("value")},
		{[]byte("key"), nil},
	}

	for _, tt := range tests {
		err := store.Put(tt.key, tt.value)
		assert.Nil(t, err)

		got, err := store.Get(tt.key)
		assert.Nil(t, err)
		assert.Equal(t, tt.value, got)

		err = store.Delete(tt.key)
		assert.Nil(t, err)

		_, err = store.Get(tt.key)
		assert.True(t, db.IsNotFound(err))
	}
}

type mockStore struct {
	kv.Store
	getErr error
	putErr error
}

func (m *mockStore) Get(key []byte) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return nil, errors.New("not found")
}

func (m *mockStore) Put(key, value []byte) error {
	return m.putErr
}

func (m *mockStore) IsNotFound(err error) bool {
	return err != nil && err.Error() == "not found"
}

func TestDBConfig(t *testing.T) {
	db := NewMem()
	defer db.Close()

	store := db.NewStore(propStoreName)

	cfg := config{
		HistPtnFactor:    2000,
		DedupedPtnFactor: 3000,
	}

	err := cfg.LoadOrSave(store)
	assert.Nil(t, err)

	loaded := config{}
	err = loaded.LoadOrSave(store)
	assert.Nil(t, err)

	assert.Equal(t, cfg.HistPtnFactor, loaded.HistPtnFactor)
	assert.Equal(t, cfg.DedupedPtnFactor, loaded.DedupedPtnFactor)

	// Test Get error that is not NotFound
	_mockStore := &mockStore{getErr: errors.New("db error")}
	err = loaded.LoadOrSave(_mockStore)
	assert.Equal(t, errors.New("db error"), err)

	// Test Put error
	_mockStore = &mockStore{putErr: errors.New("put error")}
	err = loaded.LoadOrSave(_mockStore)
	assert.Equal(t, errors.New("put error"), err)

	// Test invalid JSON unmarshal
	invalidData := []byte("invalid-json")
	err = store.Put([]byte(configKey), invalidData)
	assert.Nil(t, err)

	err = loaded.LoadOrSave(store)
	assert.NotNil(t, err)
}

func TestCorruptDBRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.db")

	opts := &Options{
		TrieNodeCacheSizeMB:        128,
		TrieHistPartitionFactor:    1000,
		TrieDedupedPartitionFactor: 2000,
		OpenFilesCacheCapacity:     16,
		ReadCacheMB:                32,
		WriteBufferMB:              16,
	}

	db, err := Open(path, opts)
	assert.Nil(t, err)
	db.Close()

	corruptFile := filepath.Join(path, "CURRENT")
	err = os.WriteFile(corruptFile, []byte("corrupted"), 0644)
	assert.Nil(t, err)

	db, err = Open(path, opts)
	if assert.Error(t, err) {
		assert.IsType(t, &storage.ErrCorrupted{}, err)
		return
	}
	db.Close()
}

func TestTrieOperations(t *testing.T) {
	db := NewMem()
	defer db.Close()

	tr := db.NewTrie("test", trie.Root{})

	key := []byte("test-key")
	value := []byte("test-value")

	err := tr.Update(key, value, nil)
	assert.Nil(t, err)

	ver := trie.Version{Major: 1}
	err = tr.Commit(ver, false)
	assert.Nil(t, err)

	root := tr.Hash()
	tr2 := db.NewTrie("test", trie.Root{Hash: root, Ver: ver})

	val, _, err := tr2.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, value, val)
}

func TestEnableMetrics(t *testing.T) {
	db := NewMem()
	db.EnableMetrics()

	time.Sleep(metricsSampleInterval + time.Second)
	db.Close()
}

func TestMultipleStores(t *testing.T) {
	db := NewMem()
	defer db.Close()

	store1 := db.NewStore("store1")
	store2 := db.NewStore("store2")

	key := []byte("key")
	val1 := []byte("val1")
	val2 := []byte("val2")

	err := store1.Put(key, val1)
	assert.Nil(t, err)
	err = store2.Put(key, val2)
	assert.Nil(t, err)

	got1, err := store1.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val1, got1)

	got2, err := store2.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, val2, got2)
}

type mockEngine struct {
	kv.Store
	statsCalled bool
}

func (m *mockEngine) Stats(stats *leveldb.DBStats) error {
	m.statsCalled = true
	return errors.New("mock stats error")
}

func (m *mockEngine) Close() error {
	return nil
}

func TestDeleteTrieHistory(t *testing.T) {
	db := NewMem()
	defer db.Close()

	tr := db.NewTrie("test", trie.Root{})
	key := []byte("key")
	value := []byte("value")

	err := tr.Update(key, value, nil)
	assert.Nil(t, err)

	ver1 := trie.Version{Major: 1}
	err = tr.Commit(ver1, false)
	assert.Nil(t, err)

	err = db.DeleteTrieHistoryNodes(context.Background(), 0, 2)
	assert.Nil(t, err)
}
