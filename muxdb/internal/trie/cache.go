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
	cache.queriedNodes = freecache.NewCache(sizeBytes / 4)
	cache.committedNodes = freecache.NewCache(sizeBytes - sizeBytes/4)
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
func (c *Cache) AddNodeBlob(name string, commitNum, distinctNum uint32, path []byte, blob []byte, isCommitting bool) {
	if c == nil {
		return
	}
	k := hasherPool.Get().(*hasher)
	defer hasherPool.Put(k)

	k.buf = append(k.buf[:0], name...)
	k.buf = append(k.buf, path...)

	if isCommitting {
		// committing cache key: name + path
		v := hasherPool.Get().(*hasher)
		defer hasherPool.Put(v)

		// concat commit & distinct number with blob as cache value
		v.buf = appendUint32(v.buf[:0], commitNum)
		v.buf = appendUint32(v.buf, distinctNum)
		v.buf = append(v.buf, blob...)

		_ = c.committedNodes.Set(k.buf, v.buf, 0)
	} else {
		// querying cache key: name + path + commitNum + distinctNum
		k.buf = appendUint32(k.buf, commitNum)
		k.buf = appendUint32(k.buf, distinctNum)
		_ = c.queriedNodes.Set(k.buf, blob, 0)
	}
}

// GetNodeBlob returns the cached node blob.
func (c *Cache) GetNodeBlob(name string, commitNum, distinctNum uint32, path []byte, peek bool, dst []byte) []byte {
	if c == nil {
		return nil
	}

	lookupQueried := c.queriedNodes.GetFn
	lookupCommitted := c.committedNodes.GetFn
	if peek {
		lookupQueried = c.queriedNodes.PeekFn
		lookupCommitted = c.committedNodes.PeekFn
	}

	k := hasherPool.Get().(*hasher)
	defer hasherPool.Put(k)

	k.buf = append(k.buf[:0], name...)
	k.buf = append(k.buf, path...)

	// lookup from committing cache
	var blob []byte
	lookupCommitted(k.buf, func(b []byte) error {
		if binary.BigEndian.Uint64(b) == (uint64(commitNum)<<32)|uint64(distinctNum) {
			blob = append(dst, b[8:]...)
		}
		return nil
	})
	if len(blob) > 0 {
		if !peek {
			c.nodeStats.Hit()
		}
		return blob
	}

	// fallback to querying cache
	k.buf = appendUint32(k.buf, commitNum)
	k.buf = appendUint32(k.buf, distinctNum)
	lookupQueried(k.buf, func(b []byte) error {
		blob = append(dst, b...)
		return nil
	})
	if len(blob) > 0 {
		if !peek {
			c.nodeStats.Hit()
		}
		return blob
	}
	if !peek {
		c.nodeStats.Miss()
	}
	return nil
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
	key := (uint64(n.CommitNum()) << 32) | uint64(n.DistinctNum())
	sub.Add(key, n)
	return true
}

// GetRootNode returns the cached root node.
func (c *Cache) GetRootNode(name string, commitNum, distinctNum uint32, peek bool) *trie.Node {
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
		key := (uint64(commitNum) << 32) | uint64(distinctNum)
		if cached, has := getByKey(key); has {
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
