// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import "github.com/vechain/thor/thor"

// ExtendedTrie is an extended Merkle Patricia Trie which supports commit-number
// and leaf metadata.
type ExtendedTrie struct {
	init         func() (*Trie, error)
	originalRoot thor.Bytes32
}

// NewExtended creates an extended trie.
func NewExtended(root thor.Bytes32, commitNum uint32, db Database) *ExtendedTrie {
	isRootEmpty := (root == thor.Bytes32{}) || root == emptyRoot
	if !isRootEmpty && db == nil {
		panic("trie.NewExtended: cannot use existing root without a database")
	}
	var (
		trie *Trie
		err  error
	)
	return &ExtendedTrie{
		func() (*Trie, error) {
			if err != nil {
				return nil, err
			}
			if trie != nil {
				return trie, nil
			}

			trie = &Trie{db: db}
			if !isRootEmpty {
				if trie.root, _, err = trie.resolveHash(&hashNode{root[:], commitNum}, nil); err != nil {
					return nil, err
				}
			}
			return trie, nil
		},
		root,
	}
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key.
func (e *ExtendedTrie) NodeIterator(start []byte, filter NodeFilter) NodeIterator {
	t, err := e.init()
	if err != nil {
		return &errorIterator{err}
	}
	return newNodeIterator(t, start, filter)
}

// Get returns the value and metadata for key stored in the trie.
// The value and meta bytes must not be modified by the caller.
// If a node was not found in the database, a MissingNodeError is returned.
func (e *ExtendedTrie) Get(key []byte) (val, meta []byte, err error) {
	t, err := e.init()
	if err != nil {
		return nil, nil, err
	}
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
	t, err := e.init()
	if err != nil {
		return err
	}

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
	t, err := e.init()
	if err != nil {
		return e.originalRoot
	}
	return t.Hash()
}

// Commit writes all nodes with the given commit number to the trie's database.
//
// Committing flushes nodes from memory.
// Subsequent Get calls will load nodes from the database.
func (e *ExtendedTrie) Commit(commitNum uint32) (root thor.Bytes32, err error) {
	t, err := e.init()
	if err != nil {
		return thor.Bytes32{}, err
	}
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
	t, err := e.init()
	if err != nil {
		return thor.Bytes32{}, err
	}
	hash, cached, err := e.hashRoot(db, commitNum)
	if err != nil {
		return thor.Bytes32{}, err
	}
	t.root = cached
	return thor.BytesToBytes32(hash.(*hashNode).hash), nil
}

func (e *ExtendedTrie) hashRoot(db DatabaseWriter, commitNum uint32) (node, node, error) {
	t, err := e.init()
	if err != nil {
		return nil, nil, err
	}
	if t.root == nil {
		return &hashNode{hash: emptyRoot.Bytes()}, nil, nil
	}
	h := newHasher()
	defer returnHasherToPool(h)
	return h.hash(t.root, db, nil, true, &commitNum)
}

// errorIterator an iterator always in error state.
type errorIterator struct {
	err error
}

func (i *errorIterator) Next(bool) bool       { return false }
func (i *errorIterator) Error() error         { return i.err }
func (i *errorIterator) Hash() thor.Bytes32   { return thor.Bytes32{} }
func (i *errorIterator) CommitNum() uint32    { return 0 }
func (i *errorIterator) Short() bool          { return false }
func (i *errorIterator) ShortKey() []byte     { return nil }
func (i *errorIterator) Parent() thor.Bytes32 { return thor.Bytes32{} }
func (i *errorIterator) Path() []byte         { return nil }
func (i *errorIterator) Leaf() *Leaf          { return nil }
func (i *errorIterator) LeafKey() []byte      { return nil }
func (i *errorIterator) LeafProof() [][]byte  { return nil }
