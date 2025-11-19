// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package sqlitedb

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

func TestSequence_Errors(t *testing.T) {
	t.Run("blockNum out of range", func(t *testing.T) {
		_, err := newSequence(blockNumMask+1, 0, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "block number out of range")
	})
	t.Run("txIndex out of range", func(t *testing.T) {
		_, err := newSequence(0, txIndexMask+1, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tx index out of range")
	})
	t.Run("logIndex out of range", func(t *testing.T) {
		_, err := newSequence(0, 0, logIndexMask+1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "log index out of range")
	})
}

// In case some one messes up the bit allocation
func TestSequenceValue(t *testing.T) {
	//#nosec G404
	for range 2 {
		blk := rand.Uint32N(blockNumMask)
		txIndex := rand.Uint32N(txIndexMask)
		logIndex := rand.Uint32N(logIndexMask)

		seq, err := newSequence(blk, txIndex, logIndex)
		assert.Nil(t, err)
		assert.True(t, seq > 0, "sequence should be positive")

		a := rand.Uint32N(blockNumMask)
		b := rand.Uint32N(txIndexMask)
		c := rand.Uint32N(logIndexMask)

		seq1, err := newSequence(a, b, c)
		assert.Nil(t, err)
		assert.True(t, seq1 > 0, "sequence should be positive")

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
	assert.Less(t, blockNumBits+txIndexBits+logIndexBits, 64, "total bits in sequence should be less than 64")
}
