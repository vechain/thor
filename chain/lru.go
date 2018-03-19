package chain

import cache "github.com/hashicorp/golang-lru/simplelru"

type lru struct {
	*cache.LRU
	loader func(key interface{}) (interface{}, error)
}

func newLRU(maxSize int, loader func(key interface{}) (interface{}, error)) *lru {
	cache, err := cache.NewLRU(maxSize, nil)
	if err != nil {
		panic(err)
	}
	return &lru{cache, loader}
}

func (l *lru) GetOrLoad(key interface{}) (interface{}, error) {
	if value, ok := l.Get(key); ok {
		return value, nil
	}
	value, err := l.loader(key)
	if err != nil {
		return nil, err
	}
	l.Add(key, value)
	return value, nil
}
