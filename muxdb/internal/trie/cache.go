// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coocood/freecache"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

// Cache is the cache layer for trie.
type Cache struct {
	nodes       *freecache.Cache // caches node blobs.
	roots       *lru.ARCCache    // caches root nodes.
	nodeStats   cacheStats
	rootStats   cacheStats
	lastLogTime int64
	logMu       sync.Mutex
}

// NewCache creates a cache object with the given cache size.
func NewCache(sizeMB int, rootCap int) *Cache {
	var cache Cache
	cache.nodes = freecache.NewCache(sizeMB * 1024 * 1024)
	cache.roots, _ = lru.NewARC(rootCap)
	cache.lastLogTime = time.Now().UnixNano()
	return &cache
}

func (c *Cache) log() {
	c.logMu.Lock()
	defer c.logMu.Unlock()

	if now := time.Now().UnixNano(); now-c.lastLogTime > int64(time.Second*20) {
		c.lastLogTime = now
		c.nodeStats.Log("node cache stats")
		c.rootStats.Log("root cache stats")
	}
}

// AddNodeBlob adds node blob into the cache.
func (c *Cache) AddNodeBlob(name string, key HistNodeKey, node []byte) {
	if c == nil {
		return
	}

	k := bufferPool.Get().(*buffer)
	defer bufferPool.Put(k)

	// concat name with path as cache key
	k.b = append(k.b[:0], name...)
	k.b = append(k.b, key.PathBlob()...)

	v := bufferPool.Get().(*buffer)
	defer bufferPool.Put(v)

	// concat hash pointer with blob as cache value
	v.b = append(v.b[:0], key.HashPointer()...)
	v.b = append(v.b, node...)

	_ = c.nodes.Set(k.b, v.b, 0)
}

// GetNodeBlob returns the cached node blob.
func (c *Cache) GetNodeBlob(name string, key HistNodeKey, peek bool) (node []byte) {
	if c == nil {
		return
	}

	get := c.nodes.Get
	if peek {
		get = c.nodes.Peek
	}

	buf := bufferPool.Get().(*buffer)
	defer bufferPool.Put(buf)

	// concat name with path as cache key
	buf.b = append(buf.b[:0], name...)
	buf.b = append(buf.b, key.PathBlob()...)

	if val, _ := get(buf.b); len(val) > 0 {
		// compare the hash pointer
		hp := key.HashPointer()
		if bytes.Equal(hp, val[:len(hp)]) {
			if !peek {
				c.nodeStats.Hit()
			}
			return val[len(hp):]
		}
	}
	if !peek {
		c.nodeStats.Miss()
	}
	return
}

type rootNodeKey struct {
	root      thor.Bytes32
	commitNum uint32
}

// AddRootNode add the root node into the cache.
func (c *Cache) AddRootNode(name string, n *trie.Node) bool {
	if c == nil {
		return false
	}
	if n.Dirty() {
		return false
	}
	var sub *lru.Cache
	if q, has := c.roots.Get(name); has {
		sub = q.(*lru.Cache)
	} else {
		sub, _ = lru.New(4)
		c.roots.Add(name, sub)
	}
	sub.Add(rootNodeKey{n.Hash(), n.CommitNum()}, n)
	return true
}

// GetRootNode returns the cached root node.
func (c *Cache) GetRootNode(name string, root thor.Bytes32, commitNum uint32) *trie.Node {
	if c == nil {
		return nil
	}

	if sub, has := c.roots.Get(name); has {
		if cached, has := sub.(*lru.Cache).Get(rootNodeKey{root, commitNum}); has {
			if c.rootStats.Hit()%1000 == 0 {
				c.log()
			}
			return cached.(*trie.Node)
		}
	}
	c.rootStats.Miss()
	return nil
}

type cacheStats struct {
	hit, miss int64
}

func (cs *cacheStats) Hit() int64  { return atomic.AddInt64(&cs.hit, 1) }
func (cs *cacheStats) Miss() int64 { return atomic.AddInt64(&cs.miss, 1) }

func (cs *cacheStats) Log(msg string) {
	hit := atomic.LoadInt64(&cs.hit)
	miss := atomic.LoadInt64(&cs.miss)
	lookups := hit + miss

	var hitrate string
	if lookups > 0 {
		hitrate = fmt.Sprintf("%.3f", float64(hit)/float64(lookups))
	} else {
		hitrate = "n/a"
	}

	log.Info(msg,
		"lookups", lookups,
		"hitrate", hitrate,
	)
}
