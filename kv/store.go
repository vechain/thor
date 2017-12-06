package kv

import (
	"github.com/syndtr/goleveldb/leveldb"
)

// Reader wraps the read methods of kv store.
type Reader interface {
	// Get value for given key.
	// An error returned if key not found. It can be checked via IsNotFound.
	Get(key []byte) (value []byte, err error)
	Has(key []byte) (bool, error)
}

// Writer wraps the write methods of kv store.
type Writer interface {
	Put(key, value []byte) error
	Delete(key []byte) error
}

// Duplex composes reader and writer.
type Duplex interface {
	Reader
	Writer
}

// Store defines interface of kv store.
type Store interface {
	Duplex

	Close() error

	NewBatch() Batch
	NewIterator(r *Range) Iterator
}

// Batch defines batch of write ops.
type Batch interface {
	Writer
	Len() int
	Write() error
}

// Iterator to iterates kvs in store.
type Iterator interface {
	First() bool
	Last() bool
	Seek(key []byte) bool
	Next() bool
	Prev() bool

	Release()

	Error() error

	Key() []byte
	Value() []byte
}

// Options to be specified when create store instance.
type Options struct {
	CacheSize              int
	OpenFilesCacheCapacity int
}

// New create persistent store at fs path specified.
func New(path string, opts Options) (Store, error) {
	return newPersistentLevelDB(path, opts.CacheSize, opts.OpenFilesCacheCapacity)
}

// NewMem create in-memory store, for testing purpose.
func NewMem(opts Options) (Store, error) {
	return newMemLevelDB(opts.CacheSize)
}

// IsNotFound returns whether an error indicates key not found.
func IsNotFound(err error) bool {
	return err == leveldb.ErrNotFound
}
