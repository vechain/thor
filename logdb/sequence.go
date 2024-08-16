// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

type sequence int64

// Adjust these constants based on your bit allocation requirements
const (
	blockNumBits = 31
	txIndexBits  = 12
	logIndexBits = 21
	// Max = 2^31 - 1 = 2,147,483,647
	blockNumMask = (1 << blockNumBits) - 1
	// Max = 2^12 - 1 = 4,095
	txIndexMask = (1 << txIndexBits) - 1
	// Max = 2^21 - 1 = 2,097,151
	logIndexMask = (1 << logIndexBits) - 1
)

func newSequence(blockNum uint32, txIndex uint32, logIndex uint32) sequence {
	if blockNum > blockNumMask {
		panic("block number too large")
	}
	if txIndex > txIndexMask {
		panic("transaction index too large")
	}
	if logIndex > logIndexMask {
		panic("log index too large")
	}
	return (sequence(blockNum) << (txIndexBits + logIndexBits)) |
		(sequence(txIndex) << logIndexBits) |
		sequence(logIndex)
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
