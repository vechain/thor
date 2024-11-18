// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qianbin/directcache"
	"github.com/vechain/thor/v2/trie"
)

// cache is the cache layer for trie.
type cache struct {
	queriedNodes   *directcache.Cache // caches recently queried node blobs.
	committedNodes *directcache.Cache // caches newly committed node blobs.
	roots          struct {           // caches root nodes.
		m        map[string]trie.Node
		lock     sync.RWMutex
		maxMajor uint32
		ttl      uint32
	}

	nodeStats   cacheStats
	rootStats   cacheStats
	lastLogTime atomic.Int64
}

// newCache creates a cache object with the given cache size.
func newCache(sizeMB int, rootTTL uint32) *cache {
	sizeBytes := sizeMB * 1024 * 1024
	cache := &cache{
		queriedNodes:   directcache.New(sizeBytes / 4),
		committedNodes: directcache.New(sizeBytes - sizeBytes/4),
	}
	cache.lastLogTime.Store(time.Now().UnixNano())
	cache.roots.m = make(map[string]trie.Node)
	cache.roots.ttl = rootTTL
	return cache
}

func (c *cache) log() {
	now := time.Now().UnixNano()
	last := c.lastLogTime.Swap(now)

	if now-last > int64(time.Second*20) {
		c.nodeStats.Log("node cache stats")
		c.rootStats.Log("root cache stats")
	} else {
		c.lastLogTime.CompareAndSwap(now, last)
	}
}

// AddNodeBlob adds encoded node blob into the cache.
func (c *cache) AddNodeBlob(keyBuf *[]byte, name string, path []byte, ver trie.Version, blob []byte, isCommitting bool) {
	if c == nil {
		return
	}

	// the version part
	v := binary.AppendUvarint((*keyBuf)[:0], uint64(ver.Major))
	v = binary.AppendUvarint(v, uint64(ver.Minor))
	// the full key
	k := append(v, name...)
	k = append(k, path...)
	*keyBuf = k

	if isCommitting {
		_ = c.committedNodes.AdvSet(k[len(v):], len(blob)+len(v), func(val []byte) {
			copy(val, v)
			copy(val[len(v):], blob)
		})
	} else {
		_ = c.queriedNodes.Set(k, blob)
	}
}

// GetNodeBlob returns the cached node blob.
func (c *cache) GetNodeBlob(keyBuf *[]byte, name string, path []byte, ver trie.Version, peek bool) []byte {
	if c == nil {
		return nil
	}
	// the version part
	v := binary.AppendUvarint((*keyBuf)[:0], uint64(ver.Major))
	v = binary.AppendUvarint(v, uint64(ver.Minor))
	// the full key
	k := append(v, name...)
	k = append(k, path...)
	*keyBuf = k

	var blob []byte
	// lookup from committing cache
	if c.committedNodes.AdvGet(k[len(v):], func(val []byte) {
		if bytes.Equal(k[:len(v)], val[:len(v)]) {
			blob = append([]byte(nil), val[len(v):]...)
		}
	}, peek) && len(blob) > 0 {
		if !peek {
			c.nodeStats.Hit()
		}
		return blob
	}

	// fallback to querying cache
	if c.queriedNodes.AdvGet(k, func(val []byte) {
		blob = append([]byte(nil), val...)
	}, peek) && len(blob) > 0 {
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
func (c *cache) AddRootNode(name string, n trie.Node) {
	if c == nil || n == nil {
		return
	}
	c.roots.lock.Lock()
	defer c.roots.lock.Unlock()

	major := n.Version().Major
	if major > c.roots.maxMajor {
		c.roots.maxMajor = major
		// evict old root nodes
		for k, r := range c.roots.m {
			if major-r.Version().Major > c.roots.ttl {
				delete(c.roots.m, k)
			}
		}
	}
	c.roots.m[name] = n
}

// GetRootNode returns the cached root node.
func (c *cache) GetRootNode(name string, ver trie.Version) trie.Node {
	if c == nil {
		return nil
	}
	c.roots.lock.RLock()
	defer c.roots.lock.RUnlock()

	if r, has := c.roots.m[name]; has {
		if r.Version() == ver {
			if c.rootStats.Hit()%2000 == 0 {
				c.log()
			}
			return r
		}
	}
	c.rootStats.Miss()
	return nil
}

type cacheStats struct {
	hit, miss atomic.Int64
	flag      atomic.Int32
}

func (cs *cacheStats) Hit() int64  { return cs.hit.Add(1) }
func (cs *cacheStats) Miss() int64 { return cs.miss.Add(1) }

func (cs *cacheStats) Log(msg string) {
	hit := cs.hit.Load()
	miss := cs.miss.Load()
	lookups := hit + miss

	// Calculate hitrate
	hitrate := float64(hit) / float64(lookups)
	flag := int32(hitrate * 1000)

	// Log the current state unless the flag is unchanged
	if cs.flag.Load() == flag {
		return
	}

	// Log cache stats
	var str string
	if lookups > 0 {
		str = fmt.Sprintf("%.3f", hitrate)
	} else {
		str = "n/a"
	}

	logger.Info(msg,
		"lookups", lookups,
		"hitrate", str,
	)

	// Update the flag
	cs.flag.Store(flag)
}
