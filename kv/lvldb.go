package kv

import (
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var writeOpt = &opt.WriteOptions{}
var readOpt = &opt.ReadOptions{}

// implements Batch interface
type lvldbBatch struct {
	db    *leveldb.DB
	batch *leveldb.Batch
}

func (b *lvldbBatch) Put(key, value []byte) error {
	b.batch.Put(key, value)
	return nil
}
func (b *lvldbBatch) Delete(key []byte) error {
	b.batch.Delete(key)
	return nil
}

func (b *lvldbBatch) Len() int {
	return b.batch.Len()
}
func (b *lvldbBatch) Write() error {
	return b.db.Write(b.batch, writeOpt)
}

// implements Store interface
type lvldb struct {
	db *leveldb.DB
}

func openLevelDB(stg storage.Storage, cacheSize, openFilesCacheCapacity int) (*lvldb, error) {
	if cacheSize < 128 {
		cacheSize = 128
	}

	if openFilesCacheCapacity < 64 {
		openFilesCacheCapacity = 64
	}

	db, err := leveldb.Open(stg, &opt.Options{
		OpenFilesCacheCapacity: openFilesCacheCapacity,
		BlockCacheCapacity:     cacheSize / 2 * opt.MiB,
		WriteBuffer:            cacheSize / 4 * opt.MiB, // Two of these are used internally
		Filter:                 filter.NewBloomFilter(10),
	})

	if err != nil {
		return nil, errors.Wrap(err, "open level db")
	}
	return &lvldb{db: db}, nil
}

func newMemLevelDB(cacheSize int) (*lvldb, error) {
	return openLevelDB(storage.NewMemStorage(), cacheSize, 0)
}

func newPersistentLevelDB(path string, cacheSize int, openFilesCacheCapacity int) (*lvldb, error) {
	stg, err := storage.OpenFile(path, false)
	if err != nil {
		return nil, errors.Wrap(err, "new persistent level db")
	}
	return openLevelDB(stg, cacheSize, openFilesCacheCapacity)
}

func (ldb *lvldb) Get(key []byte) (value []byte, err error) {
	return ldb.db.Get(key, readOpt)
}

func (ldb *lvldb) Has(key []byte) (bool, error) {
	return ldb.db.Has(key, readOpt)
}

func (ldb *lvldb) Put(key, value []byte) error {
	return ldb.db.Put(key, value, writeOpt)
}

func (ldb *lvldb) Delete(key []byte) error {
	return ldb.db.Delete(key, writeOpt)
}

func (ldb *lvldb) Close() error {
	return ldb.db.Close()
}

func (ldb *lvldb) NewBatch() Batch {
	return &lvldbBatch{
		ldb.db,
		&leveldb.Batch{},
	}
}

func (ldb *lvldb) NewIterator(r *Range) Iterator {
	return ldb.db.NewIterator(r.r, readOpt)
}

func (ldb *lvldb) NewKeyspace(space string) Keyspace {
	return newKeyspace(space, ldb)
}
