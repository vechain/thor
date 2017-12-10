package cache

import lru "github.com/hashicorp/golang-lru"

// LRU a LRU cache extends golang-lru.
type LRU struct {
	*lru.Cache
}

// NewLRU create a LRU cache instance.
// maxSize should be > 0, or an error returned.
func NewLRU(maxSize int) (*LRU, error) {
	cache, err := lru.New(maxSize)
	if err != nil {
		return nil, err
	}
	return &LRU{cache}, nil
}

// GetOrLoad first try to get from cache, do load if missed.
func (l *LRU) GetOrLoad(key interface{}, loader func() interface{}) interface{} {
	if v, ok := l.Get(key); ok {
		return v
	}
	if v := loader(); v != nil {
		l.Add(key, v)
		return v
	}
	return nil
}
