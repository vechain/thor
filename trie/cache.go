package trie

import (
	"sync/atomic"
	"time"

	"github.com/allegro/bigcache"
)

// Cache the frontend cache for trie database.
type Cache struct {
	bigcache *bigcache.BigCache
}

// NewCache create database cache.
func NewCache(maxSizeMB int) *Cache {
	bigcache, _ := bigcache.NewBigCache(bigcache.Config{
		Shards:             1024,
		LifeWindow:         time.Hour,
		MaxEntriesInWindow: maxSizeMB * 1024,
		MaxEntrySize:       512,
		HardMaxCacheSize:   maxSizeMB,
	})
	return &Cache{bigcache}
}

// Serve serve the given database and return the cached one.
func (c *Cache) Serve(db Database) Database {
	return &cachedDatabase{db, c.bigcache}
}

// ServeWriter serve the given database writer and return the cached one.
func (c *Cache) ServeWriter(db DatabaseWriter) DatabaseWriter {
	return &cachedDatabaseWriter{db, c.bigcache}
}

type cachedDatabase struct {
	db       Database
	bigcache *bigcache.BigCache
}

func (cdb *cachedDatabase) Get(key []byte) ([]byte, error) {
	value, err := cdb.bigcache.Get(string(key))
	if err == nil {
		return value, nil
	}

	value, err = cdb.db.Get(key)
	if err != nil {
		return nil, err
	}
	cdb.bigcache.Set(string(key), value)
	return value, nil
}

func (cdb *cachedDatabase) Has(key []byte) (bool, error) {
	return cdb.db.Has(key)
}

func (cdb *cachedDatabase) Put(key, value []byte) error {
	if err := cdb.db.Put(key, value); err != nil {
		return err
	}

	cdb.bigcache.Set(string(key), value)
	return nil
}

type cachedDatabaseWriter struct {
	db       DatabaseWriter
	bigcache *bigcache.BigCache
}

func (cdb *cachedDatabaseWriter) Put(key, value []byte) error {
	if err := cdb.db.Put(key, value); err != nil {
		return err
	}

	cdb.bigcache.Set(string(key), value)
	return nil
}

var cacheValue atomic.Value

// SetCache set trie cache.
func SetCache(c *Cache) {
	cacheValue.Store(c)
}

func cacheDatabase(db Database) Database {
	if db != nil {
		if v := cacheValue.Load(); v != nil {
			return v.(*Cache).Serve(db)
		}
	}
	return db
}

func cacheDatabaseWriter(db DatabaseWriter) DatabaseWriter {
	if db != nil {
		if v := cacheValue.Load(); v != nil {
			return v.(*Cache).ServeWriter(db)
		}
	}
	return db
}
