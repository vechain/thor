// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"bytes"
	"testing"
)

func TestAppendString(t *testing.T) {
	var buf []byte
	want := []byte("vechain")
	buf = vp.AppendString(buf, want)
	got, buf, err := vp.SplitString(buf)
	if err != nil {
		t.Error("should no err")
	}

	if !bytes.Equal(got, want) {
		t.Errorf("want %v got %v", want, got)
	}

	if len(buf) != 0 {
		t.Error("rest buf should be 0")
	}
}

func TestAppendUint(t *testing.T) {
	var buf []byte
	const want = 1234567
	buf = vp.AppendUint32(buf, want)
	got, buf, err := vp.SplitUint32(buf)
	if err != nil {
		t.Error("should no err")
	}
	if got != want {
		t.Errorf("want %v got %v", want, got)
	}

	if len(buf) != 0 {
		t.Error("rest buf should be 0")
	}
}
