// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import "github.com/vechain/thor/thor"

// ExtendedTrie is an extended Merkle Patricia Trie which supports commit-number
// and leaf metadata.
type ExtendedTrie struct {
	trie          Trie
	cachedNodeTTL int
}

// Node contains the internal node object.
type Node struct{ node node }

// Dirty returns if the node is dirty.
func (n *Node) Dirty() bool {
	if n.node != nil {
		_, dirty := n.node.cache()
		return dirty
	}
	return true
}

// Hash returns the hash of the node. It returns zero hash in case of embedded or not computed.
func (n *Node) Hash() (hash thor.Bytes32) {
	if n.node != nil {
		if h, _ := n.node.cache(); h != nil {
			copy(hash[:], h.hash)
		}
	}
	return
}

// CommitNum returns the node's commit number. 0 is returned if the node is dirty.
func (n *Node) CommitNum() uint32 {
	if n.node != nil {
		return n.node.commitNum()
	}
	return 0
}

// NewExtended creates an extended trie.
func NewExtended(root thor.Bytes32, commitNum uint32, db Database) (*ExtendedTrie, error) {
	isRootEmpty := (root == thor.Bytes32{}) || root == emptyRoot
	if !isRootEmpty && db == nil {
		panic("trie.NewExtended: cannot use existing root without a database")
	}
	ext := ExtendedTrie{Trie{db: db}, 0}
	if !isRootEmpty {
		rootnode, _, err := ext.trie.resolveHash(&hashNode{root[:], commitNum}, nil)
		if err != nil {
			return nil, err
		}
		ext.trie.root = rootnode
	}
	return &ext, nil
}

// NewExtendedCached creates an extended trie with the given root node.
func NewExtendedCached(rootNode *Node, db Database) *ExtendedTrie {
	return &ExtendedTrie{Trie{root: rootNode.node, db: db}, 0}
}

// SetCacheTTL sets life time of a cached node. The life time is equivalent to
// the differenc of commit number.
func (e *ExtendedTrie) SetCachedNodeTTL(ttl int) {
	e.cachedNodeTTL = ttl
}

// CachedNodeTTL returns the life time of a cached node.
func (e *ExtendedTrie) CachedNodeTTL() int {
	return e.cachedNodeTTL
}

// RootNode returns the current root node.
func (e *ExtendedTrie) RootNode() *Node {
	return &Node{e.trie.root}
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key. It filters out nodes that have commit number smaller than
// minCommitNum.
func (e *ExtendedTrie) NodeIterator(start []byte, minCommitNum uint32) NodeIterator {
	t := &e.trie
	return newNodeIterator(t, start, minCommitNum)
}

// Get returns the value and metadata for key stored in the trie.
// The value and meta bytes must not be modified by the caller.
// If a node was not found in the database, a MissingNodeError is returned.
func (e *ExtendedTrie) Get(key []byte) (val, meta []byte, err error) {
	t := &e.trie

	value, newroot, didResolve, err := t.tryGet(t.root, keybytesToHex(key), 0)
	if err != nil {
		return nil, nil, err
	}

	if didResolve {
		t.root = newroot
	}
	if value != nil {
		return value.value, value.meta, nil
	}
	return nil, nil, nil
}

// Update associates key with value and metadata in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value and meta bytes must not be modified by the caller while they are
// stored in the trie.
//
// If a node was not found in the database, a MissingNodeError is returned.
func (e *ExtendedTrie) Update(key, value, meta []byte) error {
	t := &e.trie

	k := keybytesToHex(key)
	if len(value) != 0 {
		_, n, err := t.insert(t.root, nil, k, &valueNode{value: value, meta: meta})
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

// Hash returns the root hash of the trie. It does not write to the
// database and can be used even if the trie doesn't have one.
func (e *ExtendedTrie) Hash() thor.Bytes32 {
	t := &e.trie
	return t.Hash()
}

// Commit writes all nodes with the given commit number to the trie's database.
//
// Committing flushes nodes from memory.
// Subsequent Get calls will load nodes from the database.
func (e *ExtendedTrie) Commit(commitNum uint32) (root thor.Bytes32, err error) {
	t := &e.trie
	if t.db == nil {
		panic("Commit called on trie with nil database")
	}
	return e.CommitTo(t.db, commitNum)
}

// CommitTo writes all nodes with the given commit number to the given database.
//
// Committing flushes nodes from memory. Subsequent Get calls will
// load nodes from the trie's database. Calling code must ensure that
// the changes made to db are written back to the trie's attached
// database before using the trie.
func (e *ExtendedTrie) CommitTo(db DatabaseWriter, commitNum uint32) (root thor.Bytes32, err error) {
	t := &e.trie
	hash, cached, err := e.hashRoot(db, commitNum)
	if err != nil {
		return thor.Bytes32{}, err
	}
	t.root = cached
	return thor.BytesToBytes32(hash.(*hashNode).hash), nil
}

func (e *ExtendedTrie) hashRoot(db DatabaseWriter, commitNum uint32) (node, node, error) {
	t := &e.trie
	if t.root == nil {
		return &hashNode{hash: emptyRoot.Bytes()}, nil, nil
	}
	h := newHasher(e.cachedNodeTTL)
	defer returnHasherToPool(h)
	return h.hash(t.root, db, nil, true, &commitNum)
}
