package cache

import (
	lru "github.com/hashicorp/golang-lru"
)

// LRU a LRU cache extends golang-lru.
type LRU struct {
	*lru.Cache
}

// NewLRU create a LRU cache instance.
func NewLRU(maxSize int) *LRU {
	if maxSize < 16 {
		maxSize = 16
	}
	cache, _ := lru.New(maxSize)
	return &LRU{cache}
}

// Loader defines loader to load value.
type Loader func(key interface{}) (interface{}, error)

// GetOrLoad first try to get from cache, do load if missed.
func (l *LRU) GetOrLoad(key interface{}, loader Loader) (interface{}, error) {
	if v, ok := l.Get(key); ok {
		return v, nil
	}
	v, err := loader(key)
	if err != nil {
		return nil, err
	}

	l.Add(key, v)
	return v, nil
}
