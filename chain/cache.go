package chain

import lru "github.com/hashicorp/golang-lru"

type cache struct {
	*lru.ARCCache
	loader func(key interface{}) (interface{}, error)
}

func newLRU(maxSize int, loader func(key interface{}) (interface{}, error)) *cache {
	arc, err := lru.NewARC(maxSize)
	if err != nil {
		panic(err)
	}
	return &cache{arc, loader}
}

func (c *cache) GetOrLoad(key interface{}) (interface{}, error) {
	if value, ok := c.Get(key); ok {
		return value, nil
	}
	value, err := c.loader(key)
	if err != nil {
		return nil, err
	}
	c.Add(key, value)
	return value, nil
}
