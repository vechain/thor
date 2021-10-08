// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"bytes"
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

const (
	HistSpace     = byte(0) // the space saves historical trie nodes.
	DedupedSpace  = byte(1) // the space saves deduped trie nodes.
	LeafBankSpace = byte(2) // the space for leaf bank.
)

var log = log15.New("pkg", "muxdb.trie")

// Backend is the backend of the trie.
type Backend struct {
	Store            kv.Store
	Cache            *Cache
	HistPtnFactor    PartitionFactor
	DedupedPtnFactor PartitionFactor
}

// Trie is the managed trie.
type Trie struct {
	back        *Backend
	name        string
	secure      bool
	root        thor.Bytes32
	commitNum   uint32
	init        func() (*trie.ExtendedTrie, error)
	dirty       bool
	noFillCache bool
	fastLeafGet func(nodeCommitNum uint32) (*trie.Leaf, error)
}

// New creates a managed trie.
func New(
	back *Backend,
	name string,
	secure bool,
	root thor.Bytes32,
	commitNum uint32,
	cachedNodeTTL int,
) *Trie {
	t := &Trie{
		back:      back,
		name:      name,
		secure:    secure,
		root:      root,
		commitNum: commitNum,
	}

	var (
		ext *trie.ExtendedTrie
		err error
	)
	t.init = func() (*trie.ExtendedTrie, error) {
		if ext != nil || err != nil {
			return ext, err
		}
		if rootNode := t.back.Cache.GetRootNode(name, root, commitNum); rootNode != nil {
			ext = trie.NewExtendedCached(rootNode, t.newDatabase())
		} else {
			ext, err = trie.NewExtended(root, commitNum, t.newDatabase())
		}
		if ext != nil {
			ext.SetCachedNodeTTL(cachedNodeTTL)
		}
		return ext, err
	}
	return t
}

// Name returns the trie name.
func (t *Trie) Name() string {
	return t.name
}

// CommitNum returns the current commit number.
func (t *Trie) CommitNum() uint32 {
	return t.commitNum
}

// newDatabase creates a database instance for low-level trie construction.
func (t *Trie) newDatabase() trie.Database {
	var (
		histBkt    = kv.Bucket(string(HistSpace) + t.name)
		dedupedBkt = kv.Bucket(string(DedupedSpace) + t.name)
		dedupedKey DedupedNodeKey
	)

	return &struct {
		trie.DatabaseReader
		trie.DatabaseWriter
		trie.DatabaseKeyEncoder
	}{
		kv.GetFunc(func(key []byte) (blob []byte, err error) {
			// get from cache
			if blob = t.back.Cache.GetNodeBlob(t.name, key, t.noFillCache); len(blob) > 0 {
				return
			}
			defer func() {
				if err == nil && !t.noFillCache {
					t.back.Cache.AddNodeBlob(t.name, key, blob, false)
				}
			}()

			// if cache missed, try fast leaf get
			if t.fastLeafGet != nil {
				if leaf, err := t.fastLeafGet(HistNodeKey(key).CommitNum()); err != nil {
					return nil, err
				} else if leaf != nil {
					// good, leaf got. returns a special error to short-circuit further node lookups.
					return nil, &leafAvailable{leaf}
				}
			}

			// have to lookup nodes
			err = t.back.Store.Snapshot(func(getter kv.Getter) error {
				// Get node from hist space first, then from deduped space.
				// Don't change the order, or the trie might be broken when during pruning.
				if data, err := histBkt.NewGetter(getter).Get(key); err != nil {
					if !t.back.Store.IsNotFound(err) {
						return err
					}
					// not found in hist space, fallback to deduped space
				} else {
					blob = data
					return nil
				}

				// get from deduped space
				dKey := dedupedKey.FromHistKey(t.back.DedupedPtnFactor, HistNodeKey(key))
				if data, err := dedupedBkt.NewGetter(getter).Get(dKey); err != nil {
					return err
				} else {
					// the deduped node key uses path as db key.
					// to ensure the node is correct, we need to verify the node hash.
					if ok, err := verifyNodeHash(data, HistNodeKey(key).Hash()); err != nil {
						return err
					} else if !ok {
						return errors.New("node hash checksum error")
					}
					blob = data
					return nil
				}
			})
			return
		}),
		nil, // nil is ok
		func() databaseKeyEncodeFunc {
			var histKey HistNodeKey
			return func(hash []byte, commitNum uint32, path []byte) []byte {
				return histKey.Encode(t.back.HistPtnFactor, hash, commitNum, path)
			}
		}(),
	}
}

// Copy make a copy of this trie.
func (t *Trie) Copy() *Trie {
	ext, err := t.init()
	cpy := *t
	if ext != nil {
		extCpy := trie.NewExtendedCached(ext.RootNode(), cpy.newDatabase())
		extCpy.SetCachedNodeTTL(ext.CachedNodeTTL())
		cpy.init = func() (*trie.ExtendedTrie, error) {
			return extCpy, nil
		}
		cpy.noFillCache = false
	} else {
		cpy.init = func() (*trie.ExtendedTrie, error) { return nil, err }
	}
	return &cpy
}

// CacheRoot caches the current root node.
// Returns true if it is properly cached.
func (t *Trie) CacheRoot() bool {
	ext, err := t.init()
	if err != nil {
		return false
	}
	return t.back.Cache.AddRootNode(t.name, ext.RootNode())
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	ext, err := t.init()
	if err != nil {
		return nil, nil, err
	}
	if t.secure {
		h := hasherPool.Get().(*hasher)
		defer hasherPool.Put(h)
		key = h.Hash(key)
	}
	return ext.Get(key)
}

// FastGet uses a fast way to query the value for key stored in the trie.
// See VIP-212 for detail.
func (t *Trie) FastGet(key []byte, leafBank *LeafBank, steadyCommitNum uint32) ([]byte, []byte, error) {
	ext, err := t.init()
	if err != nil {
		return nil, nil, err
	}
	if t.secure {
		h := hasherPool.Get().(*hasher)
		defer hasherPool.Put(h)
		key = h.Hash(key)
	}
	// setup fast leaf getter
	var (
		leaf          *trie.Leaf
		leafCommitNum uint32
		gotLeaf       bool
	)

	t.fastLeafGet = func(nodeCommitNum uint32) (*trie.Leaf, error) {
		if nodeCommitNum > steadyCommitNum {
			return nil, nil
		}
		if !gotLeaf {
			var err error
			if leaf, leafCommitNum, err = leafBank.Lookup(t.name, key); err != nil {
				return nil, err
			}
			gotLeaf = true
		}
		if leaf == nil {
			return nil, nil
		}

		// see VIP-212 for detail.
		if len(leaf.Value) > 0 {
			if nodeCommitNum <= leafCommitNum && leafCommitNum <= steadyCommitNum {
				// good, that's the leaf!
				return leaf, nil
			}
		} else {
			// enough for empty leaf
			if nodeCommitNum <= leafCommitNum {
				return leaf, nil
			}
		}
		return nil, nil
	}
	defer func() { t.fastLeafGet = nil }()

	val, meta, err := ext.Get(key)
	if err != nil {
		if miss, ok := err.(*trie.MissingNodeError); ok {
			if la, ok := miss.Err.(*leafAvailable); ok {
				return la.Value, la.Meta, nil
			}
		}
		return nil, nil, err
	}
	return val, meta, nil
}

// Update associates key with value in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value bytes must not be modified by the caller while they are
// stored in the trie.
func (t *Trie) Update(key, val, meta []byte) error {
	ext, err := t.init()
	if err != nil {
		return err
	}
	t.dirty = true
	if t.secure {
		h := hasherPool.Get().(*hasher)
		defer hasherPool.Put(h)
		key = h.Hash(key)
	}
	return ext.Update(key, val, meta)
}

// Hash returns the root hash of the trie.
func (t *Trie) Hash() thor.Bytes32 {
	ext, err := t.init()
	if err != nil {
		return t.root
	}
	return ext.Hash()
}

// Commit writes all nodes to the trie database.
func (t *Trie) Commit(commitNum uint32) (root thor.Bytes32, err error) {
	ext, err := t.init()
	if err != nil {
		return
	}
	defer func() {
		if err == nil {
			t.root = root
			t.commitNum = commitNum
			t.dirty = false
		}
	}()

	err = t.back.Store.Batch(func(putter kv.Putter) error {
		var (
			histPutter = kv.Bucket(string(HistSpace) + t.name).NewPutter(putter)
			cErr       error
		)
		// commit the trie
		root, cErr = ext.CommitTo(&struct {
			trie.DatabaseWriter
			trie.DatabaseKeyEncoder
		}{
			kv.PutFunc(func(key, blob []byte) error {
				if err := histPutter.Put(key, blob); err != nil {
					return err
				}
				if !t.noFillCache {
					t.back.Cache.AddNodeBlob(t.name, key, blob, true)
				}
				return nil
			}),
			func() databaseKeyEncodeFunc {
				var histKey HistNodeKey
				return func(hash []byte, commitNum uint32, path []byte) []byte {
					return histKey.Encode(t.back.HistPtnFactor, hash, commitNum, path)
				}
			}(),
		}, commitNum)
		return cErr
	})
	return
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key
func (t *Trie) NodeIterator(start []byte, minCommitNum uint32) trie.NodeIterator {
	ext, err := t.init()
	if err != nil {
		return &errorIterator{err}
	}
	return ext.NodeIterator(start, minCommitNum)
}

// SetNoFillCache enable or disable cache filling.
func (t *Trie) SetNoFillCache(b bool) {
	t.noFillCache = b
}

// Prune prunes redundant nodes in the range of [baseCommitNum, thisCommitNum].
func (t *Trie) Prune(ctx context.Context, baseCommitNum uint32) error {

	if t.dirty {
		return errors.New("dirty trie")
	}

	// debounced context checker
	checkContext := func() func() error {
		count := 0
		return func() error {
			count++
			if count%500 == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
			return nil
		}
	}()

	return t.back.Store.Batch(func(putter kv.Putter) error {
		var (
			dedupedKey    DedupedNodeKey
			dedupedPutter = kv.Bucket(string(DedupedSpace) + t.name).NewPutter(putter)
			rootCommitNum = t.commitNum
			histBkt       = kv.Bucket(string(HistSpace) + t.name)
			histPutter    = histBkt.NewPutter(putter)
			histRng       = kv.Range{
				Start: appendUint32(nil, t.back.HistPtnFactor.Which(baseCommitNum)),
				Limit: appendUint32(nil, t.back.HistPtnFactor.Which(rootCommitNum)+1),
			}
			it = t.NodeIterator(nil, baseCommitNum)
		)
		for it.Next(true) {
			if err := checkContext(); err != nil {
				return err
			}
			// save all new nodes into deduped space
			if err := it.Node(true, func(blob []byte) error {
				key := dedupedKey.Encode(t.back.DedupedPtnFactor, it.CommitNum(), it.Path())
				return dedupedPutter.Put(key, blob)
			}); err != nil {
				return err
			}
		}
		if err := it.Error(); err != nil {
			return err
		}

		// clean hist nodes
		return histBkt.NewStore(t.back.Store).Iterate(histRng, func(pair kv.Pair) (bool, error) {
			if err := checkContext(); err != nil {
				return false, err
			}
			if cn := HistNodeKey(pair.Key()).CommitNum(); cn >= baseCommitNum && cn <= rootCommitNum {
				if err := histPutter.Delete(pair.Key()); err != nil {
					return false, err
				}
			}
			return true, nil
		})
	})
}

func verifyNodeHash(blob, expectedHash []byte) (bool, error) {
	// strip the trailing
	_, _, trailing, err := rlp.Split(blob)
	if err != nil {
		return false, err
	}

	node := blob[:len(blob)-len(trailing)]

	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	return bytes.Equal(expectedHash, h.Hash(node)), nil
}

// individual functions of trie backend interface.
type (
	databaseKeyEncodeFunc func(hash []byte, commitNum uint32, path []byte) []byte
)

func (f databaseKeyEncodeFunc) Encode(hash []byte, commitNum uint32, path []byte) []byte {
	return f(hash, commitNum, path)
}

var (
	_ trie.DatabaseKeyEncoder = databaseKeyEncodeFunc(nil)
)

// leafAvailable is a special error type to short circuit trie get method.
type leafAvailable struct {
	*trie.Leaf
}

func (*leafAvailable) Error() string {
	return "leaf available"
}

// errorIterator an iterator always in error state.
type errorIterator struct {
	err error
}

func (i *errorIterator) Next(bool) bool                           { return false }
func (i *errorIterator) Error() error                             { return i.err }
func (i *errorIterator) Hash() thor.Bytes32                       { return thor.Bytes32{} }
func (i *errorIterator) Node(bool, func(blob []byte) error) error { return i.err }
func (i *errorIterator) CommitNum() uint32                        { return 0 }
func (i *errorIterator) Parent() thor.Bytes32                     { return thor.Bytes32{} }
func (i *errorIterator) Path() []byte                             { return nil }
func (i *errorIterator) Leaf() *trie.Leaf                         { return nil }
func (i *errorIterator) LeafKey() []byte                          { return nil }
func (i *errorIterator) LeafProof() [][]byte                      { return nil }
