// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"testing"
)

func TestPartitionFactor_Range(t *testing.T) {
	const factor = PartitionFactor(10)

	tests := []struct {
		pid   uint32
		want  uint32
		want1 uint32
	}{
		{0, 0, 0},
		{1, 1, 10},
		{2, 11, 20},
	}
	for _, tt := range tests {
		got, got1 := factor.Range(tt.pid)
		if got != tt.want {
			t.Errorf("PartitionFactor.Range() got = %v, want %v", got, tt.want)
		}
		if got1 != tt.want1 {
			t.Errorf("PartitionFactor.Range() got1 = %v, want %v", got1, tt.want1)
		}
	}
}

func TestPartitionFactor_Which(t *testing.T) {
	const factor = PartitionFactor(10)
	tests := []struct {
		cn      uint32
		wantPid uint32
	}{
		{0, 0},
		{1, 1},
		{10, 1},
		{11, 2},
		{20, 2},
	}
	for _, tt := range tests {
		if gotPid := factor.Which(tt.cn); gotPid != tt.wantPid {
			t.Errorf("PartitionFactor.Which() = %v, want %v", gotPid, tt.wantPid)
		}
	}
}

func Test_encodePath32(t *testing.T) {
	tests := []struct {
		path []byte
		want uint32
	}{
		{[]byte{}, 0x00000000},
		{[]byte{0}, 0x00000001},
		{[]byte{0, 1}, 0x01000002},
		{[]byte{0, 1, 2, 3, 4, 5, 6}, 0x01234567},       // len == 7
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7}, 0x01234568},    // len > 7
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8}, 0x01234568}, // len > 7
	}
	for _, tt := range tests {
		if got := encodePath32(tt.path); got != tt.want {
			t.Errorf("encodePath32() = %v, want %v", got, tt.want)
		}
	}
}
