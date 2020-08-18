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
	"github.com/vechain/thor/thor"
)

const (
	trieSpaceP         = byte(15) // the space to store permanent(pruned) trie nodes
	trieSpaceA         = byte(0)  // the space to store live trie nodes
	trieSpaceB         = byte(1)  // the space to store live trie nodes
	trieSecureKeySpace = byte(16)
	namedStoreSpace    = byte(32)

	propsStoreName = "muxdb.props"
)

type engine interface {
	kv.Store
	Close() error
}

// Options optional parameters for MuxDB.
type Options struct {
	// EncodedTrieNodeCacheSizeMB is the size of encoded trie node cache.
	EncodedTrieNodeCacheSizeMB int
	// DecodedTrieNodeCacheCapacity is max count of cached decoded trie nodes.
	DecodedTrieNodeCacheCapacity int
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
	engine        engine
	trieCache     *trieCache
	trieLiveSpace *trieLiveSpace
	storageCloser io.Closer
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
			util.BytesPrefix([]byte{trieSpaceA}),
			util.BytesPrefix([]byte{trieSpaceB}),
			util.BytesPrefix([]byte{trieSecureKeySpace}),
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

	propsStore := newNamedStore(engine, propsStoreName)
	trieLiveSpace, err := newTrieLiveSpace(propsStore)
	if err != nil {
		engine.Close()
		return nil, err
	}

	return &MuxDB{
		engine: engine,
		trieCache: newTrieCache(
			options.EncodedTrieNodeCacheSizeMB,
			options.DecodedTrieNodeCacheCapacity),
		trieLiveSpace: trieLiveSpace,
		storageCloser: storage,
	}, nil
}

// NewMem creates a memory-backed DB.
func NewMem() *MuxDB {
	storage := storage.NewMemStorage()
	ldb, _ := leveldb.Open(storage, nil)

	engine := newLevelEngine(ldb)
	propsStore := newNamedStore(engine, propsStoreName)
	trieLiveSpace, _ := newTrieLiveSpace(propsStore)

	return &MuxDB{
		engine:        newLevelEngine(ldb),
		trieCache:     newTrieCache(0, 8192),
		trieLiveSpace: trieLiveSpace,
		storageCloser: storage,
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
func (db *MuxDB) NewTrie(name string, root thor.Bytes32) *Trie {
	return newTrie(
		db.engine,
		name,
		root,
		db.trieCache,
		false,
		db.trieLiveSpace,
	)
}

// NewSecureTrie creates secure trie.
// In a secure trie, keys are hashed using blake2b. It prevents depth attack.
func (db *MuxDB) NewSecureTrie(name string, root thor.Bytes32) *Trie {
	return newTrie(
		db.engine,
		name,
		root,
		db.trieCache,
		true,
		db.trieLiveSpace,
	)
}

// NewTriePruner creates trie pruner.
func (db *MuxDB) NewTriePruner() *TriePruner {
	return newTriePruner(db)
}

// NewStore creates named kv-store.
func (db *MuxDB) NewStore(name string) kv.Store {
	return newNamedStore(db.engine, name)
}

// LowStore returns underlying kv-store. It's for test purpose only.
func (db *MuxDB) LowStore() kv.Store {
	return db.engine
}

// IsNotFound returns if the error indicates key not found.
func (db *MuxDB) IsNotFound(err error) bool {
	return db.engine.IsNotFound(err)
}

func newNamedStore(src kv.Store, name string) kv.Store {
	bkt := bucket(append([]byte{namedStoreSpace}, name...))
	return &struct {
		kv.Getter
		kv.Putter
		kv.SnapshotFunc
		kv.BatchFunc
		kv.IterateFunc
		kv.IsNotFoundFunc
	}{
		bkt.ProxyGetter(src),
		bkt.ProxyPutter(src),
		func(fn func(kv.Getter) error) error {
			return src.Snapshot(func(getter kv.Getter) error {
				return fn(bkt.ProxyGetter(getter))
			})
		},
		func(fn func(kv.PutFlusher) error) error {
			return src.Batch(func(putter kv.PutFlusher) error {
				return fn(struct {
					kv.Putter
					kv.FlushFunc
				}{
					bkt.ProxyPutter(putter),
					putter.Flush,
				})
			})
		},
		func(r kv.Range, fn func(kv.Pair) bool) error {
			return src.Iterate(bkt.MakeRange(r), func(pair kv.Pair) bool {
				return fn(bkt.MakePair(pair))
			})
		},
		src.IsNotFound,
	}
}
