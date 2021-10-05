// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"io"

	"github.com/syndtr/goleveldb/leveldb"
	dberrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb/internal/trie"
	"github.com/vechain/thor/thor"
)

const (
	namedStoreSpace = byte(32)
)

type engine interface {
	kv.Store
	Close() error
}

// Trie is the managed trie.
type Trie = trie.Trie

type TrieLeafBank = trie.LeafBank
type TrieSaveLeaf = trie.SaveLeaf

// TrieDedupedNodePartitionFactor the partition factor for deduped nodes.
const TrieDedupedNodePartitionFactor = trie.PartitionFactor(500000)

// Options optional parameters for MuxDB.
type Options struct {
	// TrieNodeCacheSizeMB is the size of the cache for trie node blobs.
	TrieNodeCacheSizeMB int
	// TrieRootCacheCapacity is the capacity of the cache for trie root nodes.
	TrieRootCacheCapacity int
	// TrieCachedNodeTTL defines the life time(times of commit) of cached trie nodes.
	TrieCachedNodeTTL int
	// TrieLeafBankSlotCapacity defines max count of cached slot for leaf bank.
	TrieLeafBankSlotCapacity int
	// TrieHistNodePartitionFactor defines the partition factor for history nodes.
	// It must be consistant over the lifetime of the DB.
	TrieHistNodePartitionFactor trie.PartitionFactor

	// OpenFilesCacheCapacity is the capacity of open files caching for underlying database.
	OpenFilesCacheCapacity int
	// ReadCacheMB is the size of read cache for underlying database.
	ReadCacheMB int
	// WriteBufferMB is the size of write buffer for underlying database.
	WriteBufferMB int
	// DisablePageCache Disable page cache for database file.
	// It's for test purpose only.
	DisablePageCache bool
}

// MuxDB is the database to efficiently store state trie and block-chain data.
type MuxDB struct {
	engine            engine
	storageCloser     io.Closer
	trieBackend       *trie.Backend
	trieLeafBank      *trie.LeafBank
	trieCachedNodeTTL int
}

// Open opens or creates DB at the given path.
func Open(path string, options *Options) (*MuxDB, error) {
	// prepare leveldb options
	ldbOpts := opt.Options{
		OpenFilesCacheCapacity:        options.OpenFilesCacheCapacity,
		BlockCacheCapacity:            options.ReadCacheMB * opt.MiB,
		WriteBuffer:                   options.WriteBufferMB * opt.MiB,
		Filter:                        filter.NewBloomFilter(10),
		BlockSize:                     1024 * 32, // balance performance of point reads and compression ratio.
		DisableSeeksCompaction:        true,
		CompactionTableSizeMultiplier: 2,
		VibrantKeys: []*util.Range{
			util.BytesPrefix([]byte{trie.LeafBankSpace}),
		},
	}

	storage, err := openLevelFileStorage(path, false, options.DisablePageCache)
	if err != nil {
		return nil, err
	}

	// open leveldb
	ldb, err := leveldb.Open(storage, &ldbOpts)
	if _, corrupted := err.(*dberrors.ErrCorrupted); corrupted {
		ldb, err = leveldb.Recover(storage, &ldbOpts)
	}
	if err != nil {
		storage.Close()
		return nil, err
	}

	// as engine
	engine := newLevelEngine(ldb)

	return &MuxDB{
		engine:        engine,
		storageCloser: storage,
		trieBackend: &trie.Backend{
			Store:            engine,
			Cache:            trie.NewCache(options.TrieNodeCacheSizeMB, options.TrieRootCacheCapacity),
			HistPtnFactor:    options.TrieHistNodePartitionFactor,
			DedupedPtnFactor: TrieDedupedNodePartitionFactor,
		},
		trieLeafBank:      trie.NewLeafBank(engine, options.TrieLeafBankSlotCapacity),
		trieCachedNodeTTL: options.TrieCachedNodeTTL,
	}, nil
}

// NewMem creates a memory-backed DB.
func NewMem() *MuxDB {
	storage := storage.NewMemStorage()
	ldb, _ := leveldb.Open(storage, nil)

	engine := newLevelEngine(ldb)
	return &MuxDB{
		engine:        engine,
		storageCloser: storage,
		trieBackend: &trie.Backend{
			Store:            engine,
			HistPtnFactor:    1,
			DedupedPtnFactor: TrieDedupedNodePartitionFactor,
		},
		trieLeafBank:      trie.NewLeafBank(engine, 1),
		trieCachedNodeTTL: 32,
	}
}

// Close closes the DB.
func (db *MuxDB) Close() error {
	err := db.engine.Close()
	if err1 := db.storageCloser.Close(); err == nil {
		err = err1
	}
	return err
}

// NewTrie creates trie either with existing root node.
//
// If root is zero or blake2b hash of an empty string, the trie is
// initially empty.
func (db *MuxDB) NewTrie(name string, root thor.Bytes32, commitNum uint32) *Trie {
	return trie.New(
		db.trieBackend,
		name,
		false,
		root,
		commitNum,
		db.trieCachedNodeTTL,
	)
}

// NewSecureTrie creates secure trie.
// In a secure trie, keys are hashed using blake2b. It prevents depth attack.
func (db *MuxDB) NewSecureTrie(name string, root thor.Bytes32, commitNum uint32) *Trie {
	return trie.New(
		db.trieBackend,
		name,
		true,
		root,
		commitNum,
		db.trieCachedNodeTTL,
	)
}

// NewStore creates named kv-store.
func (db *MuxDB) NewStore(name string) kv.Store {
	return kv.Bucket(string(namedStoreSpace) + name).NewStore(db.engine)
}

// LowEngine returns underlying kv-engine. It's for test purpose only.
func (db *MuxDB) LowEngine() kv.Store {
	return db.engine
}

// IsNotFound returns if the error indicates key not found.
func (db *MuxDB) IsNotFound(err error) bool {
	return db.engine.IsNotFound(err)
}

func (db *MuxDB) TrieLeafBank() *TrieLeafBank {
	return db.trieLeafBank
}
