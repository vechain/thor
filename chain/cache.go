// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	lru "github.com/hashicorp/golang-lru"
)

type cache struct {
	*lru.ARCCache
}

func newCache(maxSize int) *cache {
	c, _ := lru.NewARC(maxSize)
	return &cache{c}
}

// GetOrLoad returns the value associated with the key if it exists in the cache.
// Otherwise, it calls the load function to get the value and adds it to the cache.
// It returns the value, a boolean indicating whether the value was loaded, and an error if any.
func (c *cache) GetOrLoad(key interface{}, load func() (interface{}, error)) (interface{}, bool, error) {
	if value, ok := c.Get(key); ok {
		return value, false, nil
	}
	value, err := load()
	if err != nil {
		return nil, true, err
	}
	c.Add(key, value)
	return value, true, nil
}
