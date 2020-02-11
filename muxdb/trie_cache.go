// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"github.com/coocood/freecache"
	lru "github.com/hashicorp/golang-lru"
)

const (
	// encTrieNodeCacheSeg number of segments for the encoded trie node cache.
	//
	// In practical, it's more efficient if divide the cache into
	// several segments by node path length.
	// 8 is the ideal value now.
	encTrieNodeCacheSeg = 8
)

type trieCache struct {
	enc [encTrieNodeCacheSeg]*freecache.Cache // for encoded nodes
	dec *lru.Cache                            // for decoded nodes
}

func newTrieCache(encSizeMB int, decCapacity int) *trieCache {
	var cache trieCache
	if encSizeMB > 0 {
		for i := 0; i < encTrieNodeCacheSeg; i++ {
			cache.enc[i] = freecache.NewCache(encSizeMB * 1024 * 1024 / encTrieNodeCacheSeg)
		}
	}
	if decCapacity > 0 {
		cache.dec, _ = lru.New(decCapacity)
	}
	return &cache
}

func (c *trieCache) GetEncoded(key []byte, pathLen int, peek bool) (val []byte) {
	i := pathLen
	if i >= encTrieNodeCacheSeg {
		i = encTrieNodeCacheSeg - 1
	}
	if enc := c.enc[i]; enc != nil {
		if peek {
			val, _ = enc.Peek(key)
		} else {
			val, _ = enc.Get(key)
		}
		return val
	}
	return
}
func (c *trieCache) SetEncoded(key, val []byte, pathLen int) {
	i := pathLen
	if i >= encTrieNodeCacheSeg {
		i = encTrieNodeCacheSeg - 1
	}
	if enc := c.enc[i]; enc != nil {
		_ = enc.Set(key, val, 8*3600)
	}
}

func (c *trieCache) GetDecoded(key []byte, peek bool) (val interface{}) {
	if c.dec == nil {
		return nil
	}
	if peek {
		val, _ = c.dec.Peek(string(key))
	} else {
		val, _ = c.dec.Get(string(key))
	}
	return val
}

func (c *trieCache) SetDecoded(key []byte, val interface{}) {
	if c.dec == nil {
		return
	}
	c.dec.Add(string(key), val)
}
