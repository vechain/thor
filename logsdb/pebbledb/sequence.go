// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"encoding/binary"
	"errors"
	"math"
)

type sequence int64

// Bit allocation compatible with SQLite implementation
const (
	blockNumBits = 28
	txIndexBits  = 15
	logIndexBits = 20

	// Masks for extracting components
	blockNumMask = (1 << blockNumBits) - 1 // 268,435,455
	txIndexMask  = (1 << txIndexBits) - 1  // 32,767
	logIndexMask = (1 << logIndexBits) - 1 // 1,048,575
)

// newSequence creates a new sequence from block number, tx index, and log index
func newSequence(blockNum, txIndex, logIndex uint32) (sequence, error) {
	if blockNum > blockNumMask {
		return 0, errors.New("block number out of range: max 268435455")
	}
	if txIndex > txIndexMask {
		return 0, errors.New("tx index out of range: max 32767")
	}
	if logIndex > logIndexMask {
		return 0, errors.New("log index out of range: max 1048575")
	}

	return (sequence(blockNum) << (txIndexBits + logIndexBits)) |
		(sequence(txIndex) << logIndexBits) |
		sequence(logIndex), nil
}

// BlockNumber extracts the block number from the sequence
func (s sequence) BlockNumber() uint32 {
	return uint32(s>>(txIndexBits+logIndexBits)) & blockNumMask
}

// TxIndex extracts the transaction index from the sequence
func (s sequence) TxIndex() uint32 {
	return uint32((s >> logIndexBits) & txIndexMask)
}

// LogIndex extracts the log index from the sequence
func (s sequence) LogIndex() uint32 {
	return uint32(s & logIndexMask)
}

// BigEndianBytes returns the sequence as 8-byte big-endian representation
// This ensures lexicographic order matches chronological order
func (s sequence) BigEndianBytes() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(s))
	return b
}

// Next returns the next sequence value (used for exclusive upper bounds)
func (s sequence) Next() sequence {
	if s == sequence(math.MaxInt64) {
		return s // Overflow protection
	}
	return s + 1
}

// sequenceFromBytes decodes a sequence from 8-byte big-endian representation
func sequenceFromBytes(b []byte) sequence {
	if len(b) != 8 {
		return 0
	}
	return sequence(binary.BigEndian.Uint64(b))
}

// sequenceFromKey extracts the sequence from the last 8 bytes of a key
func sequenceFromKey(key []byte) sequence {
	if len(key) < 8 {
		return 0
	}
	seqBytes := key[len(key)-8:]
	return sequence(binary.BigEndian.Uint64(seqBytes))
}

// sequenceRangeForBlocks calculates min and max sequences for a block range
func sequenceRangeForBlocks(fromBlock, toBlock uint32) (minSeq, maxSeq sequence, err error) {
	minSeq, err = newSequence(fromBlock, 0, 0)
	if err != nil {
		return 0, 0, err
	}

	maxSeq, err = newSequence(toBlock, txIndexMask, logIndexMask)
	if err != nil {
		return 0, 0, err
	}

	return minSeq, maxSeq, nil
}

// MaxSequenceValue represents the maximum representable sequence value
var MaxSequenceValue = func() sequence {
	maxSeq, _ := newSequence(blockNumMask, txIndexMask, logIndexMask)
	return maxSeq
}()
