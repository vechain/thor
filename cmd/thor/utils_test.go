// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"math"
	"testing"
)

func TestReadIntFromUInt64Flag_WithinRange(t *testing.T) {
	got, err := readIntFromUInt64Flag(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

func TestReadIntFromUInt64Flag_MaxInt(t *testing.T) {
	val := uint64(math.MaxInt)
	got, err := readIntFromUInt64Flag(val)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != int(val) {
		t.Fatalf("want %d, got %d", val, got)
	}
}

func TestReadIntFromUInt64Flag_TooLarge(t *testing.T) {
	val := uint64(math.MaxInt) + 1
	if _, err := readIntFromUInt64Flag(val); err == nil {
		t.Fatalf("expected error for value > MaxInt")
	}
}
