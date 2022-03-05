// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"reflect"
	"testing"
)

func Test_encodePath(t *testing.T) {
	tests := []struct {
		path []byte
		want []byte
	}{
		{[]byte{}, []byte{0, 0}},
		{[]byte{8}, []byte{0, (8 << 3) | 1}},
		{[]byte{8, 9}, []byte{0, 0x80 | (8 << 3) | (9 >> 1), 0x80}},
		{[]byte{8, 9, 0xa}, []byte{0, 0xc4, 0x80 | (0xa << 3) | 1}},
		{[]byte{8, 9, 0xa, 0xb}, []byte{1, 0x89, 0xab, 0}},
		{[]byte{8, 9, 0xa, 0xb, 0xc}, []byte{1, 0x89, 0xab, (0xc << 3) | 1}},
		{[]byte{8, 9, 0xa, 0xb, 0xc, 0xd}, []byte{1, 0x89, 0xab, 0x80 | (0xc << 3) | (0xd >> 1), 0x80}},
		{[]byte{8, 9, 0xa, 0xb, 0xc, 0xd, 0xe}, []byte{1, 0x89, 0xab, 0x80 | (0xc << 3) | (0xd >> 1), 0x80 | (0xe << 3) | 1}},
	}
	for _, tt := range tests {
		if got := encodePath(nil, tt.path); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("encodePath() = %v, want %v", got, tt.want)
		}
	}
}
