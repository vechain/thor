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

func (c *cache) GetOrLoad(key any, load func() (any, error)) (any, bool, error) {
	if value, ok := c.Get(key); ok {
		return value, true, nil
	}
	value, err := load()
	if err != nil {
		return nil, false, err
	}
	c.Add(key, value)
	return value, false, nil
}
