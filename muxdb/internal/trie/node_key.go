// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"math"
)

// nodeKey helps encode/decode trie node key.
// A trie node key is composed in (space, name, path, commitNum, hash).
// space - 1 byte
// name - var len
// path - 8 bytes
// commitNum - 4 bytes
// hash - 32 bytes
type nodeKey []byte

// newNodeKey create a node key with the given trie name.
func newNodeKey(name string) nodeKey {
	buf := make([]byte, 1+len(name)+8+4+32)
	copy(buf[1:], name)
	return buf
}

// Encode returns the encoded key for saving trie node.
func (k nodeKey) Encode(hash []byte, commitNum uint32, path []byte) []byte {
	if len(path) <= 15 {
		k[0] = NodeSpace       // space
		p := k[len(k)-8-4-32:] // path
		c := p[8:]             // commit num
		h := c[4:]             // hash

		binary.BigEndian.PutUint64(p, uint64(newPath64(path)))
		binary.BigEndian.PutUint32(c, commitNum)
		copy(h, hash)
		return k
	}

	// for a path-len overflowed node, the key is encoded as (space, name, commitNum, hash)
	k[0] = OverflowNodeSpace // space
	c := k[len(k)-8-4-32:]   // commit num
	h := c[4:]               // hash

	binary.BigEndian.PutUint32(c, commitNum)
	copy(h, hash)
	return k[:len(k)-8]
}

// Path returns the path part.
func (k nodeKey) Path() path64 {
	if k[0] == NodeSpace {
		return path64(binary.BigEndian.Uint64(k[len(k)-8-4-32:]))
	}
	return math.MaxUint64
}

// CommitNum returns the commit num part.
func (k nodeKey) CommitNum() uint32 {
	return binary.BigEndian.Uint32(k[len(k)-4-32:])
}
