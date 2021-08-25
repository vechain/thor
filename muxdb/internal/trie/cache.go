// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"github.com/coocood/freecache"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

// Cache is the cache layer for trie.
type Cache struct {
	nodes  [7]*freecache.Cache // slots to cache node blobs.
	pfkeys *freecache.Cache    // caches prefilter keys.
	roots  *lru.Cache          // caches root nodes.
}

// NewCache creates a cache object with the given cache size.
func NewCache(sizeMB int, rootCap int) *Cache {
	var cache Cache
	if sizeMB > 0 {
		nSlot := len(cache.nodes) + 1
		size := sizeMB * 1024 * 1024 / nSlot
		for i := 0; i < len(cache.nodes); i++ {
			cache.nodes[i] = freecache.NewCache(size)
		}
		cache.pfkeys = freecache.NewCache(size)
	}
	if rootCap > 0 {
		cache.roots, _ = lru.New(rootCap)
	}
	return &cache
}

func (c *Cache) nodeSlot(pathLen int) *freecache.Cache {
	if n := len(c.nodes); pathLen >= n {
		pathLen = n - 1
	}
	return c.nodes[pathLen]
}

// AddNodeBlob adds node into the cache.
func (c *Cache) AddNodeBlob(key, node []byte, pathLen int) {
	if s := c.nodeSlot(pathLen); s != nil {
		_ = s.Set(key, node, 0)
	}
}

// GetNodeBlob returns the cached node.
func (c *Cache) GetNodeBlob(key []byte, pathLen int, peek bool) (node []byte) {
	if s := c.nodeSlot(pathLen); s != nil {
		f := s.Get
		if peek {
			f = s.Peek
		}
		node, _ = f(key)
	}
	return
}

// AddPrefilterKey add prefilter key into the cache.
func (c *Cache) AddPrefilterKey(key []byte) {
	if s := c.pfkeys; s != nil {
		s.Set(key, nil, 0)
	}
}

// HasPrefilterKey check if the given key is in the cache.
func (c *Cache) HasPrefilterKey(key []byte) bool {
	if s := c.pfkeys; s != nil {
		_, err := s.Get(key)
		return err == nil
	}
	return false
}

type rootNodeKey struct {
	name      string
	root      thor.Bytes32
	commitNum uint32
}

// AddRootNode add the root node into the cache
func (c *Cache) AddRootNode(name string, root thor.Bytes32, commitNum uint32, n *trie.Node) {
	if r := c.roots; r != nil {
		r.Add(rootNodeKey{name, root, commitNum}, n)
	}
}

// GetRootNode returns the cached root node.
func (c *Cache) GetRootNode(name string, root thor.Bytes32, commitNum uint32) *trie.Node {
	if r := c.roots; r != nil {
		if cached, has := r.Get(rootNodeKey{name, root, commitNum}); has {
			return cached.(*trie.Node)
		}
	}
	return nil
}
