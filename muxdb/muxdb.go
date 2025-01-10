// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"context"
	"encoding/json"
	"os"
	"syscall"

	"github.com/syndtr/goleveldb/leveldb"
	dberrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/muxdb/engine"
	"github.com/vechain/thor/v2/trie"
)

const (
	trieHistSpace    = byte(0) // the key space for historical trie nodes.
	trieDedupedSpace = byte(1) // the key space for deduped trie nodes.
	namedStoreSpace  = byte(2) // the key space for named store.
)

const (
	propStoreName = "muxdb.props"
	configKey     = "config"
)

var logger = log.WithContext("pkg", "muxdb")

// Options optional parameters for MuxDB.
type Options struct {
	// TrieNodeCacheSizeMB is the size of the cache for trie node blobs.
	TrieNodeCacheSizeMB int
	// TrieCachedNodeTTL defines the life time(times of commit) of cached trie nodes.
	TrieCachedNodeTTL uint16
	// TrieHistPartitionFactor is the partition factor for historical trie nodes.
	TrieHistPartitionFactor uint32
	// TrieDedupedPartitionFactor is the partition factor for deduped trie nodes.
	TrieDedupedPartitionFactor uint32
	// TrieWillCleanHistory is the hint to tell if historical nodes will be cleaned.
	TrieWillCleanHistory bool

	// OpenFilesCacheCapacity is the capacity of open files caching for underlying database.
	OpenFilesCacheCapacity int
	// ReadCacheMB is the size of read cache for underlying database.
	ReadCacheMB int
	// WriteBufferMB is the size of write buffer for underlying database.
	WriteBufferMB int
}

// MuxDB is the database to efficiently store state trie and block-chain data.
type MuxDB struct {
	engine      engine.Engine
	trieBackend *backend
}

// Adds metrics if the error is due to file/db lock.
func addMetricsIfLocked(err error, event string) {
	if err == nil {
		return
	}

	if pathErr, ok := err.(*os.PathError); ok {
		// Eventually calls to openFileNoLog https://go.dev/src/os/file_unix.go
		if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EWOULDBLOCK {
			metricLeveldbLock().AddWithLabel(1, map[string]string{"event": event})
		}
	} else if err == syscall.EWOULDBLOCK { // setFileLock https://github.com/vechain/goleveldb/blob/master/leveldb/storage/file_storage_unix.go#L50
		metricLeveldbLock().AddWithLabel(1, map[string]string{"event": event})
	} else if err == storage.ErrLocked { // If using sessions
		metricLeveldbLock().AddWithLabel(1, map[string]string{"event": event})
	}
}

// Open opens or creates DB at the given path.
func Open(path string, options *Options) (*MuxDB, error) {
	// prepare leveldb options
	ldbOpts := opt.Options{
		OpenFilesCacheCapacity: options.OpenFilesCacheCapacity,
		BlockCacheCapacity:     options.ReadCacheMB * opt.MiB,
		WriteBuffer:            options.WriteBufferMB * opt.MiB,
		Filter:                 filter.NewBloomFilter(10),
		BlockSize:              1024 * 32, // balance performance of point reads and compression ratio.
		CompactionTableSize:    4 * opt.MiB,
	}

	if options.TrieWillCleanHistory {
		// this option gets disk space efficiently reclaimed.
		// only set when pruner enabled.
		ldbOpts.OverflowPrefix = []byte{trieHistSpace}
	}

	// open leveldb
	ldb, err := leveldb.OpenFile(path, &ldbOpts)
	addMetricsIfLocked(err, "open-file")
	if _, corrupted := err.(*dberrors.ErrCorrupted); corrupted {
		ldb, err = leveldb.RecoverFile(path, &ldbOpts)
		addMetricsIfLocked(err, "recover-file")
	}

	if err != nil {
		return nil, err
	}

	// as engine
	engine := engine.NewLevelEngine(ldb)

	propStore := kv.Bucket(string(namedStoreSpace) + propStoreName).NewStore(engine)
	// persists critical options to avoid corruption when tweaked.
	cfg := config{
		HistPtnFactor:    options.TrieHistPartitionFactor,
		DedupedPtnFactor: options.TrieDedupedPartitionFactor,
	}
	if err := cfg.LoadOrSave(propStore); err != nil {
		ldb.Close()
		return nil, err
	}

	return &MuxDB{
		engine: engine,
		trieBackend: &backend{
			Store: engine,
			Cache: newCache(
				options.TrieNodeCacheSizeMB,
				uint32(options.TrieCachedNodeTTL)),
			HistPtnFactor:    cfg.HistPtnFactor,
			DedupedPtnFactor: cfg.DedupedPtnFactor,
			CachedNodeTTL:    options.TrieCachedNodeTTL,
		},
	}, nil
}

// NewMem creates a memory-backed DB.
func NewMem() *MuxDB {
	storage := storage.NewMemStorage()
	ldb, err := leveldb.Open(storage, nil)
	addMetricsIfLocked(err, "open-memory-backed-db")

	engine := engine.NewLevelEngine(ldb)
	return &MuxDB{
		engine: engine,
		trieBackend: &backend{
			Store:            engine,
			Cache:            &dummyCache{},
			HistPtnFactor:    1,
			DedupedPtnFactor: 1,
			CachedNodeTTL:    32,
		},
	}
}

// Close closes the DB.
func (db *MuxDB) Close() error {
	return db.engine.Close()
}

// NewTrie creates trie with existing root node.
// If root is zero value, the trie is initially empty.
func (db *MuxDB) NewTrie(name string, root trie.Root) *Trie {
	return newTrie(
		name,
		db.trieBackend,
		root,
	)
}

// DeleteTrieHistoryNodes deletes trie history nodes within partitions of [startMajorVer, limitMajorVer).
func (db *MuxDB) DeleteTrieHistoryNodes(ctx context.Context, startMajorVer, limitMajorVer uint32) error {
	return db.trieBackend.DeleteHistoryNodes(ctx, startMajorVer, limitMajorVer)
}

// NewStore creates named kv-store.
func (db *MuxDB) NewStore(name string) kv.Store {
	return kv.Bucket(string(namedStoreSpace) + name).NewStore(db.engine)
}

// IsNotFound returns if the error indicates key not found.
func (db *MuxDB) IsNotFound(err error) bool {
	return db.engine.IsNotFound(err)
}

type config struct {
	HistPtnFactor    uint32
	DedupedPtnFactor uint32
}

func (c *config) LoadOrSave(store kv.Store) error {
	// try to load
	data, err := store.Get([]byte(configKey))
	if err == nil {
		// and decode
		return json.Unmarshal(data, c)
	}

	if !store.IsNotFound(err) {
		return err
	}
	// not found
	// encode and save
	data, err = json.Marshal(c)
	if err != nil {
		return err
	}
	return store.Put([]byte(configKey), data)
}
