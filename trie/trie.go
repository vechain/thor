// Copyright 2014 The go-ethereum Authors
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

// Package trie implements Merkle Patricia Tries.
package trie

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

// This is the known root hash of an empty trie.
var emptyRoot = thor.Blake2b(rlp.EmptyString)

// Version is the version number of a standalone trie node.
type Version struct {
	Major,
	Minor uint32
}

// String pretty prints version.
func (v Version) String() string {
	return fmt.Sprintf("%v.%v", v.Major, v.Minor)
}

// Compare compares with b.
// The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
func (a Version) Compare(b Version) int {
	if a.Major > b.Major {
		return 1
	}
	if a.Major < b.Major {
		return -1
	}
	if a.Minor > b.Minor {
		return 1
	}
	if a.Minor < b.Minor {
		return -1
	}
	return 0
}

// Root wraps hash and version of the root node.
type Root struct {
	Hash thor.Bytes32
	Ver  Version
}

// Node is the alias of inner node type.
type Node = node

// DatabaseReader wraps the Get method of a backing store for the trie.
type DatabaseReader interface {
	Get(path []byte, ver Version) (value []byte, err error)
}

// DatabaseWriter wraps the Put method of a backing store for the trie.
type DatabaseWriter interface {
	// Put stores the mapping (path, ver)->value in the database.
	// Implementations must not hold onto the value bytes, the trie
	// will reuse the slice across calls to Put.
	Put(path []byte, ver Version, value []byte) error
}

// Trie is a Merkle Patricia Trie.
// The zero value is an empty trie with no database.
// Use New to create a trie that sits on top of a database.
//
// Trie is not safe for concurrent use.
type Trie struct {
	root node
	db   DatabaseReader

	cacheGen uint16 // cache generation counter for next committed nodes
	cacheTTL uint16 // the life time of cached nodes
}

// SetCacheTTL sets the number of 'cache generations' to keep.
// A cache generation is increased by a call to Commit.
func (t *Trie) SetCacheTTL(ttl uint16) {
	t.cacheTTL = ttl
}

// newFlag returns the cache flag value for a newly created node.
func (t *Trie) newFlag() nodeFlag {
	return nodeFlag{dirty: true, gen: t.cacheGen}
}

// RootNode returns the root node.
func (t *Trie) RootNode() Node {
	return t.root
}

// New creates a trie with an existing root node from db.
//
// If root hash is zero or the hash of an empty string, the trie is initially empty .
// Accessing the trie loads nodes from db on demand.
func New(root Root, db DatabaseReader) *Trie {
	if root.Hash == emptyRoot || root.Hash.IsZero() {
		return &Trie{db: db}
	}

	return &Trie{
		root: &refNode{root.Hash.Bytes(), root.Ver},
		db:   db,
	}
}

// FromRootNode creates a trie from a live root node.
func FromRootNode(rootNode Node, db DatabaseReader) *Trie {
	if rootNode != nil {
		_, gen, _ := rootNode.cache()
		return &Trie{
			root:     rootNode,
			db:       db,
			cacheGen: gen + 1, // cacheGen is always one bigger than gen of root node
		}
	}
	// allows nil root node
	return &Trie{db: db}
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key. Nodes with version smaller than minVer are filtered out.
func (t *Trie) NodeIterator(start []byte, minVer Version) NodeIterator {
	return newNodeIterator(t, start, minVer)
}

// Get returns the value with meta for key stored in the trie.
// The value and meta bytes must not be modified by the caller.
// If a node was not found in the database, a MissingNodeError is returned.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	value, newRoot, _, err := t.tryGet(t.root, keybytesToHex(key), 0)
	if err != nil {
		return nil, nil, err
	}
	t.root = newRoot
	if value != nil {
		return value.val, value.meta, nil
	}
	return nil, nil, nil
}

func (t *Trie) tryGet(origNode node, key []byte, pos int) (value *valueNode, newnode node, didResolve bool, err error) {
	switch n := (origNode).(type) {
	case nil:
		return nil, nil, false, nil
	case *valueNode:
		return n, n, false, nil
	case *shortNode:
		if len(key)-pos < len(n.key) || !bytes.Equal(n.key, key[pos:pos+len(n.key)]) {
			// key not found in trie
			return nil, n, false, nil
		}
		if value, newnode, didResolve, err = t.tryGet(n.child, key, pos+len(n.key)); err != nil {
			return
		}
		if didResolve {
			n = n.copy()
			n.child = newnode
			n.flags.gen = t.cacheGen
		}
		return value, n, didResolve, nil
	case *fullNode:
		if value, newnode, didResolve, err = t.tryGet(n.children[key[pos]], key, pos+1); err != nil {
			return
		}
		if didResolve {
			n = n.copy()
			n.flags.gen = t.cacheGen
			n.children[key[pos]] = newnode
		}
		return value, n, didResolve, nil
	case *refNode:
		var child node
		if child, err = t.resolveRef(n, key[:pos]); err != nil {
			return
		}
		if value, newnode, _, err = t.tryGet(child, key, pos); err != nil {
			return
		}
		return value, newnode, true, nil
	default:
		panic(fmt.Sprintf("%T: invalid node: %v", origNode, origNode))
	}
}

// Update associates key with value in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value bytes must not be modified by the caller while they are
// stored in the trie.
//
// If a node was not found in the database, a MissingNodeError is returned.
func (t *Trie) Update(key, value, meta []byte) error {
	k := keybytesToHex(key)
	if len(value) != 0 {
		_, n, err := t.insert(t.root, nil, k, &valueNode{value, meta})
		if err != nil {
			return err
		}
		t.root = n
	} else {
		_, n, err := t.delete(t.root, nil, k)
		if err != nil {
			return err
		}
		t.root = n
	}
	return nil
}

func (t *Trie) insert(n node, prefix, key []byte, value node) (bool, node, error) {
	if len(key) == 0 {
		if v, ok := n.(*valueNode); ok {
			newVal := value.(*valueNode)
			// dirty when value or meta is not equal
			return !bytes.Equal(v.val, newVal.val) || !bytes.Equal(v.meta, newVal.meta), value, nil
		}
		return true, value, nil
	}
	switch n := n.(type) {
	case *shortNode:
		matchlen := prefixLen(key, n.key)
		// If the whole key matches, keep this short node as is
		// and only update the value.
		if matchlen == len(n.key) {
			dirty, nn, err := t.insert(n.child, append(prefix, key[:matchlen]...), key[matchlen:], value)
			if !dirty || err != nil {
				return false, n, err
			}
			return true, &shortNode{n.key, nn, t.newFlag()}, nil
		}
		// Otherwise branch out at the index where they differ.
		branch := &fullNode{flags: t.newFlag()}
		var err error
		_, branch.children[n.key[matchlen]], err = t.insert(nil, append(prefix, n.key[:matchlen+1]...), n.key[matchlen+1:], n.child)
		if err != nil {
			return false, nil, err
		}
		_, branch.children[key[matchlen]], err = t.insert(nil, append(prefix, key[:matchlen+1]...), key[matchlen+1:], value)
		if err != nil {
			return false, nil, err
		}
		// Replace this shortNode with the branch if it occurs at index 0.
		if matchlen == 0 {
			return true, branch, nil
		}
		// Otherwise, replace it with a short node leading up to the branch.
		return true, &shortNode{key[:matchlen], branch, t.newFlag()}, nil

	case *fullNode:
		dirty, nn, err := t.insert(n.children[key[0]], append(prefix, key[0]), key[1:], value)
		if !dirty || err != nil {
			return false, n, err
		}
		n = n.copy()
		n.flags = t.newFlag()
		n.children[key[0]] = nn
		return true, n, nil

	case nil:
		return true, &shortNode{key, value, t.newFlag()}, nil

	case *refNode:
		// We've hit a part of the trie that isn't loaded yet. Load
		// the node and insert into it. This leaves all child nodes on
		// the path to the value in the trie.
		rn, err := t.resolveRef(n, prefix)
		if err != nil {
			return false, nil, err
		}
		dirty, nn, err := t.insert(rn, prefix, key, value)
		if !dirty || err != nil {
			return false, rn, err
		}
		return true, nn, nil

	default:
		panic(fmt.Sprintf("%T: invalid node: %v", n, n))
	}
}

// delete returns the new root of the trie with key deleted.
// It reduces the trie to minimal form by simplifying
// nodes on the way up after deleting recursively.
func (t *Trie) delete(n node, prefix, key []byte) (bool, node, error) {
	switch n := n.(type) {
	case *shortNode:
		matchlen := prefixLen(key, n.key)
		if matchlen < len(n.key) {
			return false, n, nil // don't replace n on mismatch
		}
		if matchlen == len(key) {
			return true, nil, nil // remove n entirely for whole matches
		}
		// The key is longer than n.Key. Remove the remaining suffix
		// from the subtrie. Child can never be nil here since the
		// subtrie must contain at least two other values with keys
		// longer than n.Key.
		dirty, child, err := t.delete(n.child, append(prefix, key[:len(n.key)]...), key[len(n.key):])
		if !dirty || err != nil {
			return false, n, err
		}
		switch child := child.(type) {
		case *shortNode:
			// Deleting from the subtrie reduced it to another
			// short node. Merge the nodes to avoid creating a
			// shortNode{..., shortNode{...}}. Use concat (which
			// always creates a new slice) instead of append to
			// avoid modifying n.Key since it might be shared with
			// other nodes.
			return true, &shortNode{concat(n.key, child.key...), child.child, t.newFlag()}, nil
		default:
			return true, &shortNode{n.key, child, t.newFlag()}, nil
		}

	case *fullNode:
		dirty, nn, err := t.delete(n.children[key[0]], append(prefix, key[0]), key[1:])
		if !dirty || err != nil {
			return false, n, err
		}
		n = n.copy()
		n.flags = t.newFlag()
		n.children[key[0]] = nn

		// Check how many non-nil entries are left after deleting and
		// reduce the full node to a short node if only one entry is
		// left. Since n must've contained at least two children
		// before deletion (otherwise it would not be a full node) n
		// can never be reduced to nil.
		//
		// When the loop is done, pos contains the index of the single
		// value that is left in n or -2 if n contains at least two
		// values.
		pos := -1
		for i, cld := range n.children {
			if cld != nil {
				if pos == -1 {
					pos = i
				} else {
					pos = -2
					break
				}
			}
		}
		if pos >= 0 {
			if pos != 16 {
				// If the remaining entry is a short node, it replaces
				// n and its key gets the missing nibble tacked to the
				// front. This avoids creating an invalid
				// shortNode{..., shortNode{...}}.  Since the entry
				// might not be loaded yet, resolve it just for this
				// check.
				cnode, err := t.resolve(n.children[pos], append(prefix, byte(pos)))
				if err != nil {
					return false, nil, err
				}
				if cnode, ok := cnode.(*shortNode); ok {
					k := append([]byte{byte(pos)}, cnode.key...)
					return true, &shortNode{k, cnode.child, t.newFlag()}, nil
				}
			}
			// Otherwise, n is replaced by a one-nibble short node
			// containing the child.
			return true, &shortNode{[]byte{byte(pos)}, n.children[pos], t.newFlag()}, nil
		}
		// n still contains at least two values and cannot be reduced.
		return true, n, nil

	case *valueNode:
		return true, nil, nil

	case nil:
		return false, nil, nil

	case *refNode:
		// We've hit a part of the trie that isn't loaded yet. Load
		// the node and delete from it. This leaves all child nodes on
		// the path to the value in the trie.
		rn, err := t.resolveRef(n, prefix)
		if err != nil {
			return false, nil, err
		}
		dirty, nn, err := t.delete(rn, prefix, key)
		if !dirty || err != nil {
			return false, rn, err
		}
		return true, nn, nil

	default:
		panic(fmt.Sprintf("%T: invalid node: %v (%v)", n, n, key))
	}
}

func concat(s1 []byte, s2 ...byte) []byte {
	r := make([]byte, len(s1)+len(s2))
	copy(r, s1)
	copy(r[len(s1):], s2)
	return r
}

func (t *Trie) resolve(n node, prefix []byte) (node, error) {
	if ref, ok := n.(*refNode); ok {
		node, err := t.resolveRef(ref, prefix)
		return node, err
	}
	return n, nil
}

func (t *Trie) resolveRef(ref *refNode, prefix []byte) (node, error) {
	blob, err := t.db.Get(prefix, ref.ver)
	if err != nil {
		return nil, &MissingNodeError{Ref: *ref, Path: prefix, Err: err}
	}
	return mustDecodeNode(ref, blob, t.cacheGen), nil
}

// Hash returns the root hash of the trie. It does not write to the
// database and can be used even if the trie doesn't have one.
func (t *Trie) Hash() thor.Bytes32 {
	if t.root == nil {
		return emptyRoot
	}

	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	hash := h.hash(t.root, true)
	return thor.BytesToBytes32(hash)
}

// Commit writes all nodes to the trie's database.
//
// Committing flushes nodes from memory.
// Subsequent Get calls will load nodes from the database.
// If skipHash is true, less disk space is taken up but crypto features of merkle trie lost.
func (t *Trie) Commit(db DatabaseWriter, newVer Version, skipHash bool) error {
	if t.root == nil {
		return nil
	}

	// the root node might be refNode, resolve it before later process.
	resolved, err := t.resolve(t.root, nil)
	if err != nil {
		return err
	}

	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)
	if !skipHash {
		// hash the resolved root node before storing
		h.hash(resolved, true)
	}

	h.newVer = newVer
	h.cacheTTL = t.cacheTTL
	h.skipHash = skipHash

	rn, err := h.store(resolved, db, nil)
	if err != nil {
		return err
	}
	t.root = rn
	t.cacheGen++
	return nil
}
