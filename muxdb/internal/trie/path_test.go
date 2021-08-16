// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"testing"
)

func Test_packPath(t *testing.T) {
	tests := []struct {
		path []byte
		want path64
	}{
		{nil, 0},
		{[]byte{2}, 0x2000000000000001},
		{[]byte{0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf}, 0xffffffffffffffff},
		{[]byte{0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf, 0xf}, 0xffffffffffffffff},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := newPath64(tt.path); got != tt.want {
				t.Errorf("packPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_path64_Append(t *testing.T) {

	tests := []struct {
		p    path64
		e    byte
		want path64
	}{
		{0, 2, 0x2000000000000001},
		{0x2000000000000001, 3, 0x2300000000000002},
		{0xffffffffffffffff, 2, 0xffffffffffffffff},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := tt.p.Append(tt.e); got != tt.want {
				t.Errorf("path64.Append() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_path64_Len(t *testing.T) {
	tests := []struct {
		p    path64
		want int
	}{
		{0, 0},
		{0xffffffffffffffff, 15},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := tt.p.Len(); got != tt.want {
				t.Errorf("path64.Len() = %v, want %v", got, tt.want)
			}
		})
	}
}
