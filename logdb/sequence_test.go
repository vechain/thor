// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSequence(t *testing.T) {
	type args struct {
		blockNum uint32
		txIndex  uint32
		logIndex uint32
	}
	tests := []struct {
		name string
		args args
	}{
		{"regular", args{1, 2, 3}},
		{"max bn", args{blockNumMask, 1, 2}},
		{"max tx index", args{5, txIndexMask, 4}},
		{"max log index", args{5, 4, logIndexMask}},
		{"close to max", args{blockNumMask - 5, txIndexMask - 5, logIndexMask - 5}},
		{"both max", args{blockNumMask, txIndexMask, logIndexMask}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newSequence(tt.args.blockNum, tt.args.txIndex, tt.args.logIndex)
			if err != nil {
				t.Error(err)
			}

			assert.True(t, got > 0, "sequence should be positive")
			if bn := got.BlockNumber(); bn != tt.args.blockNum {
				t.Errorf("seq.blockNum() = %v, want %v", bn, tt.args.blockNum)
			}
			if ti := got.TxIndex(); ti != tt.args.txIndex {
				t.Errorf("seq.txIndex() = %v, want %v", ti, tt.args.txIndex)
			}
			if i := got.LogIndex(); i != tt.args.logIndex {
				t.Errorf("seq.index() = %v, want %v", i, tt.args.logIndex)
			}
		})
	}
}
