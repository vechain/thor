package thor

import (
	"math"
	"testing"
)

// TestForkConfigString verifies that the String method returns expected values.
func TestForkConfigString(t *testing.T) {
	fc := ForkConfig{
		VIP191:    1,
		ETH_CONST: math.MaxUint32,
		BLOCKLIST: 2,
		ETH_IST:   math.MaxUint32,
		VIP214:    math.MaxUint32,
		FINALITY:  math.MaxUint32,
		ETH_SH:    math.MaxUint32,
	}

	expectedStr := "VIP191: #1, BLOCKLIST: #2"
	if fc.String() != expectedStr {
		t.Errorf("ForkConfig.String() = %v, want %v", fc.String(), expectedStr)
	}
}

// TestNoFork verifies the NoFork variable is correctly set up.
func TestNoFork(t *testing.T) {
	if NoFork.VIP191 != math.MaxUint32 || NoFork.BLOCKLIST != math.MaxUint32 {
		t.Errorf("NoFork does not correctly represent a configuration with no forks")
	}
}

// TestGetForkConfig checks retrieval of fork configurations for known genesis IDs.
func TestGetForkConfig(t *testing.T) {
	// You'll need to adjust these based on the actual genesis IDs and expected configurations
	mainnetID := MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a")
	testnetID := MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127")
	unknownID := MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	tests := []struct {
		id          Bytes32
		expectFound bool
	}{
		{mainnetID, true},
		{testnetID, true},
		{unknownID, false}, // Expect no config for unknown ID
	}

	for _, tt := range tests {
		config := GetForkConfig(tt.id)
		if (config != ForkConfig{}) != tt.expectFound {
			t.Errorf("GetForkConfig(%v) found = %v, want %v", tt.id, !tt.expectFound, tt.expectFound)
		}
	}
}
