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

package trie

import (
	"bytes"
	"errors"
)

// Iterator is a key-value trie iterator that traverses a Trie.
type Iterator struct {
	nodeIt NodeIterator

	Key   []byte // Current data key on which the iterator is positioned on
	Value []byte // Current data value on which the iterator is positioned on
	Meta  []byte // Current metadata on which the iterator is positioned on
	Err   error
}

// NewIterator creates a new key-value iterator from a node iterator.
// Note that the value returned by the iterator is raw. If the content is encoded
// (e.g. storage value is RLP-encoded), it's caller's duty to decode it.
func NewIterator(it NodeIterator) *Iterator {
	return &Iterator{
		nodeIt: it,
	}
}

// Next moves the iterator forward one key-value entry.
func (it *Iterator) Next() bool {
	for it.nodeIt.Next(true) {
		if leaf := it.nodeIt.Leaf(); leaf != nil {
			it.Key = it.nodeIt.LeafKey()
			it.Value = leaf.Value
			it.Meta = leaf.Meta
			return true
		}
	}
	it.Key = nil
	it.Value = nil
	it.Meta = nil
	it.Err = it.nodeIt.Error()
	return false
}

// Leaf presents the leaf node.
type Leaf struct {
	Value []byte
	Meta  []byte
}

// NodeIterator is an iterator to traverse the trie pre-order.
type NodeIterator interface {
	// Next moves the iterator to the next node. If the parameter is false, any child
	// nodes will be skipped.
	Next(bool) bool

	// Error returns the error status of the iterator.
	Error() error

	// Blob returns the encoded blob and version num of the current node.
	// If the current node is not stored as standalone node, the returned blob has zero length.
	Blob() ([]byte, Version, error)

	// Path returns the hex-encoded path to the current node.
	// Callers must not retain references to the return value after calling Next.
	// For leaf nodes, the last element of the path is the 'terminator symbol' 0x10.
	Path() []byte

	// Leaf returns the leaf node if the current node is a leaf node, or nil returned.
	Leaf() *Leaf

	// LeafKey returns the key of the leaf. The method panics if the iterator is not
	// positioned at a leaf. Callers must not retain references to the value after
	// calling Next.
	LeafKey() []byte
}

// nodeIteratorState represents the iteration state at one particular node of the
// trie, which can be resumed at a later invocation.
type nodeIteratorState struct {
	node    node   // Trie node being iterated
	index   int    // Child to be processed next
	pathlen int    // Length of the path to this node
	blob    []byte // Encoded blob of the node
}

type nodeIterator struct {
	trie   *Trie                // Trie being iterated
	stack  []*nodeIteratorState // Hierarchy of trie nodes persisting the iteration state
	path   []byte               // Path to the current node
	err    error                // Failure set in case of an internal error in the iterator
	minVer Version              // Skips nodes whose version lower than minVer
}

// errIteratorEnd is stored in nodeIterator.err when iteration is done.
var errIteratorEnd = errors.New("end of iteration")

// seekError is stored in nodeIterator.err if the initial seek has failed.
type seekError struct {
	key []byte
	err error
}

func (e seekError) Error() string {
	return "seek error: " + e.err.Error()
}

func newNodeIterator(trie *Trie, start []byte, min Version) NodeIterator {
	it := &nodeIterator{
		trie:   trie,
		minVer: min,
	}
	it.err = it.seek(start)
	return it
}

func (it *nodeIterator) Blob() (blob []byte, ver Version, err error) {
	if len(it.stack) == 0 {
		return nil, Version{}, nil
	}
	st := it.stack[len(it.stack)-1]
	ref, _, dirty := st.node.cache()
	// dirty node has no blob
	if dirty {
		return
	}

	if len(st.blob) > 0 {
		blob, ver = st.blob, ref.ver
		return
	}

	// load from db
	if blob, err = it.trie.db.Get(it.path, ref.ver); err != nil {
		return
	}
	st.blob, ver = blob, ref.ver
	return
}

func (it *nodeIterator) Leaf() *Leaf {
	if len(it.stack) > 0 {
		if vn, ok := it.stack[len(it.stack)-1].node.(*valueNode); ok {
			return &Leaf{Value: vn.val, Meta: vn.meta}
		}
	}
	return nil
}

func (it *nodeIterator) LeafKey() []byte {
	if len(it.stack) > 0 {
		if _, ok := it.stack[len(it.stack)-1].node.(*valueNode); ok {
			return hexToKeybytes(it.path)
		}
	}
	panic("not at leaf")
}

func (it *nodeIterator) Path() []byte {
	return it.path
}

func (it *nodeIterator) Error() error {
	if it.err == errIteratorEnd {
		return nil
	}
	if seek, ok := it.err.(seekError); ok {
		return seek.err
	}
	return it.err
}

// Next moves the iterator to the next node, returning whether there are any
// further nodes. In case of an internal error this method returns false and
// sets the Error field to the encountered failure. If `descend` is false,
// skips iterating over any subnodes of the current node.
func (it *nodeIterator) Next(descend bool) bool {
	if it.err == errIteratorEnd {
		return false
	}
	if seek, ok := it.err.(seekError); ok {
		if it.err = it.seek(seek.key); it.err != nil {
			return false
		}
	}
	// Otherwise step forward with the iterator and report any errors.
	state, parentIndex, path, err := it.peek(descend)
	it.err = err
	if it.err != nil {
		return false
	}
	it.push(state, parentIndex, path)
	return true
}

func (it *nodeIterator) seek(prefix []byte) error {
	// The path we're looking for is the hex encoded key without terminator.
	key := keybytesToHex(prefix)
	key = key[:len(key)-1]
	// Move forward until we're just before the closest match to key.
	for {
		state, parentIndex, path, err := it.peek(bytes.HasPrefix(key, it.path))
		if err == errIteratorEnd {
			return errIteratorEnd
		} else if err != nil {
			return seekError{prefix, err}
		} else if bytes.Compare(path, key) >= 0 {
			return nil
		}
		it.push(state, parentIndex, path)
	}
}

// peek creates the next state of the iterator.
func (it *nodeIterator) peek(descend bool) (*nodeIteratorState, *int, []byte, error) {
	if len(it.stack) == 0 {
		n := it.trie.root
		if n == nil {
			return nil, nil, nil, errIteratorEnd
		}
		if ref, _, dirty := n.cache(); !dirty {
			if ref.ver.Compare(it.minVer) < 0 {
				return nil, nil, nil, errIteratorEnd
			}
		}
		// Initialize the iterator if we've just started.
		state := &nodeIteratorState{node: it.trie.root, index: -1}
		if err := state.resolve(it.trie, nil); err != nil {
			return nil, nil, nil, err
		}
		return state, nil, nil, nil
	}
	if !descend {
		// If we're skipping children, pop the current node first
		it.pop()
	}

	// Continue iteration to the next child
	for len(it.stack) > 0 {
		parent := it.stack[len(it.stack)-1]
		state, path, ok := it.nextChild(parent)
		if ok {
			if err := state.resolve(it.trie, path); err != nil {
				return parent, &parent.index, path, err
			}
			return state, &parent.index, path, nil
		}
		// No more child nodes, move back up.
		it.pop()
	}
	return nil, nil, nil, errIteratorEnd
}

func (st *nodeIteratorState) resolve(tr *Trie, path []byte) error {
	if ref, ok := st.node.(*refNode); ok {
		blob, err := tr.db.Get(path, ref.ver)
		if err != nil {
			return &MissingNodeError{Ref: *ref, Path: path, Err: err}
		}
		st.blob = blob
		st.node = mustDecodeNode(ref, blob, 0)
	}
	return nil
}

func (it *nodeIterator) nextChild(parent *nodeIteratorState) (*nodeIteratorState, []byte, bool) {
	switch node := parent.node.(type) {
	case *fullNode:
		// Full node, move to the first non-nil child.
		for i := parent.index + 1; i < len(node.children); i++ {
			if child := node.children[i]; child != nil {
				if ref, _, dirty := child.cache(); !dirty {
					if ref.ver.Compare(it.minVer) < 0 {
						continue
					}
				}

				state := &nodeIteratorState{
					node:    child,
					index:   -1,
					pathlen: len(it.path),
				}

				parent.index = i - 1
				return state, append(it.path, byte(i)), true
			}
		}
	case *shortNode:
		// Short node, return the pointer singleton child
		if parent.index < 0 {
			if ref, _, dirty := node.child.cache(); !dirty {
				if ref.ver.Compare(it.minVer) < 0 {
					break
				}
			}

			state := &nodeIteratorState{
				node:    node.child,
				index:   -1,
				pathlen: len(it.path),
			}
			return state, append(it.path, node.key...), true
		}
	}
	return parent, it.path, false
}

func (it *nodeIterator) push(state *nodeIteratorState, parentIndex *int, path []byte) {
	it.path = path
	it.stack = append(it.stack, state)
	if parentIndex != nil {
		*parentIndex++
	}
}

func (it *nodeIterator) pop() {
	parent := it.stack[len(it.stack)-1]
	it.path = it.path[:parent.pathlen]
	it.stack = it.stack[:len(it.stack)-1]
}
