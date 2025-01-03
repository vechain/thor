// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import "errors"

type sequence int64

// Adjust these constants based on your bit allocation requirements
// 64th bit is the sign bit so we have 63 bits to use
const (
	blockNumBits = 28
	txIndexBits  = 15
	logIndexBits = 20
	// Max = 2^28 - 1 = 268,435,455 (unsigned int 28)
	blockNumMask = (1 << blockNumBits) - 1
	// Max = 2^15 - 1 = 32,767
	txIndexMask = (1 << txIndexBits) - 1
	// Max = 2^20 - 1 = 1,048,575
	logIndexMask = (1 << logIndexBits) - 1

	MaxBlockNumber = blockNumMask
)

func newSequence(blockNum uint32, txIndex uint32, logIndex uint32) (sequence, error) {
	if blockNum > blockNumMask {
		return 0, errors.New("block number out of range: uint28")
	}
	if txIndex > txIndexMask {
		return 0, errors.New("tx index out of range: uint15")
	}
	if logIndex > logIndexMask {
		return 0, errors.New("log index out of range: uint21")
	}

	return (sequence(blockNum) << (txIndexBits + logIndexBits)) |
		(sequence(txIndex) << logIndexBits) |
		sequence(logIndex), nil
}

func (s sequence) BlockNumber() uint32 {
	return uint32(s>>(txIndexBits+logIndexBits)) & blockNumMask
}

func (s sequence) TxIndex() uint32 {
	return uint32((s >> logIndexBits) & txIndexMask)
}

func (s sequence) LogIndex() uint32 {
	return uint32(s & logIndexMask)
}
