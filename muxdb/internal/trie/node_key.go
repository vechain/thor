// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
)

// HistNodeKey provides the buffer and helps to encode historical-node key.
// The historical-node key consists of partition id, path, commitNum, and hash.
// partition id: 4bytes
// path: >= 8 bytes
// commitNum: 4 bytes
// hash: 32 bytes
type HistNodeKey []byte

// Encode returns the encoded historical-node key.
func (k *HistNodeKey) Encode(factor PartitionFactor, hash []byte, commitNum uint32, path []byte) []byte {
	// partition id
	pid := factor.Which(commitNum)
	*k = appendUint32((*k)[:0], pid)

	// path
	for i := 0; i < len(path); i += 7 {
		*k = appendUint32(*k, encodePath32(path[i:]))
	}

	// commit number
	*k = appendUint32(*k, commitNum)
	// hash
	*k = append(*k, hash...)
	return *k
}

// PathBlob returns the path blob.
func (k HistNodeKey) PathBlob() []byte {
	return k[4 : len(k)-4-32]
}

// Hash returns the hash.
func (k HistNodeKey) Hash() []byte {
	return k[len(k)-32:]
}

// CommitNum returns the commit number of the node.
func (k HistNodeKey) CommitNum() uint32 {
	return binary.BigEndian.Uint32(k[len(k)-4-32:])
}

// DedupedNodeKey provide the buffer and helps to encode deduped-node key.
// The deduped-node key consists of partition id and path.
type DedupedNodeKey []byte

// Encode returns the encoded deduped-node key.
func (k *DedupedNodeKey) Encode(factor PartitionFactor, commitNum uint32, path []byte) []byte {
	// partition id
	pid := factor.Which(commitNum)
	*k = appendUint32((*k)[:0], pid)

	// path
	for i := 0; i < len(path); i += 7 {
		*k = appendUint32(*k, encodePath32(path[i:]))
	}
	return *k
}

// FromHistKey convert the historical-node key into deduped-node key.
func (k *DedupedNodeKey) FromHistKey(factor PartitionFactor, hnk HistNodeKey) []byte {
	// partition id
	pid := factor.Which(hnk.CommitNum())
	*k = appendUint32((*k)[:0], pid)
	// path
	*k = append(*k, hnk.PathBlob()...)
	return *k
}

func encodePath32(path []byte) uint32 {
	n := len(path)
	if n > 7 {
		n = 7
	}

	var v uint32
	for i := 0; i < 7; i++ {
		if i < n {
			v |= uint32(path[i])
		}
		v <<= 4
	}
	return v | uint32(n)
}

func appendUint32(b []byte, v uint32) []byte {
	return append(b,
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v),
	)
}
