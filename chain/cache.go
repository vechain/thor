// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/thor"
)

type cache struct {
	*lru.ARCCache
}

func newCache(maxSize int) *cache {
	c, _ := lru.NewARC(maxSize)
	return &cache{c}
}

func (c *cache) GetOrLoad(id thor.Bytes32, load func() (interface{}, error)) (interface{}, error) {
	if value, ok := c.Get(id); ok {
		return value, nil
	}
	value, err := load()
	if err != nil {
		return nil, err
	}
	c.Add(id, value)
	return value, nil
}
