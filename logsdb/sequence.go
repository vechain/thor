// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logsdb

import "errors"

type Sequence int64

// Adjust these constants based on your bit allocation requirements
// 64th bit is the sign bit so we have 63 bits to use
const (
	blockNumBits = 28
	txIndexBits  = 15
	logIndexBits = 20
	// Max = 2^28 - 1 = 268,435,455 (unsigned int 28)
	BlockNumMask = (1 << blockNumBits) - 1
	// Max = 2^15 - 1 = 32,767
	TxIndexMask = (1 << txIndexBits) - 1
	// Max = 2^20 - 1 = 1,048,575
	LogIndexMask = (1 << logIndexBits) - 1

	MaxBlockNumber = BlockNumMask
)

func NewSequence(blockNum uint32, txIndex uint32, logIndex uint32) (Sequence, error) {
	if blockNum > BlockNumMask {
		return 0, errors.New("block number out of range: uint28")
	}
	if txIndex > TxIndexMask {
		return 0, errors.New("tx index out of range: uint15")
	}
	if logIndex > LogIndexMask {
		return 0, errors.New("log index out of range: uint21")
	}

	return (Sequence(blockNum) << (txIndexBits + logIndexBits)) |
		(Sequence(txIndex) << logIndexBits) |
		Sequence(logIndex), nil
}

func (s Sequence) BlockNumber() uint32 {
	return uint32(s>>(txIndexBits+logIndexBits)) & BlockNumMask
}

func (s Sequence) TxIndex() uint32 {
	return uint32((s >> logIndexBits) & TxIndexMask)
}

func (s Sequence) LogIndex() uint32 {
	return uint32(s & LogIndexMask)
}
