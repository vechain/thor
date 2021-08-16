// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"reflect"
	"testing"

	"github.com/vechain/thor/thor"
)

func Test_newNodeKey(t *testing.T) {
	tests := []struct {
		name string
		want nodeKey
	}{
		{"test", append(append([]byte{0}, "test"...), make([]byte, 44)...)},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := newNodeKey(tt.name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newNodeKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nodeKey_Encode(t *testing.T) {
	type args struct {
		hash      []byte
		commitNum uint32
		path      []byte
	}
	tests := []struct {
		name string
		k    nodeKey
		args args
		want []byte
	}{
		{"regular", newNodeKey("test"), args{[]byte{1}, 2, []byte{3}},
			append(append([]byte{NodeSpace}, "test"...),
				0x30, 0, 0, 0, 0, 0, 0, 1,
				0, 0, 0, 2,
				1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			),
		},
		{"overflow", newNodeKey("test"), args{[]byte{1}, 2, []byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}},
			append(append([]byte{OverflowNodeSpace}, "test"...),
				0, 0, 0, 2,
				1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.k.Encode(tt.args.hash, tt.args.commitNum, tt.args.path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nodeKey.Encode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nodeKey_Path(t *testing.T) {
	tests := []struct {
		k    nodeKey
		want path64
	}{
		{nodeKey(newNodeKey("test").Encode((thor.Bytes32{}).Bytes(), 0, []byte{2})), 0x2000000000000001},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := tt.k.Path(); got != tt.want {
				t.Errorf("nodeKey.Path() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nodeKey_CommitNum(t *testing.T) {
	tests := []struct {
		k    nodeKey
		want uint32
	}{
		{nodeKey(newNodeKey("test").Encode((thor.Bytes32{}).Bytes(), 1, []byte{2})), 1},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := tt.k.CommitNum(); got != tt.want {
				t.Errorf("nodeKey.CommitNum() = %v, want %v", got, tt.want)
			}
		})
	}
}
