// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"fmt"
	"testing"
	"time"
)

// TestDiscoveryData tests the discovery subsystem and prints discovered data
// This test validates that discovery works and shows what data was found
// No assertions - only output for verification
func TestDiscoveryData(t *testing.T) {
	// Measure discovery time
	startTime := time.Now()

	// Execute discovery (this will use sync.Once internally)
	data := GetDiscoveryData()

	// Calculate discovery duration
	duration := time.Since(startTime)

	// Validate that we got some data (basic sanity check)
	if data == nil {
		t.Error("GetDiscoveryData() returned nil")
		return
	}

	// Print discovery results in a formatted way
	printDiscoveryResults(data, duration)
}

// printDiscoveryResults outputs all discovered data in a readable format
func printDiscoveryResults(data *DiscoveryData, duration time.Duration) {
	fmt.Printf("\n=== Discovery Data ===\n")
	fmt.Printf("Discovery took: %.3fs\n\n", duration.Seconds())

	// Event Address Data
	fmt.Printf("HotAddresses (%d):\n", len(data.HotAddresses))
	for i, addr := range data.HotAddresses {
		if i < 5 { // Show first 5 for brevity
			fmt.Printf("  - %s\n", truncateAddress(addr))
		}
	}
	if len(data.HotAddresses) > 5 {
		fmt.Printf("  ... (%d more)\n", len(data.HotAddresses)-5)
	}
	fmt.Println()

	fmt.Printf("MediumAddresses (%d):\n", len(data.MediumAddresses))
	for i, addr := range data.MediumAddresses {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateAddress(addr))
		}
	}
	if len(data.MediumAddresses) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.MediumAddresses)-3)
	}
	fmt.Println()

	fmt.Printf("SparseAddresses (%d):\n", len(data.SparseAddresses))
	for i, addr := range data.SparseAddresses {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateAddress(addr))
		}
	}
	if len(data.SparseAddresses) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.SparseAddresses)-3)
	}
	fmt.Println()

	// Topic Data
	fmt.Printf("HotTopics (%d):\n", len(data.HotTopics))
	for i, topic := range data.HotTopics {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateHash(topic))
		}
	}
	if len(data.HotTopics) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.HotTopics)-3)
	}
	fmt.Println()

	fmt.Printf("MediumTopics (%d):\n", len(data.MediumTopics))
	for i, topic := range data.MediumTopics {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateHash(topic))
		}
	}
	if len(data.MediumTopics) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.MediumTopics)-3)
	}
	fmt.Println()

	fmt.Printf("SparseTopics (%d):\n", len(data.SparseTopics))
	for i, topic := range data.SparseTopics {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateHash(topic))
		}
	}
	if len(data.SparseTopics) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.SparseTopics)-3)
	}
	fmt.Println()

	// Multi-Topic Patterns
	fmt.Printf("MultiTopicPatterns (%d):\n", len(data.MultiTopicPatterns))
	for i, pattern := range data.MultiTopicPatterns {
		if i < 5 { // Show first 5 patterns
			fmt.Printf("  { addr=%s", truncateAddress(pattern.Address))
			if pattern.Topic0 != "" {
				fmt.Printf(", topic0=%s", truncateHash(pattern.Topic0))
			}
			if pattern.Topic1 != "" {
				fmt.Printf(", topic1=%s", truncateHash(pattern.Topic1))
			}
			if pattern.Topic2 != "" {
				fmt.Printf(", topic2=%s", truncateHash(pattern.Topic2))
			}
			if pattern.Topic3 != "" {
				fmt.Printf(", topic3=%s", truncateHash(pattern.Topic3))
			}
			if pattern.Topic4 != "" {
				fmt.Printf(", topic4=%s", truncateHash(pattern.Topic4))
			}
			fmt.Printf(" }\n")
		}
	}
	if len(data.MultiTopicPatterns) > 5 {
		fmt.Printf("  ... (%d more)\n", len(data.MultiTopicPatterns)-5)
	}
	fmt.Println()

	// Transfer Address Data
	fmt.Printf("TransferHotAddresses (%d):\n", len(data.TransferHotAddresses))
	for i, addr := range data.TransferHotAddresses {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateAddress(addr))
		}
	}
	if len(data.TransferHotAddresses) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.TransferHotAddresses)-3)
	}
	fmt.Println()

	fmt.Printf("TransferMediumAddresses (%d):\n", len(data.TransferMediumAddresses))
	for i, addr := range data.TransferMediumAddresses {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateAddress(addr))
		}
	}
	if len(data.TransferMediumAddresses) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.TransferMediumAddresses)-3)
	}
	fmt.Println()

	fmt.Printf("TransferSparseAddresses (%d):\n", len(data.TransferSparseAddresses))
	for i, addr := range data.TransferSparseAddresses {
		if i < 3 { // Show first 3 for brevity
			fmt.Printf("  - %s\n", truncateAddress(addr))
		}
	}
	if len(data.TransferSparseAddresses) > 3 {
		fmt.Printf("  ... (%d more)\n", len(data.TransferSparseAddresses)-3)
	}
	fmt.Println()

	// Summary
	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Total discovered addresses: %d\n",
		len(data.HotAddresses)+len(data.MediumAddresses)+len(data.SparseAddresses)+
			len(data.TransferHotAddresses)+len(data.TransferMediumAddresses)+len(data.TransferSparseAddresses))
	fmt.Printf("Total discovered topics: %d\n",
		len(data.HotTopics)+len(data.MediumTopics)+len(data.SparseTopics))
	fmt.Printf("Total multi-topic patterns: %d\n", len(data.MultiTopicPatterns))
	fmt.Printf("Discovery completed successfully in %.3fs\n", duration.Seconds())
}

// truncateAddress truncates an address to show first and last few characters
func truncateAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	if len(addr) >= 42 && addr[:2] == "0x" {
		// Ethereum-style address: 0x1234...abcd
		return fmt.Sprintf("%s...%s", addr[:6], addr[len(addr)-4:])
	}
	// Generic truncation
	return fmt.Sprintf("%s...%s", addr[:6], addr[len(addr)-4:])
}

// truncateHash truncates a hash to show first few characters
func truncateHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	if len(hash) >= 66 && hash[:2] == "0x" {
		// Ethereum-style hash: 0x1234...
		return fmt.Sprintf("%s...", hash[:10])
	}
	// Generic truncation
	return fmt.Sprintf("%s...", hash[:8])
}
