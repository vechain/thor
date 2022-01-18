// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"context"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	dberrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb/internal/engine"
	"github.com/vechain/thor/muxdb/internal/trie"
	"github.com/vechain/thor/thor"
)

const (
	trieHistSpace     = byte(0) // the key space for historical trie nodes.
	trieDedupedSpace  = byte(1) // the key space for deduped trie nodes.
	trieLeafBankSpace = byte(2) // the key space for the trie leaf bank.
	namedStoreSpace   = byte(3) // the key space for named store.
)

// Trie is the managed trie.
type Trie = trie.Trie
type TriePartitionFactor = trie.PartitionFactor

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
	// TrieHistPartitionFactor is the partition factor for historical trie nodes.
	TrieHistPartitionFactor trie.PartitionFactor
	// TrieDedupedPartitionFactor is the partition factor for deduped trie nodes.
	TrieDedupedPartitionFactor trie.PartitionFactor
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
	trieBackend *trie.Backend
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
		NoFullFSync:            true,
	}

	if options.TrieWillCleanHistory {
		// this option gets disk space efficiently reclaimed.
		// only set when pruner enabled.
		ldbOpts.OverflowPrefix = []byte{trieHistSpace}
	}

	// open leveldb
	ldb, err := leveldb.OpenFile(path, &ldbOpts)
	if _, corrupted := err.(*dberrors.ErrCorrupted); corrupted {
		ldb, err = leveldb.RecoverFile(path, &ldbOpts)
	}
	if err != nil {
		return nil, err
	}

	// as engine
	engine := engine.NewLevelEngine(ldb)

	var ptnConfig triePartitionConfig
	if err := ptnConfig.Load(engine); err != nil {
		if !engine.IsNotFound(err) {
			ldb.Close()
			return nil, err
		}

		// not configured. save in db to avoid trie corruption
		// when partition factor occasionally tweaked.
		ptnConfig.Hist = options.TrieHistPartitionFactor
		ptnConfig.Deduped = options.TrieDedupedPartitionFactor

		if err := ptnConfig.Save(engine); err != nil {
			ldb.Close()
			return nil, err
		}
	}

	trieCache := trie.NewCache(
		options.TrieNodeCacheSizeMB,
		options.TrieRootCacheCapacity)

	trieLeafBank := trie.NewLeafBank(
		engine,
		trieLeafBankSpace,
		options.TrieLeafBankSlotCapacity)

	return &MuxDB{
		engine: engine,
		trieBackend: &trie.Backend{
			Store:            engine,
			Cache:            trieCache,
			LeafBank:         trieLeafBank,
			HistSpace:        trieHistSpace,
			DedupedSpace:     trieDedupedSpace,
			HistPtnFactor:    ptnConfig.Hist,
			DedupedPtnFactor: ptnConfig.Deduped,
			CachedNodeTTL:    options.TrieCachedNodeTTL,
		},
	}, nil
}

// NewMem creates a memory-backed DB.
func NewMem() *MuxDB {
	storage := storage.NewMemStorage()
	ldb, _ := leveldb.Open(storage, nil)

	engine := engine.NewLevelEngine(ldb)
	return &MuxDB{
		engine: engine,
		trieBackend: &trie.Backend{
			Store:            engine,
			Cache:            nil,
			LeafBank:         nil,
			HistSpace:        trieHistSpace,
			DedupedSpace:     trieDedupedSpace,
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

func (db *MuxDB) TrieHistPartitionFactor() TriePartitionFactor {
	return db.trieBackend.HistPtnFactor
}

func (db *MuxDB) TrieDedupedPartitionFactor() TriePartitionFactor {
	return db.trieBackend.DedupedPtnFactor
}

// NewTrie creates trie either with existing root node.
//
// If root is zero or blake2b hash of an empty string, the trie is
// initially empty.
func (db *MuxDB) NewTrie(name string, root thor.Bytes32, commitNum, distinctNum uint32) *Trie {
	return trie.New(
		db.trieBackend,
		name,
		root,
		commitNum,
		distinctNum,
	)
}

// CleanTrieHistory clean trie history within [startCommitNum, limitCommitNum).
func (db *MuxDB) CleanTrieHistory(ctx context.Context, startCommitNum, limitCommitNum uint32) error {
	return trie.CleanHistory(ctx, db.trieBackend, startCommitNum, limitCommitNum)
}

// NewStore creates named kv-store.
func (db *MuxDB) NewStore(name string) kv.Store {
	return kv.Bucket(string(namedStoreSpace) + name).NewStore(db.engine)
}

// IsNotFound returns if the error indicates key not found.
func (db *MuxDB) IsNotFound(err error) bool {
	return db.engine.IsNotFound(err)
}

const (
	configStoreName  = "muxdb.config"
	triePtnConfigKey = "triePtn"
)

type triePartitionConfig struct {
	Hist    trie.PartitionFactor
	Deduped trie.PartitionFactor
}

func (c *triePartitionConfig) Load(getter kv.Getter) error {
	getter = kv.Bucket(string(namedStoreSpace) + configStoreName).NewGetter(getter)
	data, err := getter.Get([]byte(triePtnConfigKey))
	if err != nil {
		return err
	}
	return rlp.DecodeBytes(data, c)
}

func (c *triePartitionConfig) Save(putter kv.Putter) error {
	data, err := rlp.EncodeToBytes(c)
	if err != nil {
		return err
	}
	putter = kv.Bucket(string(namedStoreSpace) + configStoreName).NewPutter(putter)
	return putter.Put([]byte(triePtnConfigKey), data)
}
