// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"context"
	"errors"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

// Trie is the managed trie.
type Trie struct {
	name        string
	back        *backend
	trie        *trie.Trie
	noFillCache bool
}

// newTrie creates a managed trie.
func newTrie(
	name string,
	back *backend,
	root trie.Root,
) *Trie {
	t := &Trie{
		name: name,
		back: back,
	}

	if rn := back.Cache.GetRootNode(name, root.Ver); rn != nil {
		t.trie = trie.FromRootNode(rn, t.newDatabaseReader())
	} else {
		t.trie = trie.New(root, t.newDatabaseReader())
	}
	t.trie.SetCacheTTL(back.CachedNodeTTL)
	return t
}

// newDatabase creates a database instance for low-level trie construction.
func (t *Trie) newDatabaseReader() trie.DatabaseReader {
	var keyBuf []byte

	return &struct {
		trie.DatabaseReader
	}{
		databaseGetFunc(func(path []byte, ver trie.Version) (blob []byte, err error) {
			// get from cache
			if blob = t.back.Cache.GetNodeBlob(&keyBuf, t.name, path, ver, t.noFillCache); len(blob) > 0 {
				return
			}
			defer func() {
				if err == nil && !t.noFillCache {
					t.back.Cache.AddNodeBlob(&keyBuf, t.name, path, ver, blob, false)
				}
			}()

			// query in db
			snapshot := t.back.Store.Snapshot()
			defer snapshot.Release()

			// get from hist space first
			keyBuf = t.back.AppendHistNodeKey(keyBuf[:0], t.name, path, ver)
			if blob, err = snapshot.Get(keyBuf); err != nil {
				if !snapshot.IsNotFound(err) {
					return
				}
			} else {
				// found in hist space
				return
			}

			// enforce root node to be only fetched from hist space
			// to prevent accessing root node of a revision that has been pruned
			if len(path) == 0 {
				return nil, errors.New("not found")
			}

			// then from deduped space
			keyBuf = t.back.AppendDedupedNodeKey(keyBuf[:0], t.name, path, ver)
			return snapshot.Get(keyBuf)
		}),
	}
}

// Copy make a copy of this trie.
func (t *Trie) Copy() *Trie {
	cpy := *t
	cpy.trie = trie.FromRootNode(t.trie.RootNode(), cpy.newDatabaseReader())
	cpy.trie.SetCacheTTL(t.back.CachedNodeTTL)
	return &cpy
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	return t.trie.Get(key)
}

// Update associates key with value in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value bytes must not be modified by the caller while they are
// stored in the trie.
func (t *Trie) Update(key, val, meta []byte) error {
	return t.trie.Update(key, val, meta)
}

// Hash returns the root hash of the trie.
func (t *Trie) Hash() thor.Bytes32 {
	return t.trie.Hash()
}

// Commit writes all nodes to the trie's database.
//
// Committing flushes nodes from memory.
// Subsequent Get calls will load nodes from the database.
// If skipHash is true, less disk space is taken up but crypto features of merkle trie lost.
func (t *Trie) Commit(newVer trie.Version, skipHash bool) error {
	var (
		bulk   = t.back.Store.Bulk()
		keyBuf []byte
	)

	db := &struct{ trie.DatabaseWriter }{
		databasePutFunc(func(path []byte, ver trie.Version, blob []byte) error {
			keyBuf = t.back.AppendHistNodeKey(keyBuf[:0], t.name, path, ver)
			if err := bulk.Put(keyBuf, blob); err != nil {
				return err
			}
			if !t.noFillCache {
				t.back.Cache.AddNodeBlob(&keyBuf, t.name, path, ver, blob, true)
			}
			return nil
		}),
	}

	if err := t.trie.Commit(db, newVer, skipHash); err != nil {
		return err
	}

	if err := bulk.Write(); err != nil {
		return err
	}

	if !t.noFillCache {
		t.back.Cache.AddRootNode(t.name, t.trie.RootNode())
	}
	return nil
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key
func (t *Trie) NodeIterator(start []byte, baseMajorVer uint32) trie.NodeIterator {
	return t.trie.NodeIterator(start, trie.Version{Major: baseMajorVer})
}

// SetNoFillCache enable or disable cache filling.
func (t *Trie) SetNoFillCache(b bool) {
	t.noFillCache = b
}

// Checkpoint transfers standalone nodes, whose major version within [baseMajorVer, thisMajorVer], into deduped space.
func (t *Trie) Checkpoint(ctx context.Context, baseMajorVer uint32, handleLeaf func(*trie.Leaf)) error {
	var (
		checkContext = newContextChecker(ctx, 5000)
		bulk         = t.back.Store.Bulk()
		iter         = t.NodeIterator(nil, baseMajorVer)
		keyBuf       []byte
	)
	bulk.EnableAutoFlush()

	for iter.Next(true) {
		if err := checkContext(); err != nil {
			return err
		}

		blob, ver, err := iter.Blob()
		if err != nil {
			return err
		}
		if len(blob) > 0 {
			keyBuf = t.back.AppendDedupedNodeKey(keyBuf[:0], t.name, iter.Path(), ver)
			if err := bulk.Put(keyBuf, blob); err != nil {
				return err
			}
		}
		if handleLeaf != nil {
			if leaf := iter.Leaf(); leaf != nil {
				handleLeaf(leaf)
			}
		}
	}
	if err := iter.Error(); err != nil {
		return err
	}
	return bulk.Write()
}

// individual functions of trie database interface.
type (
	databaseGetFunc func(path []byte, ver trie.Version) ([]byte, error)
	databasePutFunc func(path []byte, ver trie.Version, value []byte) error
)

func (f databaseGetFunc) Get(path []byte, ver trie.Version) ([]byte, error) {
	return f(path, ver)
}

func (f databasePutFunc) Put(path []byte, ver trie.Version, value []byte) error {
	return f(path, ver, value)
}

// newContextChecker creates a debounced context checker.
func newContextChecker(ctx context.Context, debounce int) func() error {
	count := 0
	return func() error {
		count++
		if count > debounce {
			count = 0
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		return nil
	}
}
