// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"testing"
)

// TestLoadCorrectReceiptsRootsForKeyValue tests a specific key/value pair in the map returned by LoadCorrectReceiptsRoots
func TestLoadCorrectReceiptsRootsForKeyValue(t *testing.T) {
	// Define the key/value pair you expect to find
	expectedKey := "0x000c2c63d845188f8390de84d1364b59cd2890f276452762f9ab0761ecc93069"
	expectedValue := "0xe467857991d0e04da9b2e0365110b49f198febf9d3fa8413c536527f857bd246"

	// Load the map
	actualMap := LoadCorrectReceiptsRoots()

	// Check if the key exists
	actualValue, exists := actualMap[expectedKey]
	if !exists {
		t.Fatalf("Expected key %s not found in the map", expectedKey)
	}

	// Check if the value matches
	if expectedValue != actualValue {
		t.Errorf("For key %s, expected value %s, got %s", expectedKey, expectedValue, actualValue)
	}
}
