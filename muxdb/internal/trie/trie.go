// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

const (
	NodeSpace         = 0 // the space for regular trie nodes.
	OverflowNodeSpace = 1 // the space for trie nodes whose path length > 15.
	PrefilterSpace    = 2 // the space for prefilter keys.
)

// individual functions of trie backend interface.
type (
	databaseKeyEncodeFunc func(hash []byte, commitNum uint32, path []byte) []byte
)

func (f databaseKeyEncodeFunc) Encode(hash []byte, commitNum uint32, path []byte) []byte {
	return f(hash, commitNum, path)
}

var (
	_                 trie.DatabaseKeyEncoder = databaseKeyEncodeFunc(nil)
	errFalsePrefilter                         = errors.New("false prefilter")
)

// Trie is the managed trie.
type Trie struct {
	store       kv.Store
	name        string
	cache       *Cache
	secure      bool
	noFillCache bool
	pfkeys      map[uint64]struct{} // records updated keys needed by pre-filter
	curPfKey    uint64
	ext         *trie.ExtendedTrie
	err         error
}

// New creates a managed trie.
func New(
	store kv.Store,
	name string,
	cache *Cache,
	secure bool,
	root thor.Bytes32,
	commitNum uint32,
) *Trie {
	t := &Trie{
		store:  store,
		name:   name,
		cache:  cache,
		secure: secure,
	}

	if rootNode := cache.GetRootNode(name, root, commitNum); rootNode != nil {
		t.ext = trie.NewExtendedCached(*rootNode, t.newDatabase())
	} else {
		t.ext, t.err = trie.NewExtended(root, commitNum, t.newDatabase())
	}
	return t
}

// Name returns trie name.
func (t *Trie) Name() string {
	return t.name
}

func (t *Trie) newDatabase() trie.Database {
	return &struct {
		trie.DatabaseReader
		trie.DatabaseWriter
		trie.DatabaseKeyEncoder
	}{
		kv.GetFunc(func(key []byte) (blob []byte, err error) {
			// get from cache
			pathLen := nodeKey(key).Path().Len()
			if blob = t.cache.GetNodeBlob(key, pathLen, t.noFillCache); len(blob) != 0 {
				return
			}
			// prefilter when node missing in cache
			if t.curPfKey != 0 {
				if has, err := t.prefilter(t.curPfKey); err != nil {
					return nil, err
				} else {
					t.curPfKey = 0
					// short circuit if the prefilter tells not exist.
					if !has {
						return nil, errFalsePrefilter
					}
				}
			}
			// get from store
			if blob, err = t.store.Get(key); err != nil {
				return
			}
			if !t.noFillCache {
				t.cache.AddNodeBlob(key, blob, pathLen)
			}
			return
		}),
		nil,
		databaseKeyEncodeFunc(newNodeKey(t.name).Encode),
	}
}

// Copy make a copy of this trie.
func (t *Trie) Copy() *Trie {
	cpy := *t
	if t.ext != nil {
		cpy.ext = trie.NewExtendedCached(t.ext.RootNode(), cpy.newDatabase())
		cpy.curPfKey = 0
		cpy.noFillCache = false
		if len(t.pfkeys) > 0 {
			cpy.pfkeys = make(map[uint64]struct{})
			for k, v := range t.pfkeys {
				cpy.pfkeys[k] = v
			}
		} else {
			cpy.pfkeys = nil
		}
	}
	return &cpy
}

// Cache caches the current root node.
// Returns true if it is properly cached.
func (t *Trie) Cache() bool {
	if t.ext == nil {
		return false
	}

	rn := t.ext.RootNode()
	if rn.Dirty() {
		return false
	}

	root := rn.Hash()
	if root.IsZero() {
		return false
	}

	t.cache.AddRootNode(t.name, root, rn.CommitNum(), &rn)
	return true
}

// hashKey hashes the raw key into secure key and prefilter key.
func (t *Trie) hashKey(key []byte) ([]byte, uint64) {
	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	// produces secure key
	h.Reset()
	h.Write(key)
	key = h.Sum(nil)

	// produces pre-filter key
	h.Write([]byte(t.name))
	h.tmp = h.Sum(h.tmp[:0])

	return key, binary.BigEndian.Uint64(h.tmp)
}

// prefilter is a false-positive filter to check if the given key exists.
func (t *Trie) prefilter(pfkey uint64) (bool, error) {
	// check the local map
	if _, has := t.pfkeys[pfkey]; has {
		return true, nil
	}
	var b = [9]byte{PrefilterSpace}
	binary.BigEndian.PutUint64(b[1:], pfkey)
	// check the global cache
	if t.cache.HasPrefilterKey(b[1:]) {
		return true, nil
	}
	// check the store
	has, err := t.store.Has(b[:])
	if err != nil {
		return false, err
	}
	if has {
		t.cache.AddPrefilterKey(b[1:])
	}
	return has, nil
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	if t.err != nil {
		return nil, nil, t.err
	}
	if t.secure {
		key, t.curPfKey = t.hashKey(key)
		defer func() { t.curPfKey = 0 }()
	}
	val, meta, err := t.ext.Get(key)
	if err == nil {
		return val, meta, nil
	}

	if miss, ok := err.(*trie.MissingNodeError); ok && miss.Err == errFalsePrefilter {
		return nil, nil, nil
	}
	return nil, nil, err
}

// Update associates key with value in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value bytes must not be modified by the caller while they are
// stored in the trie.
func (t *Trie) Update(key, val, meta []byte) error {
	if t.err != nil {
		return t.err
	}
	if t.secure {
		var pfkey uint64
		key, pfkey = t.hashKey(key)

		if t.pfkeys == nil {
			t.pfkeys = make(map[uint64]struct{})
		}
		t.pfkeys[pfkey] = struct{}{}
	}
	return t.ext.Update(key, val, meta)
}

// Hash returns the root hash of the trie.
func (t *Trie) Hash() thor.Bytes32 {
	if t.ext != nil {
		return t.ext.Hash()
	}
	return thor.Bytes32{}
}

// Commit writes all nodes to the trie's database.
func (t *Trie) Commit(commitNum uint32) (root thor.Bytes32, err error) {
	if err = t.err; err != nil {
		return
	}

	err = t.store.Batch(func(putter kv.Putter) error {
		// flush prefilter keys
		b := [9]byte{PrefilterSpace}
		for pfkey := range t.pfkeys {
			binary.BigEndian.PutUint64(b[1:], pfkey)
			if err := putter.Put(b[:], nil); err != nil {
				return err
			}
			t.cache.AddPrefilterKey(b[1:])
		}

		var (
			db = &struct {
				trie.DatabaseWriter
				trie.DatabaseKeyEncoder
			}{
				kv.PutFunc(func(key, blob []byte) error {
					if err := putter.Put(key, blob); err != nil {
						return err
					}
					if !t.noFillCache {
						t.cache.AddNodeBlob(key, blob, nodeKey(key).Path().Len())
					}
					return nil
				}),
				databaseKeyEncodeFunc(newNodeKey(t.name).Encode),
			}
			err error
		)
		// commit the trie
		root, err = t.ext.CommitTo(db, commitNum)
		return err
	})
	if err == nil {
		t.pfkeys = nil
	}
	return
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key
func (t *Trie) NodeIterator(start []byte, filter trie.NodeFilter) trie.NodeIterator {
	if t.err != nil {
		return &errorIterator{t.err}
	}
	return t.ext.NodeIterator(start, filter)
}

// pruneRange cleans up nodes in the path range according to the mask, and returns the count of deleted nodes.
func (t *Trie) pruneRange(ctx context.Context, mask map[path64]uint32, start, limit path64, delete kv.DeleteFunc) (int, error) {
	if len(mask) == 0 {
		return 0, nil
	}

	var (
		rng          kv.Range
		innerErr     error
		nDeleted     int
		iterateCount int
	)

	// convert the path range to kv range.
	rng.Start = make([]byte, 1+len(t.name)+8)
	rng.Start[0] = NodeSpace
	copy(rng.Start[1:], t.name)
	binary.BigEndian.PutUint64(rng.Start[len(rng.Start)-8:], uint64(start))

	if limit == 0 { // to the end
		prefix := append([]byte{NodeSpace}, t.name...)
		rng.Limit = util.BytesPrefix(prefix).Limit
	} else {
		rng.Limit = append([]byte(nil), rng.Start...)
		binary.BigEndian.PutUint64(rng.Limit[len(rng.Limit)-8:], uint64(limit))
	}

	err := t.store.Iterate(rng, func(pair kv.Pair) bool {
		iterateCount++
		if iterateCount%5000 == 0 {
			select {
			case <-ctx.Done():
				innerErr = ctx.Err()
				return false
			default:
			}
		}

		nk := nodeKey(pair.Key())

		// delete the node if its commit number smaller than found in mask with the same path.
		// skip nodes has 0 commit number to keep genesis state.
		if cn := nk.CommitNum(); cn > 0 && cn < mask[nk.Path()] {
			if innerErr = delete(pair.Key()); innerErr != nil {
				return false
			}
			nDeleted++
		}
		return true
	})
	if innerErr != nil {
		err = innerErr
	}
	return nDeleted, err
}

// Prune prunes trie nodes.
func (t *Trie) Prune(ctx context.Context, baseCommitNum uint32) (int, error) {
	t.noFillCache = true
	defer func() { t.noFillCache = false }()

	var (
		it = t.NodeIterator(nil, func(path []byte, commitNum uint32) bool {
			return commitNum > baseCommitNum
		})
		mask         map[path64]uint32 // path => commitNum
		pCur, pStart path64
		nDeleted     int
		iterateCount int
	)

	return nDeleted, t.store.Batch(func(putter kv.Putter) error {
		for it.Next(true) {
			iterateCount++
			if iterateCount%1000 == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
			path, cn := it.Path(), it.CommitNum()
			if len(path) > 15 || it.Hash().IsZero() {
				continue
			}

			pLast := pCur
			// if the path spans a large distance, start range prune.
			if pCur = newPath64(path); ((pCur - pLast) >> 48) > 0 {
				n, err := t.pruneRange(ctx, mask, pStart, pLast+1, putter.Delete)
				if err != nil {
					return err
				}
				nDeleted += n
				mask = nil
			}

			// restart to build mask
			if mask == nil {
				mask = make(map[path64]uint32)
				pStart = pCur
			}
			mask[pCur] = cn
			if it.Short() {
				sk := it.ShortKey()
				// skip last path element which is term or part of child
				for i := 0; i < len(sk)-1; i++ {
					pCur = pCur.Append(sk[i])
					mask[pCur] = cn
				}
			}
		}
		if err := it.Error(); err != nil {
			return err
		}

		// remained
		n, err := t.pruneRange(ctx, mask, pStart, pCur+1, putter.Delete)
		if err != nil {
			return err
		}
		nDeleted += n
		return nil
	})
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
func (i *errorIterator) Leaf() *trie.Leaf     { return nil }
func (i *errorIterator) LeafKey() []byte      { return nil }
func (i *errorIterator) LeafProof() [][]byte  { return nil }
