// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import "github.com/vechain/thor/thor"

// ExtendedTrie is an extended Merkle Patricia Trie which supports nodes sequence number
// and leaf metadata.
type ExtendedTrie struct {
	trie      Trie
	nonCrypto bool
}

// Node contains the internal node object.
type Node struct {
	node     node
	cacheGen uint16
}

// Dirty returns if the node is dirty.
func (n Node) Dirty() bool {
	if n.node != nil {
		_, dirty, _ := n.node.cache()
		return dirty
	}
	return true
}

// Hash returns the hash of the node. It returns zero hash in case of embedded or not computed.
func (n Node) Hash() (hash thor.Bytes32) {
	if n.node != nil {
		if h, _, _ := n.node.cache(); h != nil {
			return h.Hash
		}
	}
	return
}

// SeqNum returns the node's sequence number. 0 is returned if the node is dirty.
func (n Node) SeqNum() uint64 {
	if n.node != nil {
		return n.node.seqNum()
	}
	return 0
}

// NewExtended creates an extended trie.
func NewExtended(root thor.Bytes32, seq uint64, db Database, nonCrypto bool) *ExtendedTrie {
	ext := &ExtendedTrie{trie: Trie{db: db}, nonCrypto: nonCrypto}
	if (root != thor.Bytes32{}) && root != emptyRoot {
		if db == nil {
			panic("trie.NewExtended: cannot use existing root without a database")
		}
		ext.trie.root = &hashNode{Hash: root, seq: seq}
	}
	return ext
}

// IsNonCrypto returns whether the trie is a non-crypto trie.
func (e *ExtendedTrie) IsNonCrypto() bool {
	return e.nonCrypto
}

// NewExtendedCached creates an extended trie with the given root node.
func NewExtendedCached(rootNode Node, db Database, nonCrypto bool) *ExtendedTrie {
	return &ExtendedTrie{trie: Trie{root: rootNode.node, db: db, cacheGen: rootNode.cacheGen}, nonCrypto: nonCrypto}
}

// SetCacheTTL sets life time of a cached node.
func (e *ExtendedTrie) SetCacheTTL(ttl uint16) {
	e.trie.cacheTTL = ttl
}

// CacheTTL returns the life time of a cached node.
func (e *ExtendedTrie) CacheTTL() uint16 {
	return e.trie.cacheTTL
}

// RootNode returns the current root node.
func (e *ExtendedTrie) RootNode() Node {
	return Node{e.trie.root, e.trie.cacheGen}
}

// SetRootNode replace the root node with the given one.
func (e *ExtendedTrie) SetRootNode(root Node) {
	e.trie.root = root.node
	e.trie.cacheGen = root.cacheGen
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key. It filters out nodes satisfy the filter.
func (e *ExtendedTrie) NodeIterator(start []byte, filter func(seq uint64) bool) NodeIterator {
	t := &e.trie
	return newNodeIterator(t, start, filter, true, e.nonCrypto)
}

// Get returns the value and metadata for key stored in the trie.
// The value and meta bytes must not be modified by the caller.
// If a node was not found in the database, a MissingNodeError is returned.
func (e *ExtendedTrie) Get(key []byte) (val, meta []byte, err error) {
	t := &e.trie

	value, newroot, err := t.tryGet(t.root, keybytesToHex(key), 0)
	if t.root != newroot {
		t.root = newroot
	}
	if err != nil {
		return nil, nil, err
	}

	if value != nil {
		return value.Value, value.meta, nil
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
		_, n, err := t.insert(t.root, nil, k, &valueNode{Value: value, meta: meta})
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

// Commit writes all nodes with the given sequence number to the trie's database.
//
// Committing flushes nodes from memory.
// Subsequent Get calls will load nodes from the database.
func (e *ExtendedTrie) Commit(seq uint64) (root thor.Bytes32, err error) {
	t := &e.trie
	if t.db == nil {
		panic("Commit called on trie with nil database")
	}
	return e.CommitTo(t.db, seq)
}

// CommitTo writes all nodes with the given sequence number to the given database.
//
// Committing flushes nodes from memory. Subsequent Get calls will
// load nodes from the trie's database. Calling code must ensure that
// the changes made to db are written back to the trie's attached
// database before using the trie.
func (e *ExtendedTrie) CommitTo(db DatabaseWriter, seq uint64) (root thor.Bytes32, err error) {
	t := &e.trie
	// ext trie always stores the root node even not changed. so here have to
	// resolve it (since ext trie lazily resolve the root node when initializing).
	if root, ok := t.root.(*hashNode); ok {
		rootnode, err := t.resolveHash(root, nil)
		if err != nil {
			return thor.Bytes32{}, err
		}
		t.root = rootnode
	}
	hash, cached, err := e.hashRoot(db, seq)
	if err != nil {
		return thor.Bytes32{}, err
	}
	t.root = cached
	t.cacheGen++
	return hash.(*hashNode).Hash, nil
}

func (e *ExtendedTrie) hashRoot(db DatabaseWriter, seq uint64) (node, node, error) {
	t := &e.trie
	if t.root == nil {
		return &hashNode{Hash: emptyRoot}, nil, nil
	}
	h := newHasherExtended(t.cacheGen, t.cacheTTL, seq, e.nonCrypto)
	defer returnHasherToPool(h)
	return h.hash(t.root, db, nil, true)
}
