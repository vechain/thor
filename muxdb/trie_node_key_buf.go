// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/trie"
)

// trieNodeKeyBuf buffer for trie node key composition.
// A trie node key is composed by [space, name, path, hash].
// space - 1 byte
// name - var len
// path - 8 bytes
// hash - 32 bytes
type trieNodeKeyBuf []byte

func newTrieNodeKeyBuf(name string) trieNodeKeyBuf {
	nameLen := len(name)
	buf := make([]byte, 1+nameLen+8+32)
	copy(buf[1:], name)
	return buf
}

func (b trieNodeKeyBuf) spaceSlot() *byte {
	return &b[0]
}

func (b trieNodeKeyBuf) pathSlot() []byte {
	offset := len(b) - 32 - 8
	return b[offset : offset+8]
}

func (b trieNodeKeyBuf) hashSlot() []byte {
	offset := len(b) - 32
	return b[offset:]
}

// Get gets encoded trie node from kv store.
func (b trieNodeKeyBuf) Get(get kv.GetFunc, key *trie.NodeKey) ([]byte, error) {
	b.compactPath(key.Path)
	copy(b.hashSlot(), key.Hash)

	spaceSlot := b.spaceSlot()

	// try to get from permanat space
	*spaceSlot = trieSpaceP
	if val, err := get(b); err == nil {
		return val, nil
	}

	// then live space a
	*spaceSlot = trieSpaceA
	if val, err := get(b); err == nil {
		return val, nil
	}

	// finally space b
	*spaceSlot = trieSpaceB
	return get(b)
}

// Put put encoded trie node to the given space.
func (b trieNodeKeyBuf) Put(put kv.PutFunc, key *trie.NodeKey, enc []byte, space byte) error {
	*b.spaceSlot() = space
	b.compactPath(key.Path)
	copy(b.hashSlot(), key.Hash)

	return put(b, enc)
}

func (b trieNodeKeyBuf) compactPath(path []byte) {
	pathSlot := b.pathSlot()
	for i := 0; i < 8; i++ {
		pathSlot[i] = 0
	}

	pathLen := len(path)
	if pathLen > 15 {
		pathLen = 15
	}

	if pathLen > 0 {
		// compact at most 15 nibbles and term with path len.
		for i := 0; i < pathLen; i++ {
			if i%2 == 0 {
				pathSlot[i/2] |= (path[i] << 4)
			} else {
				pathSlot[i/2] |= path[i]
			}
		}
		pathSlot[7] |= byte(pathLen)
	} else {
		// narrow the affected key range of nodes produced by trie commitment.
		pathSlot[0] = (8 << 4)
	}
}
