// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"bytes"
	"crypto/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/trie"
)

type mockedRootNode struct {
	trie.Node
	ver trie.Version
}

func (m *mockedRootNode) Version() trie.Version { return m.ver }

func TestCacheRootNode(t *testing.T) {
	cache := newCache(0, 100)

	n1 := &mockedRootNode{ver: trie.Version{Major: 1, Minor: 1}}
	cache.AddRootNode("", n1)
	assert.Equal(t, n1, cache.GetRootNode("", n1.ver))

	// minor ver not matched
	assert.Equal(t, nil, cache.GetRootNode("", trie.Version{Major: 1}))
}

func TestCacheNodeBlob(t *testing.T) {
	var (
		cache  = newCache(1, 0)
		keyBuf []byte
		blob   = []byte{1, 1, 1}
		ver    = trie.Version{Major: 1, Minor: 1}
	)

	// add to committing cache
	cache.AddNodeBlob(&keyBuf, "", nil, ver, blob, true)
	assert.Equal(t, blob, cache.GetNodeBlob(&keyBuf, "", nil, ver, false))
	// minor ver not matched
	assert.Nil(t, cache.GetNodeBlob(&keyBuf, "", nil, trie.Version{Major: 1}, false))

	cache = newCache(1, 0)

	// add to querying cache
	cache.AddNodeBlob(&keyBuf, "", nil, ver, blob, false)
	assert.Equal(t, blob, cache.GetNodeBlob(&keyBuf, "", nil, ver, false))
	// minor ver not matched
	assert.Nil(t, cache.GetNodeBlob(&keyBuf, "", nil, trie.Version{Major: 1}, false))
}

func Benchmark_cacheNodeBlob(b *testing.B) {
	var (
		cache  = newCache(100, 0)
		keyBuf []byte
		name   = "n"
		path   = []byte{1, 1}
		blob   = make([]byte, 100)
	)
	rand.Read(blob)

	for b.Loop() {
		cache.AddNodeBlob(&keyBuf, name, path, trie.Version{}, blob, true)
		got := cache.GetNodeBlob(&keyBuf, name, path, trie.Version{}, false)
		if !bytes.Equal(got, blob) {
			b.Fatalf("want %x, got %x", blob, got)
		}
	}
}

func Benchmark_cacheRootNode(b *testing.B) {
	var (
		cache = newCache(1, 0)
		name  = "n"
	)

	var tr trie.Trie
	tr.Update([]byte{1}, []byte{2}, []byte{3})

	rn := tr.RootNode()

	for b.Loop() {
		cache.AddRootNode(name, rn)
		got := cache.GetRootNode(name, trie.Version{})
		if got != rn {
			b.Fatalf("want %v, got %v", rn, got)
		}
	}
}

func TestRootNodeTTL(t *testing.T) {
	cache := newCache(1, 2) // TTL of 2

	n1 := &mockedRootNode{ver: trie.Version{Major: 1}}
	n2 := &mockedRootNode{ver: trie.Version{Major: 2}}
	n3 := &mockedRootNode{ver: trie.Version{Major: 5}} // Version difference > TTL, should cause both n1 and n2 to be evicted

	// Add first node
	cache.AddRootNode("test1", n1)
	assert.Equal(t, n1, cache.GetRootNode("test1", n1.ver))

	// Add second node
	cache.AddRootNode("test2", n2)
	assert.Equal(t, n2, cache.GetRootNode("test2", n2.ver))
	assert.Equal(t, n1, cache.GetRootNode("test1", n1.ver))

	cache.AddRootNode("test3", n3)
	assert.Equal(t, n3, cache.GetRootNode("test3", n3.ver))

	assert.Nil(t, cache.GetRootNode("test1", n1.ver), "n1 should be evicted")
	assert.Nil(t, cache.GetRootNode("test2", n2.ver), "n2 should be evicted")
}

func TestConcurrentAccess(t *testing.T) {
	cache := newCache(10, 100)
	var wg sync.WaitGroup

	// Number of concurrent goroutines
	workers := 5
	operations := 20

	// Add concurrent writers
	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			var keyBuf []byte
			for j := range operations {
				blob := []byte{byte(id), byte(j)}
				ver := trie.Version{Major: uint32(id), Minor: uint32(j)}
				cache.AddNodeBlob(&keyBuf, "test", []byte{byte(id)}, ver, blob, true)
			}
		}(i)
	}

	// Add concurrent readers
	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			var keyBuf []byte
			for j := range operations {
				ver := trie.Version{Major: uint32(id), Minor: uint32(j)}
				cache.GetNodeBlob(&keyBuf, "test", []byte{byte(id)}, ver, false)
			}
		}(i)
	}

	wg.Wait()
}

func TestCacheLogging(t *testing.T) {
	cache := newCache(1, 100)
	node := &mockedRootNode{ver: trie.Version{Major: 1}}

	// Add the root node
	cache.AddRootNode("test", node)

	// Get root node 2000 times to trigger the logging
	for range 2000 {
		result := cache.GetRootNode("test", node.ver)
		assert.NotNil(t, result)
	}

	// Get one more time after log trigger
	result := cache.GetRootNode("test", node.ver)
	assert.NotNil(t, result)

	// Test miss path
	result = cache.GetRootNode("test", trie.Version{Major: 2})
	assert.Nil(t, result)
}

func TestDummyCache(t *testing.T) {
	cache := &dummyCache{}
	var keyBuf []byte

	cache.AddNodeBlob(&keyBuf, "test", []byte{1}, trie.Version{}, []byte{1}, true)
	result := cache.GetNodeBlob(&keyBuf, "test", []byte{1}, trie.Version{}, false)
	assert.Nil(t, result)

	cache.AddRootNode("test", &mockedRootNode{})
	result2 := cache.GetRootNode("test", trie.Version{})
	assert.Nil(t, result2)
}

func TestCacheEdgeCases(t *testing.T) {
	cache := newCache(1, 100)
	var keyBuf []byte

	cache.AddNodeBlob(&keyBuf, "", nil, trie.Version{}, nil, true)
	result := cache.GetNodeBlob(&keyBuf, "", nil, trie.Version{}, false)
	assert.Nil(t, result)

	cache.AddRootNode("", nil)
	result2 := cache.GetRootNode("", trie.Version{})
	assert.Nil(t, result2)

	blob := []byte{1, 2, 3}
	ver1 := trie.Version{Major: 1, Minor: 1}
	ver2 := trie.Version{Major: 1, Minor: 2}

	cache.AddNodeBlob(&keyBuf, "test", []byte{1}, ver1, blob, true)
	result3 := cache.GetNodeBlob(&keyBuf, "test", []byte{1}, ver2, false)
	assert.Nil(t, result3)

	cache.AddNodeBlob(&keyBuf, "test", []byte{1}, ver1, blob, true)
	result4 := cache.GetNodeBlob(&keyBuf, "test", []byte{1}, ver1, true) // peek=true
	assert.NotNil(t, result4)
}

func TestCacheLogTrigger(t *testing.T) {
	cache := newCache(1, 100)
	node := &mockedRootNode{ver: trie.Version{Major: 1}}

	// Add the root node
	cache.AddRootNode("test", node)

	for range 2000 {
		result := cache.GetRootNode("test", node.ver)
		assert.NotNil(t, result)
	}

	result := cache.GetRootNode("test", node.ver)
	assert.NotNil(t, result)

	result = cache.GetRootNode("test", trie.Version{Major: 2})
	assert.Nil(t, result)
}

func TestCacheStatsFunction(t *testing.T) {
	cs := cacheStats{}

	shouldLog, hits, misses := cs.Stats()
	assert.Equal(t, int64(0), hits)
	assert.Equal(t, int64(0), misses)
	assert.False(t, shouldLog)

	cs.hit.Store(100)
	shouldLog, hits, misses = cs.Stats()
	assert.Equal(t, int64(100), hits)
	assert.Equal(t, int64(0), misses)
	assert.True(t, shouldLog)

	shouldLog, hits, misses = cs.Stats()
	assert.Equal(t, int64(100), hits)
	assert.Equal(t, int64(0), misses)
	assert.False(t, shouldLog)

	cs.miss.Store(100)
	shouldLog, hits, misses = cs.Stats()
	assert.Equal(t, int64(100), hits)
	assert.Equal(t, int64(100), misses)
	assert.True(t, shouldLog)

	cs = cacheStats{}
	cs.miss.Store(100)
	shouldLog, hits, misses = cs.Stats()
	assert.Equal(t, int64(0), hits)
	assert.Equal(t, int64(100), misses)
	assert.False(t, shouldLog)
}
