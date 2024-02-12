// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/trie"
)

// backend is the backend of the trie.
type backend struct {
	Store                           kv.Store
	Cache                           *cache
	HistPtnFactor, DedupedPtnFactor uint32
	CachedNodeTTL                   uint16
}

// AppendHistNodeKey composes hist node key and appends to buf.
func (b *backend) AppendHistNodeKey(buf []byte, name string, path []byte, ver trie.Version) []byte {
	// encoding node keys in this way has the following benefits:
	// 1. nodes are stored in order of partition id, which is friendly to LSM DB.
	// 2. adjacent versions of a node are stored together,
	//    so that node data is well compressed (ref https://gist.github.com/qianbin/bffcd248b7312c35d7d526a974018b1b )
	buf = append(buf, trieHistSpace)       // space
	if b.HistPtnFactor != math.MaxUint32 { // partition id
		buf = binary.BigEndian.AppendUint32(buf, ver.Major/b.HistPtnFactor)
	}
	buf = append(buf, name...)                          // trie name
	buf = appendNodePath(buf, path)                     // path
	buf = binary.BigEndian.AppendUint32(buf, ver.Major) // major ver
	if ver.Minor != 0 {                                 // minor ver
		buf = binary.AppendUvarint(buf, uint64(ver.Minor))
	}
	return buf
}

// AppendDedupedNodeKey composes deduped node key and appends to buf.
func (b *backend) AppendDedupedNodeKey(buf []byte, name string, path []byte, ver trie.Version) []byte {
	buf = append(buf, trieDedupedSpace)       // space
	if b.DedupedPtnFactor != math.MaxUint32 { // partition id
		buf = binary.BigEndian.AppendUint32(buf, ver.Major/b.DedupedPtnFactor)
	}
	buf = append(buf, name...)      // trie name
	buf = appendNodePath(buf, path) // path
	return buf
}

// DeleteHistoryNode deletes trie history nodes within partitions of [startMajorVer, limitMajorVer).
func (b *backend) DeleteHistoryNode(ctx context.Context, startMajorVer, limitMajorVer uint32) error {
	startPtn := startMajorVer / b.HistPtnFactor
	limitPtn := limitMajorVer / b.HistPtnFactor

	return b.Store.DeleteRange(ctx, kv.Range{
		Start: binary.BigEndian.AppendUint32([]byte{trieHistSpace}, startPtn),
		Limit: binary.BigEndian.AppendUint32([]byte{trieHistSpace}, limitPtn),
	})
}

// appendNodePath encodes the node path and appends to buf.
func appendNodePath(buf, path []byte) []byte {
	switch len(path) {
	case 0:
		return append(buf, 0, 0)
	case 1:
		return append(buf, path[0], 1)
	case 2:
		return append(buf, path[0], (path[1]<<4)|2)
	default:
		// has more
		buf = append(buf, path[0]|0x10, (path[1]<<4)|2)
		return appendNodePath(buf, path[2:])
	}
}
