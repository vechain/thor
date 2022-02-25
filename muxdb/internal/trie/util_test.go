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
		{[]byte{}, []byte{0, 0, 0}},
		{[]byte{8}, []byte{0, 0x80, 1}},
		{[]byte{8, 9}, []byte{0, 0x89, 2}},
		{[]byte{8, 9, 0xa}, []byte{0, 0x89, 0xa3}},
		{[]byte{8, 9, 0xa, 0xb}, []byte{1, 0x89, 0xab, 0, 0}},
		{[]byte{8, 9, 0xa, 0xb, 0xc}, []byte{1, 0x89, 0xab, 0xc0, 1}},
		{[]byte{8, 9, 0xa, 0xb, 0xc, 0xd}, []byte{1, 0x89, 0xab, 0xcd, 2}},
		{[]byte{8, 9, 0xa, 0xb, 0xc, 0xd, 0xe}, []byte{1, 0x89, 0xab, 0xcd, 0xe3}},
	}
	for _, tt := range tests {
		if got := encodePath(nil, tt.path); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("encodePath() = %v, want %v", got, tt.want)
		}
	}
}
