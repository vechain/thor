// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	lru "github.com/hashicorp/golang-lru"
)

type cache struct {
	*lru.Cache
	loader func(key interface{}) (interface{}, error)
}

func newCache(maxSize int, loader func(key interface{}) (interface{}, error)) *cache {
	c, err := lru.New(maxSize)
	if err != nil {
		panic(err)
	}
	return &cache{c, loader}
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
