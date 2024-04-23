package thor

import "testing"

func TestBlocklistInitialization(t *testing.T) {
	knownBlocked := []string{
		"0x4427be8010dd870395975a8fbaa7afa9439b5332",
		"0xa99bd128a454728b509ab0e41499a6d1ea0d3416",
	}

	for _, addrStr := range knownBlocked {
		addr := MustParseAddress(addrStr)
		if !IsOriginBlocked(addr) {
			t.Errorf("Address %s is expected to be blocked, but IsOriginBlocked returned false", addrStr)
		}
	}

	// Address known not to be in the blocklist
	knownUnblocked := "0x000000000000000000000000000000000000dead"
	addr := MustParseAddress(knownUnblocked)
	if IsOriginBlocked(addr) {
		t.Errorf("Address %s is expected to be unblocked, but IsOriginBlocked returned true", knownUnblocked)
	}
}

func TestMockBlocklist(t *testing.T) {
	// Setup mock blocklist with a new set of addresses
	mockAddresses := []string{
		"0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"0xfeedbeeffeedbeeffeedbeeffeedbeeffeedbeef",
	}
	MockBlocklist(mockAddresses)

	// Test to ensure the mock blocklist is now in effect
	tests := []struct {
		address string
		blocked bool
	}{
		{mockAddresses[0], true},
		{mockAddresses[1], true},
		// An address known to be in the original blocklist, expecting it to be unblocked after mocking
		{"0x4427be8010dd870395975a8fbaa7afa9439b5332", false},
	}

	for _, tt := range tests {
		addr := MustParseAddress(tt.address)
		if IsOriginBlocked(addr) != tt.blocked {
			t.Errorf("MockBlocklist failed for %v: expected blocked=%v, got blocked=%v", tt.address, tt.blocked, !tt.blocked)
		}
	}
}
