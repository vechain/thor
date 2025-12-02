// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"sync"
	"testing"
	"time"
)

// Test the discovery caching system
func TestDiscoveryCache(t *testing.T) {
	// Set the discovery database path to testnet for this test
	testnetPath := "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/logs-v2.db"
	discoveryDbPath = &testnetPath

	t.Logf("Testing discovery cache with database: %s", testnetPath)

	// First call - should either load from cache or perform discovery and save
	t.Log("=== First GetDiscoveryData() call ===")
	start1 := time.Now()
	data1 := GetDiscoveryData()
	duration1 := time.Since(start1)
	t.Logf("First call completed in: %v", duration1)

	// Verify we got some data
	if data1 == nil {
		t.Fatal("Got nil discovery data")
	}

	totalAddresses1 := len(data1.HotAddresses) + len(data1.MediumAddresses) + len(data1.SparseAddresses) +
		len(data1.TransferHotAddresses) + len(data1.TransferMediumAddresses) + len(data1.TransferSparseAddresses)
	totalTopics1 := len(data1.HotTopics) + len(data1.MediumTopics) + len(data1.SparseTopics)

	t.Logf("First call results: Addresses=%d, Topics=%d, Patterns=%d",
		totalAddresses1, totalTopics1, len(data1.MultiTopicPatterns))

	// Reset the sync.Once to test the second call from cache
	// Note: In a real scenario, this would be a separate program run
	discoveryOnce = sync.Once{}
	discovered = nil

	// Second call - should load from cache much faster
	t.Log("=== Second GetDiscoveryData() call ===")
	start2 := time.Now()
	data2 := GetDiscoveryData()
	duration2 := time.Since(start2)
	t.Logf("Second call completed in: %v", duration2)

	// Verify data consistency
	totalAddresses2 := len(data2.HotAddresses) + len(data2.MediumAddresses) + len(data2.SparseAddresses) +
		len(data2.TransferHotAddresses) + len(data2.TransferMediumAddresses) + len(data2.TransferSparseAddresses)
	totalTopics2 := len(data2.HotTopics) + len(data2.MediumTopics) + len(data2.SparseTopics)

	t.Logf("Second call results: Addresses=%d, Topics=%d, Patterns=%d",
		totalAddresses2, totalTopics2, len(data2.MultiTopicPatterns))

	// Verify the data is the same
	if totalAddresses1 != totalAddresses2 {
		t.Errorf("Address counts differ: %d vs %d", totalAddresses1, totalAddresses2)
	}
	if totalTopics1 != totalTopics2 {
		t.Errorf("Topic counts differ: %d vs %d", totalTopics1, totalTopics2)
	}
	if len(data1.MultiTopicPatterns) != len(data2.MultiTopicPatterns) {
		t.Errorf("Pattern counts differ: %d vs %d", len(data1.MultiTopicPatterns), len(data2.MultiTopicPatterns))
	}

	// If the second call was significantly faster, it likely used cache
	if duration2 < duration1/2 {
		t.Logf("✅ Cache appears to be working - second call was %v vs %v", duration2, duration1)
	} else {
		t.Logf("⚠️  Second call took %v vs %v - may not have used cache", duration2, duration1)
	}
}
