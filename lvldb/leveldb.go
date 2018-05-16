package lvldb

import (
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
)

var _ kv.GetPutCloser = (*LevelDB)(nil)

// Options options for creating level db instance.
type Options struct {
	CacheSize              int
	OpenFilesCacheCapacity int
}

var writeOpt = opt.WriteOptions{}
var readOpt = opt.ReadOptions{}

// LevelDB wraps level db impls.
type LevelDB struct {
	db *leveldb.DB
}

// New create a persistent level db instance.
// Create an empty one if not exists, or open if already there.
func New(path string, opts Options) (*LevelDB, error) {
	stg, err := storage.OpenFile(path, false)
	if err != nil {
		return nil, errors.Wrap(err, "new persistent level db")
	}
	return openLevelDB(stg, opts.CacheSize, opts.OpenFilesCacheCapacity)
}

// NewMem create a level db in memory.
func NewMem() (*LevelDB, error) {
	return openLevelDB(storage.NewMemStorage(), 0, 0)
}

func openLevelDB(stg storage.Storage, cacheSize, openFilesCacheCapacity int) (*LevelDB, error) {
	if cacheSize < 16 {
		cacheSize = 16
	}

	if openFilesCacheCapacity < 16 {
		openFilesCacheCapacity = 16
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
	return &LevelDB{db: db}, nil
}

// IsNotFound to check if the error returned by Get indicates key not found.
func (ldb *LevelDB) IsNotFound(err error) bool {
	return err == leveldb.ErrNotFound
}

// Get retrieve value for given key.
// It returns an error if key not found. The error can be checked via IsNotFound.
func (ldb *LevelDB) Get(key []byte) (value []byte, err error) {
	return ldb.db.Get(key, &readOpt)
}

// Has returns whether a key exists.
func (ldb *LevelDB) Has(key []byte) (bool, error) {
	return ldb.db.Has(key, &readOpt)
}

// Put save value fo give key.
func (ldb *LevelDB) Put(key, value []byte) error {
	return ldb.db.Put(key, value, &writeOpt)
}

// Delete deletes the give key and its value.
func (ldb *LevelDB) Delete(key []byte) error {
	return ldb.db.Delete(key, &writeOpt)
}

// Close close the level db.
// Later operations will all fail.
func (ldb *LevelDB) Close() error {
	return ldb.db.Close()
}

// NewBatch create a batch for writing ops.
func (ldb *LevelDB) NewBatch() kv.Batch {
	return &levelDBBatch{
		ldb.db,
		&leveldb.Batch{},
	}
}

// NewIterator create a iterator by range.
func (ldb *LevelDB) NewIterator(r kv.Range) kv.Iterator {
	return ldb.db.NewIterator(&util.Range{
		Start: r.From,
		Limit: r.To,
	}, &readOpt)
}

//////

// levelDBBatch wraps batch operations.
type levelDBBatch struct {
	db    *leveldb.DB
	batch *leveldb.Batch
}

// Put adds a put operation.
func (b *levelDBBatch) Put(key, value []byte) error {
	b.batch.Put(key, value)
	return nil
}

// Delete adds a delete operation.
func (b *levelDBBatch) Delete(key []byte) error {
	b.batch.Delete(key)
	return nil
}

func (b *levelDBBatch) NewBatch() kv.Batch {
	return &levelDBBatch{
		b.db,
		&leveldb.Batch{},
	}
}

// Len returns ops in the batch.
func (b *levelDBBatch) Len() int {
	return b.batch.Len()
}

// Write perform all ops in this batch.
func (b *levelDBBatch) Write() error {
	return b.db.Write(b.batch, &writeOpt)
}
