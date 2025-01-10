package trie

import (
	"bytes"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	// Test case parameters
	sizeMB := 100   // 100MB cache size
	rootCap := 1000 // root capacity

	// Create new cache
	cache := NewCache(sizeMB, rootCap)

	// Basic validation checks
	if cache == nil {
		t.Fatal("Expected non-nil cache")
	}

	// First log
	cache.GetRootNode("hi", 1, false)
	cache.log()
	logStats("Test", 1, 1)
}

func TestCacheLogging(t *testing.T) {
	// Create new cache
	cache := NewCache(100, 1000)

	// First log
	cache.GetRootNode("hi", 1, false)
	cache.log()
	logStats("Initial test", 1, 1)

	// Wait for 21 seconds
	time.Sleep(21 * time.Second)

	// Second log after delay
	cache.GetRootNode("hi", 2, false)
	cache.log()
	logStats("Test after delay", 1, 1)
}

func TestAddNodeBlob(t *testing.T) {
	// Create new cache
	cache := NewCache(100, 1000)
	if cache == nil {
		t.Fatal("Failed to create cache")
	}

	// Test data
	testName := "testNode"
	testPath := []byte("testPath")
	testBlob := []byte("testBlobData")

	// Test case 1: Adding blob with isCommitting = true
	cache.AddNodeBlob(testName, 1, testPath, testBlob, true)

	// Test case 2: Adding blob with isCommitting = false
	cache.AddNodeBlob(testName, 1, testPath, testBlob, false)

	// Verify the blobs were added by attempting to retrieve them
	retrievedBlob := cache.GetNodeBlob(testName, 1, testPath, false, nil)
	if retrievedBlob == nil {
		t.Error("Failed to retrieve added blob")
	}

	if !bytes.Equal(retrievedBlob, testBlob) {
		t.Errorf("Retrieved blob doesn't match original. got %v, want %v",
			retrievedBlob, testBlob)
	}
}
