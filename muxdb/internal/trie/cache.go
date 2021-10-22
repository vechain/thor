// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/coocood/freecache"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

// Cache is the cache layer for trie.
type Cache struct {
	// caches recently queried node blobs. Using full node key as key.
	queriedNodes *freecache.Cache
	// caches newly committed node blobs. Using node path as key.
	committedNodes *freecache.Cache
	// caches root nodes.
	roots       *lru.ARCCache
	nodeStats   cacheStats
	rootStats   cacheStats
	lastLogTime int64
}

// NewCache creates a cache object with the given cache size.
func NewCache(sizeMB int, rootCap int) *Cache {
	sizeBytes := sizeMB * 1024 * 1024
	var cache Cache
	cache.queriedNodes = freecache.NewCache(sizeBytes / 10)
	cache.committedNodes = freecache.NewCache(sizeBytes - sizeBytes/10)
	cache.roots, _ = lru.NewARC(rootCap)
	cache.lastLogTime = time.Now().UnixNano()
	return &cache
}

func (c *Cache) log() {
	now := time.Now().UnixNano()
	last := atomic.SwapInt64(&c.lastLogTime, now)

	if now-last > int64(time.Second*20) {
		c.nodeStats.Log("node cache stats")
		c.rootStats.Log("root cache stats")
	} else {
		atomic.CompareAndSwapInt64(&c.lastLogTime, now, last)
	}
}

// AddNodeBlob adds node blob into the cache.
func (c *Cache) AddNodeBlob(name string, key HistNodeKey, blob []byte, isCommitting bool) {
	if c == nil {
		return
	}
	k := hasherPool.Get().(*hasher)
	defer hasherPool.Put(k)
	if isCommitting {
		// concat name with path as cache key
		k.buf = append(k.buf[:0], name...)
		k.buf = append(k.buf, key.PathBlob()...)

		v := hasherPool.Get().(*hasher)
		defer hasherPool.Put(v)

		// concat commit number with blob as cache value
		v.buf = appendUint32(v.buf[:0], key.CommitNum())
		v.buf = append(v.buf, blob...)

		_ = c.committedNodes.Set(k.buf, v.buf, 0)
	} else {
		// concat name with full hist key as cache key
		k.buf = append(k.buf[:0], name...)
		k.buf = append(k.buf, key...)
		_ = c.queriedNodes.Set(k.buf, blob, 0)
	}
}

// GetNodeBlob returns the cached node blob.
func (c *Cache) GetNodeBlob(name string, key HistNodeKey, peek bool) []byte {
	if c == nil {
		return nil
	}

	lookupQueried := c.queriedNodes.Get
	lookupCommitted := c.committedNodes.Get
	if peek {
		lookupQueried = c.queriedNodes.Peek
		lookupCommitted = c.committedNodes.Peek
	}

	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	// concat name with path as cache key
	h.buf = append(h.buf[:0], name...)
	h.buf = append(h.buf, key.PathBlob()...)

	if val, _ := lookupCommitted(h.buf); len(val) > 0 {
		// compare the commit number
		if binary.BigEndian.Uint32(val) == key.CommitNum() {
			// then verify hash
			if ok, _ := verifyNodeHash(val[4:], key.Hash()); ok {
				if !peek {
					c.nodeStats.Hit()
				}
				return val[4:]
			}
		}
	}

	h.buf = append(h.buf[:len(name)], key...)
	if val, _ := lookupQueried(h.buf); len(val) > 0 {
		if !peek {
			c.nodeStats.Hit()
		}
		return val
	}
	if !peek {
		c.nodeStats.Miss()
	}
	return nil
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
func (c *Cache) GetRootNode(name string, root thor.Bytes32, commitNum uint32, peek bool) *trie.Node {
	if c == nil {
		return nil
	}

	getByName := c.roots.Get
	if peek {
		getByName = c.roots.Peek
	}

	if sub, has := getByName(name); has {
		getByKey := sub.(*lru.Cache).Get
		if peek {
			getByKey = sub.(*lru.Cache).Peek
		}
		if cached, has := getByKey(rootNodeKey{root, commitNum}); has {
			if !peek {
				if c.rootStats.Hit()%2000 == 0 {
					c.log()
				}
			}
			return cached.(*trie.Node)
		}
	}
	if !peek {
		c.rootStats.Miss()
	}
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
