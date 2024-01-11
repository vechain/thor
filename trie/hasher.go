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
	"fmt"
	"sync"

	"github.com/vechain/thor/v2/thor"
)

type hasher struct {
	buf []byte

	// parameters for storing nodes
	newVer   Version
	cacheTTL uint16
	skipHash bool
}

// cache hashers
var hasherPool = sync.Pool{
	New: func() any {
		return &hasher{}
	},
}

// hash computes and returns the hash of n.
// If force is true, the node is always hashed even smaller than 32 bytes.
func (h *hasher) hash(n node, force bool) []byte {
	switch n := n.(type) {
	case *fullNode:
		// already hashed
		if hash := n.flags.ref.hash; hash != nil {
			return hash
		}
		// hash all children
		for i := 0; i < 16; i++ {
			if cn := n.children[i]; cn != nil {
				h.hash(cn, false)
			}
		}

		h.buf = n.encodeConsensus(h.buf[:0])
		if len(h.buf) >= 32 || force {
			n.flags.ref.hash = thor.Blake2b(h.buf).Bytes()
			return n.flags.ref.hash
		}
		return nil
	case *shortNode:
		// already hashed
		if hash := n.flags.ref.hash; hash != nil {
			return hash
		}

		// hash child node
		h.hash(n.child, false)

		h.buf = n.encodeConsensus(h.buf[:0])
		if len(h.buf) >= 32 || force {
			n.flags.ref.hash = thor.Blake2b(h.buf).Bytes()
			return n.flags.ref.hash
		}
		return nil
	case *refNode:
		return n.hash
	case *valueNode:
		return nil
	default:
		panic(fmt.Sprintf("hash %T: unexpected node: %v", n, n))
	}
}

// store stores node n and all its dirty sub nodes.
// Root node is always stored regardless of its dirty flag.
func (h *hasher) store(n node, db DatabaseWriter, path []byte) (node, error) {
	isRoot := len(path) == 0

	switch n := n.(type) {
	case *fullNode:
		n = n.copy()
		for i := 0; i < 16; i++ {
			cn := n.children[i]
			switch cn := cn.(type) {
			case *fullNode, *shortNode:
				// store the child node if dirty
				if ref, gen, dirty := cn.cache(); dirty {
					nn, err := h.store(cn, db, append(path, byte(i)))
					if err != nil {
						return nil, err
					}
					n.children[i] = nn
				} else {
					// drop the cached node by replacing with its ref node when ttl reached
					if n.flags.gen-gen > h.cacheTTL {
						n.children[i] = &ref
					}
				}
			}
		}

		// full node is stored in case of
		// 1. it's the root node
		// 2. it has hash value
		// 3. hash is being skipped
		if isRoot || n.flags.ref.hash != nil || h.skipHash {
			h.buf = n.encode(h.buf[:0], h.skipHash)
			if err := db.Put(path, h.newVer, h.buf); err != nil {
				return nil, err
			}
			n.flags.dirty = false
			n.flags.ref.ver = h.newVer
		}
		return n, nil
	case *shortNode:
		n = n.copy()
		switch cn := n.child.(type) {
		case *fullNode, *shortNode:
			if ref, gen, dirty := cn.cache(); dirty {
				// store the child node if dirty
				nn, err := h.store(cn, db, append(path, n.key...))
				if err != nil {
					return nil, err
				}
				n.child = nn
			} else {
				// drop the cached node by replacing with its ref node when ttl reached
				if n.flags.gen-gen > h.cacheTTL {
					n.child = &ref
				}
			}
		}

		// short node is stored when only when it's the root node
		//
		// This is a very significant improvement compared to maindb-v3. Short-nodes are embedded
		// in full-nodes whenever possible. Doing this can save huge storage space, because the
		// 32-byte hash value of the short-node is omitted, and most short-nodes themselves are small,
		// only slightly larger than 32 bytes.
		if isRoot {
			h.buf = n.encode(h.buf[:0], h.skipHash)
			if err := db.Put(path, h.newVer, h.buf); err != nil {
				return nil, err
			}
			n.flags.dirty = false
			n.flags.ref.ver = h.newVer
		}
		return n, nil
	default:
		panic(fmt.Sprintf("store %T: unexpected node: %v", n, n))
	}
}
