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
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

// makeTestTrie create a sample test trie to test node-wise reconstruction.
func makeTestTrie() (*memdb, *Trie, map[string][]byte) {
	// Create an empty trie
	db := newMemDatabase()
	trie := New(Root{}, db)

	// Fill it with some arbitrary data
	content := make(map[string][]byte)
	for i := range byte(255) {
		// Map the same data under multiple keys
		key, val := common.LeftPadBytes([]byte{1, i}, 32), []byte{i}
		content[string(key)] = val
		trie.Update(key, val, nil)

		key, val = common.LeftPadBytes([]byte{2, i}, 32), []byte{i}
		content[string(key)] = val
		trie.Update(key, val, nil)

		// Add some other data to inflate the trie
		for j := byte(3); j < 13; j++ {
			key, val = common.LeftPadBytes([]byte{j, i}, 32), []byte{j, i}
			content[string(key)] = val
			trie.Update(key, val, nil)
		}
	}

	trie.Commit(db, Version{Major: 1}, false)

	// Return the generated trie
	return db, trie, content
}

func TestIterator(t *testing.T) {
	trie := new(Trie)
	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"dog", "puppy"},
		{"somethingveryoddindeedthis is", "myothernodedata"},
	}
	all := make(map[string]string)
	for _, val := range vals {
		all[val.k] = val.v
		trie.Update([]byte(val.k), []byte(val.v), nil)
	}
	db := newMemDatabase()
	trie.Commit(db, Version{}, false)

	found := make(map[string]string)
	it := NewIterator(trie.NodeIterator(nil, Version{}))
	for it.Next() {
		found[string(it.Key)] = string(it.Value)
	}

	for k, v := range all {
		if found[k] != v {
			t.Errorf("iterator value mismatch for %s: got %q want %q", k, found[k], v)
		}
	}
}

type kv struct {
	k, v []byte
	t    bool
}

func TestIteratorLargeData(t *testing.T) {
	trie := new(Trie)
	vals := make(map[string]*kv)

	for i := range byte(255) {
		value := &kv{common.LeftPadBytes([]byte{i}, 32), []byte{i}, false}
		value2 := &kv{common.LeftPadBytes([]byte{10, i}, 32), []byte{i}, false}
		trie.Update(value.k, value.v, nil)
		trie.Update(value2.k, value2.v, nil)
		vals[string(value.k)] = value
		vals[string(value2.k)] = value2
	}

	it := NewIterator(trie.NodeIterator(nil, Version{}))
	for it.Next() {
		vals[string(it.Key)].t = true
	}

	var untouched []*kv
	for _, value := range vals {
		if !value.t {
			untouched = append(untouched, value)
		}
	}

	if len(untouched) > 0 {
		t.Errorf("Missed %d nodes", len(untouched))
		for _, value := range untouched {
			t.Error(value)
		}
	}
}

// Tests that the node iterator indeed walks over the entire database contents.
func TestNodeIteratorCoverage(t *testing.T) {
	// Create some arbitrary test trie to iterate
	db, trie, _ := makeTestTrie()

	// Gather all the node storage key found by the iterator
	keys := make(map[string]struct{})
	for it := trie.NodeIterator(nil, Version{}); it.Next(true); {
		blob, ver, _ := it.Blob()
		if len(blob) > 0 {
			keys[string(makeKey(it.Path(), ver))] = struct{}{}
		}
	}
	// Cross check the hashes and the database itself
	for key := range keys {
		if _, err := db.db.Get([]byte(key)); err != nil {
			t.Errorf("failed to retrieve reported node %x: %v", key, err)
		}
	}
	for _, key := range db.db.Keys() {
		if _, ok := keys[string(key)]; !ok {
			t.Errorf("state entry not reported %x", key)
		}
	}
}

type kvs struct{ k, v string }

var testdata1 = []kvs{
	{"barb", "ba"},
	{"bard", "bc"},
	{"bars", "bb"},
	{"bar", "b"},
	{"fab", "z"},
	{"food", "ab"},
	{"foos", "aa"},
	{"foo", "a"},
}

var testdata2 = []kvs{
	{"aardvark", "c"},
	{"bar", "b"},
	{"barb", "bd"},
	{"bars", "be"},
	{"fab", "z"},
	{"foo", "a"},
	{"foos", "aa"},
	{"food", "ab"},
	{"jars", "d"},
}

func TestIteratorSeek(t *testing.T) {
	trie := new(Trie)
	for _, val := range testdata1 {
		trie.Update([]byte(val.k), []byte(val.v), nil)
	}

	// Seek to the middle.
	it := NewIterator(trie.NodeIterator([]byte("fab"), Version{}))
	if err := checkIteratorOrder(testdata1[4:], it); err != nil {
		t.Fatal(err)
	}

	// Seek to a non-existent key.
	it = NewIterator(trie.NodeIterator([]byte("barc"), Version{}))
	if err := checkIteratorOrder(testdata1[1:], it); err != nil {
		t.Fatal(err)
	}

	// Seek beyond the end.
	it = NewIterator(trie.NodeIterator([]byte("z"), Version{}))
	if err := checkIteratorOrder(nil, it); err != nil {
		t.Fatal(err)
	}
}

func checkIteratorOrder(want []kvs, it *Iterator) error {
	for it.Next() {
		if len(want) == 0 {
			return fmt.Errorf("didn't expect any more values, got key %q", it.Key)
		}
		if !bytes.Equal(it.Key, []byte(want[0].k)) {
			return fmt.Errorf("wrong key: got %q, want %q", it.Key, want[0].k)
		}
		want = want[1:]
	}
	if len(want) > 0 {
		return fmt.Errorf("iterator ended early, want key %q", want[0])
	}
	return nil
}

func TestIteratorNoDups(t *testing.T) {
	var tr Trie
	for _, val := range testdata1 {
		tr.Update([]byte(val.k), []byte(val.v), nil)
	}
	checkIteratorNoDups(t, tr.NodeIterator(nil, Version{}), nil)
}

// This test checks that nodeIterator.Next can be retried after inserting missing trie nodes.
func TestIteratorContinueAfterError(t *testing.T) {
	db := newMemDatabase()
	ver := Version{}
	tr := New(Root{}, db)
	for _, val := range testdata1 {
		tr.Update([]byte(val.k), []byte(val.v), nil)
	}
	ver.Major++
	tr.Commit(db, ver, false)
	wantNodeCount := checkIteratorNoDups(t, tr.NodeIterator(nil, Version{}), nil)
	keys := db.db.Keys()
	t.Log("node count", wantNodeCount)

	for range 20 {
		// Create trie that will load all nodes from DB.
		tr := New(Root{tr.Hash(), ver}, db)

		// Remove a random node from the database. It can't be the root node
		// because that one is already loaded.
		var rkey []byte
		for {
			//#nosec G404
			if rkey = keys[rand.N(len(keys))]; !bytes.Equal(rkey, makeKey(nil, ver)) {
				break
			}
		}
		rval, _ := db.db.Get(rkey)
		db.db.Delete(rkey)

		// Iterate until the error is hit.
		seen := make(map[string]bool)
		it := tr.NodeIterator(nil, Version{})
		checkIteratorNoDups(t, it, seen)
		missing, ok := it.Error().(*MissingNodeError)
		if !ok || !bytes.Equal(makeKey(missing.Path, ver), rkey) {
			t.Fatal("didn't hit missing node, got", it.Error())
		}

		// Add the node back and continue iteration.
		db.db.Put(rkey, rval)
		checkIteratorNoDups(t, it, seen)
		if it.Error() != nil {
			t.Fatal("unexpected error", it.Error())
		}
		if len(seen) != wantNodeCount {
			t.Fatal("wrong node iteration count, got", len(seen), "want", wantNodeCount)
		}
	}
}

func checkIteratorNoDups(t *testing.T, it NodeIterator, seen map[string]bool) int {
	if seen == nil {
		seen = make(map[string]bool)
	}
	for it.Next(true) {
		if seen[string(it.Path())] {
			t.Fatalf("iterator visited node path %x twice", it.Path())
		}
		seen[string(it.Path())] = true
	}
	return len(seen)
}

func TestIteratorNodeFilter(t *testing.T) {
	db := newMemDatabase()
	ver := Version{}
	tr := New(Root{}, db)
	for _, val := range testdata1 {
		tr.Update([]byte(val.k), []byte(val.v), nil)
	}
	ver.Major++
	tr.Commit(db, ver, false)
	for _, val := range testdata2 {
		tr.Update([]byte(val.k), []byte(val.v), nil)
	}
	ver.Major++
	tr.Commit(db, ver, false)
	root2 := tr.Hash()

	tr = New(Root{root2, Version{Major: 2}}, db)

	it := tr.NodeIterator(nil, Version{Major: 1})

	for it.Next(true) {
		if blob, ver, _ := it.Blob(); len(blob) > 0 {
			assert.True(t, ver.Major >= 1)
		}
	}

	it = tr.NodeIterator(nil, Version{Major: 2})

	for it.Next(true) {
		if blob, ver, _ := it.Blob(); len(blob) > 0 {
			assert.True(t, ver.Major >= 2)
		}
	}
}
