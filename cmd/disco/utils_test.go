// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"math"
	"path/filepath"
	"testing"
)

func TestReadIntFromUInt64Flag_WithinRange(t *testing.T) {
	got, err := readIntFromUInt64Flag(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Fatalf("want 1, got %d", got)
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

const testEnode = "enode://f0e93c6be07f15427a017d158498c7ca9397541d24b4efbd1bb368155f6de1ae07a9a2da81a7f116e60e86c26eb0c70f2cae4516c3b5b6cfe2e5f522252665cc@54.78.133.203:55555"

func TestParseBootnodes_None(t *testing.T) {
	nodes, err := parseBootnodes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("want no nodes, got %d", len(nodes))
	}
}

func TestParseBootnodes_SkipsBlanks(t *testing.T) {
	nodes, err := parseBootnodes([]string{" " + testEnode + " ", "", "   "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if got := nodes[0].String(); got != testEnode {
		t.Fatalf("want %q, got %q", testEnode, got)
	}
}

func TestParseBootnodes_Invalid(t *testing.T) {
	if _, err := parseBootnodes([]string{testEnode, "not-an-enode"}); err == nil {
		t.Fatalf("expected error for malformed enode")
	}
}

func TestResolveNodeDBPath_Empty(t *testing.T) {
	got, err := resolveNodeDBPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("want empty path (in-memory), got %q", got)
	}
}

func TestResolveNodeDBPath_Absolute(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "nodes")
	got, err := resolveNodeDBPath(abs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != abs {
		t.Fatalf("want %q, got %q", abs, got)
	}
}

func TestResolveNodeDBPath_Relative(t *testing.T) {
	got, err := resolveNodeDBPath("nodes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("want an absolute path, got %q", got)
	}
	if filepath.Base(got) != "nodes" {
		t.Fatalf("want base 'nodes', got %q", got)
	}
}
