// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/qianbin/directcache"
	"github.com/vechain/thor/trie"
)

// Cache is the cache layer for trie.
type Cache struct {
	// caches recently queried node blobs. Using full node key as key.
	queriedNodes *directcache.Cache
	// caches newly committed node blobs. Using node path as key.
	committedNodes *directcache.Cache
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
	cache.queriedNodes = directcache.New(sizeBytes / 4)
	cache.committedNodes = directcache.New(sizeBytes - sizeBytes/4)
	cache.roots, _ = lru.NewARC(rootCap)
	cache.lastLogTime = time.Now().UnixNano()
	return &cache
}

func (c *Cache) log() {
	now := time.Now().UnixNano()
	last := atomic.SwapInt64(&c.lastLogTime, now)

	if now-last > int64(time.Second*20) {
		log1, ok1 := c.nodeStats.ShouldLog("node cache stats")
		log2, ok2 := c.rootStats.ShouldLog("root cache stats")

		if ok1 || ok2 {
			log1()
			log2()
		}
	} else {
		atomic.CompareAndSwapInt64(&c.lastLogTime, now, last)
	}
}

// AddNodeBlob adds node blob into the cache.
func (c *Cache) AddNodeBlob(name string, seq sequence, path []byte, blob []byte, isCommitting bool) {
	if c == nil {
		return
	}
	cNum, dNum := seq.CommitNum(), seq.DistinctNum()
	k := bufferPool.Get().(*buffer)
	defer bufferPool.Put(k)

	k.buf = append(k.buf[:0], name...)
	k.buf = append(k.buf, path...)
	k.buf = appendUint32(k.buf, dNum)

	if isCommitting {
		// committing cache key: name + path + distinctNum
		v := bufferPool.Get().(*buffer)
		defer bufferPool.Put(v)

		// concat commit number with blob as cache value
		v.buf = appendUint32(v.buf[:0], cNum)
		v.buf = append(v.buf, blob...)

		_ = c.committedNodes.Set(k.buf, v.buf)
	} else {
		// querying cache key: name + path + distinctNum + commitNum
		k.buf = appendUint32(k.buf, cNum)
		_ = c.queriedNodes.Set(k.buf, blob)
	}
}

// GetNodeBlob returns the cached node blob.
func (c *Cache) GetNodeBlob(name string, seq sequence, path []byte, peek bool, dst []byte) []byte {
	if c == nil {
		return nil
	}

	cNum, dNum := seq.CommitNum(), seq.DistinctNum()
	lookupQueried := c.queriedNodes.AdvGet
	lookupCommitted := c.committedNodes.AdvGet

	k := bufferPool.Get().(*buffer)
	defer bufferPool.Put(k)

	k.buf = append(k.buf[:0], name...)
	k.buf = append(k.buf, path...)
	k.buf = appendUint32(k.buf, dNum)

	// lookup from committing cache
	var blob []byte
	if lookupCommitted(k.buf, func(b []byte) {
		if binary.BigEndian.Uint32(b) == cNum {
			blob = append(dst, b[4:]...)
		}
	}, peek) && len(blob) > 0 {
		if !peek {
			c.nodeStats.Hit()
		}
		return blob
	}

	// fallback to querying cache
	k.buf = appendUint32(k.buf, cNum)
	if lookupQueried(k.buf, func(b []byte) {
		blob = append(dst, b...)
	}, peek); len(blob) > 0 {
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
func (c *Cache) AddRootNode(name string, n trie.Node) bool {
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
	sub.Add(n.SeqNum(), n)
	return true
}

// GetRootNode returns the cached root node.
func (c *Cache) GetRootNode(name string, seq uint64, peek bool) (trie.Node, bool) {
	if c == nil {
		return trie.Node{}, false
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
		if cached, has := getByKey(seq); has {
			if !peek {
				if c.rootStats.Hit()%2000 == 0 {
					c.log()
				}
			}
			return cached.(trie.Node), true
		}
	}
	if !peek {
		c.rootStats.Miss()
	}
	return trie.Node{}, false
}

type cacheStats struct {
	hit, miss int64
	flag      int32
}

func (cs *cacheStats) Hit() int64  { return atomic.AddInt64(&cs.hit, 1) }
func (cs *cacheStats) Miss() int64 { return atomic.AddInt64(&cs.miss, 1) }

func (cs *cacheStats) ShouldLog(msg string) (func(), bool) {
	hit := atomic.LoadInt64(&cs.hit)
	miss := atomic.LoadInt64(&cs.miss)
	lookups := hit + miss

	hitrate := float64(hit) / float64(lookups)
	flag := int32(hitrate * 1000)
	return func() {
		var str string
		if lookups > 0 {
			str = fmt.Sprintf("%.3f", hitrate)
		} else {
			str = "n/a"
		}

		log.Info(msg,
			"lookups", lookups,
			"hitrate", str,
		)
		atomic.StoreInt32(&cs.flag, flag)
	}, atomic.LoadInt32(&cs.flag) != flag
}
