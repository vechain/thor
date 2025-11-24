// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logsdb

import (
	"math/rand/v2"
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
		{"max bn", args{BlockNumMask, 1, 2}},
		{"max tx index", args{5, TxIndexMask, 4}},
		{"max log index", args{5, 4, LogIndexMask}},
		{"close to max", args{BlockNumMask - 5, TxIndexMask - 5, LogIndexMask - 5}},
		{"both max", args{BlockNumMask, TxIndexMask, LogIndexMask}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSequence(tt.args.blockNum, tt.args.txIndex, tt.args.logIndex)
			if err != nil {
				t.Error(err)
			}

			assert.True(t, got > 0, "Sequence should be positive")
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

func TestSequence_Errors(t *testing.T) {
	t.Run("blockNum out of range", func(t *testing.T) {
		_, err := NewSequence(BlockNumMask+1, 0, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "block number out of range")
	})
	t.Run("txIndex out of range", func(t *testing.T) {
		_, err := NewSequence(0, TxIndexMask+1, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tx index out of range")
	})
	t.Run("logIndex out of range", func(t *testing.T) {
		_, err := NewSequence(0, 0, LogIndexMask+1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "log index out of range")
	})
}

// In case some one messes up the bit allocation
func TestSequenceValue(t *testing.T) {
	//#nosec G404
	for range 2 {
		blk := rand.Uint32N(BlockNumMask)
		txIndex := rand.Uint32N(TxIndexMask)
		logIndex := rand.Uint32N(LogIndexMask)

		seq, err := NewSequence(blk, txIndex, logIndex)
		assert.Nil(t, err)
		assert.True(t, seq > 0, "Sequence should be positive")

		a := rand.Uint32N(BlockNumMask)
		b := rand.Uint32N(TxIndexMask)
		c := rand.Uint32N(LogIndexMask)

		seq1, err := NewSequence(a, b, c)
		assert.Nil(t, err)
		assert.True(t, seq1 > 0, "Sequence should be positive")

		expected := func() bool {
			if blk != a {
				return blk > a
			}
			if txIndex != b {
				return txIndex > b
			}
			if logIndex != c {
				return logIndex > c
			}
			return false
		}()
		assert.Equal(t, expected, seq > seq1)
	}
}

func TestBitDistribution(t *testing.T) {
	assert.Less(t, blockNumBits+txIndexBits+logIndexBits, 64, "total bits in Sequence should be less than 64")
}
