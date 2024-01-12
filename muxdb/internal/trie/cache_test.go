package trie

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/trie"
)

func TestNewCache(t *testing.T) {
	sizeMB := 10
	rootCap := 20
	cache := NewCache(sizeMB, rootCap)

	if cache == nil {
		t.Error("NewCache returned nil")
	}

	if cache.queriedNodes == nil || cache.committedNodes == nil || cache.roots == nil {
		t.Error("Cache components are not initialized")
	}
}

func TestCache_AddGetNodeBlob_Committing(t *testing.T) {
	cache := NewCache(10, 20)
	name := "testNode"
	seq := makeSequence(1, 1) // Updated to use makeSequence
	path := []byte("path")
	blob := []byte("blob")
	isCommitting := true

	cache.AddNodeBlob(name, seq, path, blob, isCommitting)
	retBlob := cache.GetNodeBlob(name, seq, path, false, nil)

	if !reflect.DeepEqual(blob, retBlob) {
		t.Errorf("Expected blob %v, got %v", blob, retBlob)
	}
}

func TestCache_AddGetRootNode_Success(t *testing.T) {
	cache := NewCache(10, 20)
	name := "testRoot"

	// Create a Node instance. Initialize it as per your implementation requirements.
	node := trie.NewNode()

	// Add the root node to the cache
	added := cache.AddRootNode(name, node)
	if !added {
		t.Error("Failed to add root node")
	}

	// Retrieve the root node from the cache
	retrievedNode, exists := cache.GetRootNode(name, node.SeqNum(), false)
	if !exists || !reflect.DeepEqual(node, retrievedNode) {
		t.Errorf("Failed to retrieve the added root node")
	}
}

func TestCache_GetNonExistentRootNode(t *testing.T) {
	cache := NewCache(10, 20)
	name := "testRoot"

	cache.rootStats.hit = 1999 // so that log function is hit

	// Create a Node instance. Initialize it as per your implementation requirements.
	node := trie.NewNode()

	// Add the root node to the cache
	added := cache.AddRootNode(name, node)
	if !added {
		t.Error("Failed to add root node")
	}

	// Retrieve the root node from the cache
	retrievedNode, exists := cache.GetRootNode(name, node.SeqNum(), false)
	if !exists || !reflect.DeepEqual(node, retrievedNode) {
		t.Errorf("Failed to retrieve the added root node")
	}
}

func TestCache_GetNodeBlob_NonExisting(t *testing.T) {
	cache := NewCache(10, 20)
	name := "testNode"
	seq := makeSequence(1, 1) // Updated to use makeSequence
	path := []byte("nonExistingPath")

	retBlob := cache.GetNodeBlob(name, seq, path, false, nil)
	if retBlob != nil {
		t.Errorf("Expected nil, got %v", retBlob)
	}
}

func TestCache_AddGetRootNode(t *testing.T) {
	cache := NewCache(10, 20)
	name := "testRoot"
	node := trie.Node{}

	added := cache.AddRootNode(name, node)
	if added {
		t.Error("Should not add node")
	}
}

func TestCacheStats_ShouldLog_EdgeCases(t *testing.T) {
	var stats cacheStats

	// Simulate extreme case
	for i := 0; i < 1999; i++ {
		stats.Hit()
	}
	for i := 0; i < 1999; i++ {
		stats.Miss()
	}

	logFunc, shouldLog := stats.ShouldLog("test")

	if shouldLog {
		logFunc()
	}

	// Assert or check the expected behavior
	assert.True(t, shouldLog)
}
