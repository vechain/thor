// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"bytes"
	"crypto/rand"
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

	for i := 0; i < b.N; i++ {
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

	for i := 0; i < b.N; i++ {
		cache.AddRootNode(name, rn)
		got := cache.GetRootNode(name, trie.Version{})
		if got != rn {
			b.Fatalf("want %v, got %v", rn, got)
		}
	}
}
