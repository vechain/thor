// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/lowrlp"
	"github.com/vechain/thor/thor"
)

type hasher struct {
	enc      lowrlp.Encoder
	tmp      sliceBuffer
	cacheGen uint16
	cacheTTL uint16

	extended  bool
	seq       uint64
	nonCrypto bool
}

type sliceBuffer []byte

func (b *sliceBuffer) Write(data []byte) (n int, err error) {
	*b = append(*b, data...)
	return len(data), nil
}

func (b *sliceBuffer) Reset() {
	*b = (*b)[:0]
}

// hashers live in a global pool.
var hasherPool = sync.Pool{
	New: func() interface{} {
		return &hasher{
			tmp: make(sliceBuffer, 0, 700), // cap is as large as a full fullNode.
		}
	},
}

func newHasher(cacheGen, cacheTTL uint16) *hasher {
	h := hasherPool.Get().(*hasher)
	h.cacheGen = cacheGen
	h.cacheTTL = cacheTTL
	h.extended = false
	h.seq = 0
	h.nonCrypto = false
	return h
}

func newHasherExtended(cacheGen, cacheTTL uint16, seq uint64, nonCrypto bool) *hasher {
	h := hasherPool.Get().(*hasher)
	h.cacheGen = cacheGen
	h.cacheTTL = cacheTTL
	h.extended = true
	h.seq = seq
	h.nonCrypto = nonCrypto
	return h
}

func returnHasherToPool(h *hasher) {
	hasherPool.Put(h)
}

// hash collapses a node down into a hash node, also returning a copy of the
// original node initialized with the computed hash to replace the original one.
func (h *hasher) hash(n node, db DatabaseWriter, path []byte, force bool) (node, node, error) {
	// If we're not storing the node, just hashing, use available cached data
	if hash, dirty, gen := n.cache(); hash != nil {
		if db == nil {
			return hash, n, nil
		}

		if !dirty {
			if !force { // non-root node
				if h.cacheGen-gen > h.cacheTTL { // drop cached nodes exceeds life-time
					return hash, hash, nil
				}
				return hash, n, nil
			}

			if !h.extended {
				return hash, n, nil
			}
			// else for extended trie, always store root node regardless of dirty flag
		}
	}
	// Trie not processed yet or needs storage, walk the children
	collapsed, cached, err := h.hashChildren(n, db, path)
	if err != nil {
		return nil, n, err
	}
	hashed, err := h.store(collapsed, db, path, force)
	if err != nil {
		return nil, n, err
	}
	// Cache the hash of the node for later reuse and remove
	// the dirty flag in commit mode. It's fine to assign these values directly
	// without copying the node first because hashChildren copies it.
	cachedHash, _ := hashed.(*hashNode)
	switch cn := cached.(type) {
	case *shortNode:
		cn.flags.hash = cachedHash
		if db != nil {
			cn.flags.dirty = false
		}
	case *fullNode:
		cn.flags.hash = cachedHash
		if db != nil {
			cn.flags.dirty = false
		}
	}
	return hashed, cached, nil
}

// hashChildren replaces the children of a node with their hashes if the encoded
// size of the child is larger than a hash, returning the collapsed node as well
// as a replacement for the original node with the child hashes cached in.
func (h *hasher) hashChildren(original node, db DatabaseWriter, path []byte) (node, node, error) {
	var err error

	switch n := original.(type) {
	case *shortNode:
		// Hash the short node's child, caching the newly hashed subtree
		collapsed, cached := n.copy(), n.copy()
		collapsed.Key = hexToCompact(n.Key)
		cached.Key = common.CopyBytes(n.Key)

		if _, ok := n.Val.(*valueNode); !ok {

			collapsed.Val, cached.Val, err = h.hash(n.Val, db, append(path, n.Key...), false)
			if err != nil {
				return original, original, err
			}
		}
		// no need when using frlp
		// if collapsed.Val == nil {
		// 	collapsed.Val = &valueNode{} // Ensure that nil children are encoded as empty strings.
		// }
		return collapsed, cached, nil

	case *fullNode:
		// Hash the full node's children, caching the newly hashed subtrees
		collapsed, cached := n.copy(), n.copy()

		for i := 0; i < 16; i++ {
			if n.Children[i] != nil {
				collapsed.Children[i], cached.Children[i], err = h.hash(n.Children[i], db, append(path, byte(i)), false)
				if err != nil {
					return original, original, err
				}
			}
			// no need when using frlp
			// else {
			// 	collapsed.Children[i] = &valueNode{} // Ensure that nil children are encoded as empty strings.
			// }
		}
		// no need when using frlp
		// if collapsed.Children[16] == nil {
		// 	collapsed.Children[16] = &valueNode{}
		// }
		return collapsed, cached, nil

	default:
		// Value and hash nodes don't have children so they're left as were
		return n, original, nil
	}
}

func (h *hasher) store(n node, db DatabaseWriter, path []byte, force bool) (node, error) {
	// Don't store hashes or empty nodes.
	if _, isHash := n.(*hashNode); n == nil || isHash {
		return n, nil
	}
	// Generate the RLP encoding of the node
	h.enc.Reset()
	n.encode(&h.enc, h.nonCrypto)
	h.tmp.Reset()
	h.enc.ToWriter(&h.tmp)

	if h.nonCrypto {
		// fullnode and shortnode with non-value child are forced
		// just like normal trie.
		switch n := n.(type) {
		case *fullNode:
			force = true
		case *shortNode:
			if _, ok := n.Val.(*valueNode); !ok {
				force = true
			}
		}
	}

	if len(h.tmp) < 32 && !force {
		return n, nil // Nodes smaller than 32 bytes are stored inside their parent
	}
	// Larger nodes are replaced by their hash and stored in the database.
	hash, _, _ := n.cache()
	if hash == nil {
		hash = &hashNode{}
		if h.nonCrypto {
			hash.Hash = NonCryptoNodeHash
		} else {
			hash.Hash = thor.Blake2b(h.tmp)
		}
	} else {
		cpy := *hash
		hash = &cpy
	}
	if db != nil {
		// extended
		if h.extended {
			h.enc.Reset()
			n.encodeTrailing(&h.enc)
			h.enc.ToWriter(&h.tmp)
			hash.seq = h.seq
		}

		key := hash.Hash[:]
		if ke, ok := db.(DatabaseKeyEncoder); ok {
			key = ke.Encode(hash.Hash[:], h.seq, path)
		}
		return hash, db.Put(key, h.tmp)
	}
	return hash, nil
}
