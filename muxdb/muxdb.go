// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"context"
	"errors"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/cockroachdb/pebble/vfs"
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

type EngineType int

const (
	LevelDB EngineType = 0
	Pebble  EngineType = 1
)

// Options optional parameters for MuxDB.
type Options struct {
	// Engine specifies underlying KV engine.
	// defaults to level db.
	Engine EngineType
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
	// PermanentTrie if set to true, tries always commit nodes into permanent space, so pruner
	// will have no effect.
	PermanentTrie bool
	// DisablePageCache Disable page cache for database file.
	// It's for test purpose only.
	DisablePageCache bool
}

// MuxDB is the database to efficiently store state trie and block-chain data.
type MuxDB struct {
	engine        engine
	trieCache     *trieCache
	trieLiveSpace *trieLiveSpace
	permanentTrie bool
}

func newEngine(path string, options *Options) (engine, error) {
	if options.Engine == LevelDB {
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

		storage, err := storage.OpenFile(path, false)
		if err != nil {
			return nil, err
		}
		if options.DisablePageCache {
			storage = &leveldbStorageNoPageCache{storage}
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
		return newLevelEngine(ldb, storage), nil
	} else if options.Engine == Pebble {
		var fs vfs.FS
		if options.DisablePageCache {
			fs = &pebbleFSNoPageCache{vfs.Default}
		}

		cache := pebble.NewCache(int64(options.ReadCacheMB * opt.MiB))
		defer cache.Unref()

		opts := &pebble.Options{
			Cache:                       cache,
			L0CompactionThreshold:       2,
			L0StopWritesThreshold:       1000,
			FS:                          fs,
			LBaseMaxBytes:               int64(options.WriteBufferMB * opt.MiB),
			MemTableSize:                options.WriteBufferMB * opt.MiB,
			MemTableStopWritesThreshold: 4,
			MaxConcurrentCompactions:    3,
			MaxOpenFiles:                options.OpenFilesCacheCapacity,
			Levels:                      make([]pebble.LevelOptions, 7),
		}

		for i := 0; i < len(opts.Levels); i++ {
			l := &opts.Levels[i]
			l.BlockSize = 32 << 10       // 32 KB
			l.IndexBlockSize = 256 << 10 // 256 KB
			l.FilterPolicy = bloom.FilterPolicy(10)
			l.FilterType = pebble.TableFilter
			if i > 0 {
				l.TargetFileSize = opts.Levels[i-1].TargetFileSize * 2
			}
			l.EnsureDefaults()
		}
		opts.Levels[len(opts.Levels)-1].FilterPolicy = nil

		db, err := pebble.Open(path, opts)
		if err != nil {
			return nil, err
		}
		return newPebbleEngine(db), nil
	}
	return nil, errors.New("unsupported kv engine")
}

// Open opens or creates DB at the given path.
func Open(path string, options *Options) (*MuxDB, error) {
	engine, err := newEngine(path, options)
	if err != nil {
		return nil, err
	}

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
		permanentTrie: options.PermanentTrie,
	}, nil
}

// NewMem creates a memory-backed DB.
func NewMem() *MuxDB {
	storage := storage.NewMemStorage()
	ldb, _ := leveldb.Open(storage, nil)

	engine := newLevelEngine(ldb, storage)
	propsStore := newNamedStore(engine, propsStoreName)
	trieLiveSpace, _ := newTrieLiveSpace(propsStore)

	return &MuxDB{
		engine:        engine,
		trieCache:     newTrieCache(0, 8192),
		trieLiveSpace: trieLiveSpace,
	}
}

// Close closes the DB.
func (db *MuxDB) Close() error {
	return db.engine.Close()
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
		db.permanentTrie,
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
		db.permanentTrie,
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
		kv.DeleteRangeFunc
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
		func(ctx context.Context, r kv.Range) (int, error) {
			return src.DeleteRange(ctx, bkt.MakeRange(r))
		},
	}
}
