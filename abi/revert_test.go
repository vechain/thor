package abi

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestUnpackRevert(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		input     string
		expect    string
		expectErr error
	}{
		{"", "", errors.New("invalid data for unpacking")},
		{"08c379a1", "", errors.New("invalid data for unpacking")},
		{"08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000d72657665727420726561736f6e00000000000000000000000000000000000000", "revert reason", nil},
		{"4e487b710000000000000000000000000000000000000000000000000000000000000000", "generic panic", nil},
		{"4e487b7100000000000000000000000000000000000000000000000000000000000000ff", "unknown panic code: 0xff", nil},
	}
	for index, c := range cases {
		t.Run(fmt.Sprintf("case %d", index), func(t *testing.T) {
			t.Parallel()
			got, err := UnpackRevert(common.Hex2Bytes(c.input))
			if c.expectErr != nil {
				if err == nil {
					t.Fatalf("Expected non-nil error")
				}
				if err.Error() != c.expectErr.Error() {
					t.Fatalf("Expected error mismatch, want %v, got %v", c.expectErr, err)
				}
				return
			}
			if c.expect != got {
				t.Fatalf("Output mismatch, want %v, got %v", c.expect, got)
			}
		})
	}
}
