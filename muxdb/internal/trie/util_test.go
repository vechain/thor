// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"reflect"
	"testing"
)

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

func Test_encodePath(t *testing.T) {
	tests := []struct {
		path []byte
		want []byte
	}{
		{[]byte{}, []byte{0, 0, 0, 0}},
		{[]byte{0}, []byte{0, 0, 0, 1}},
		{[]byte{0, 1, 2, 3, 4, 5, 6}, []byte{1, 0x23, 0x45, 0x67}},
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7}, []byte{1, 0x23, 0x45, 0x68, 0x70, 0, 0, 1}},
	}
	for _, tt := range tests {
		if got := encodePath(nil, tt.path); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("encodePath() = %v, want %v", got, tt.want)
		}
	}
}
