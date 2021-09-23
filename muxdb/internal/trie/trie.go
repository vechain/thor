// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"bytes"
	"context"
	"errors"
	"hash"

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

// Trie is the managed trie.
type Trie struct {
	store            kv.Store
	cache            *Cache
	leafBank         *LeafBank // might be nil
	histPtnFactor    PartitionFactor
	dedupedPtnFactor PartitionFactor

	name        string
	secure      bool
	root        thor.Bytes32
	commitNum   uint32
	dirty       bool
	ext         *trie.ExtendedTrie
	err         error
	noFillCache bool
	fastLeafGet func(nodeCommitNum uint32) (*trie.Leaf, error)
}

// New creates a managed trie.
func New(
	store kv.Store,
	cache *Cache,
	leafBank *LeafBank,
	histPtnFactor PartitionFactor,
	dedupedPtnFactor PartitionFactor,
	cachedNodeTTL int,
	name string,
	secure bool,
	root thor.Bytes32,
	commitNum uint32,
) *Trie {
	t := &Trie{
		store:            store,
		cache:            cache,
		leafBank:         leafBank,
		histPtnFactor:    histPtnFactor,
		dedupedPtnFactor: dedupedPtnFactor,

		name:      name,
		secure:    secure,
		root:      root,
		commitNum: commitNum,
	}

	if rootNode := cache.GetRootNode(name, root, commitNum); rootNode != nil {
		t.ext = trie.NewExtendedCached(rootNode, t.newDatabase())
	} else {
		t.ext, t.err = trie.NewExtended(root, commitNum, t.newDatabase())
	}
	if t.ext != nil {
		t.ext.SetCachedNodeTTL(cachedNodeTTL)
	}
	return t
}

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
			if blob = t.cache.GetNodeBlob(t.name, key, t.noFillCache); len(blob) > 0 {
				return
			}
			defer func() {
				if err == nil && !t.noFillCache {
					t.cache.AddNodeBlob(t.name, key, blob)
				}
			}()

			// fast leaf get
			if t.fastLeafGet != nil {
				if leaf, err := t.fastLeafGet(HistNodeKey(key).CommitNum()); err != nil {
					return nil, err
				} else if leaf != nil {
					// short circuit
					return nil, &leafAvailable{leaf}
				}
			}

			err = t.store.Snapshot(func(getter kv.Getter) error {
				// Get node from hist space first, then from deduped space.
				// Don't change the order, or the trie might be broken when pruner enabled!
				if data, err := histBkt.NewGetter(getter).Get(key); err != nil {
					if !t.store.IsNotFound(err) {
						return err
					}
					// not found in hist space, fallback to deduped space
				} else {
					blob = data
					return nil
				}

				// get from deduped space
				if data, err := dedupedBkt.NewGetter(getter).Get(dedupedKey.FromHistKey(t.dedupedPtnFactor, HistNodeKey(key))); err != nil {
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
				return histKey.Encode(t.histPtnFactor, hash, commitNum, path)
			}
		}(),
	}
}

// Copy make a copy of this trie.
func (t *Trie) Copy() *Trie {
	cpy := *t
	if t.ext != nil {
		cpy.ext = trie.NewExtendedCached(t.ext.RootNode(), cpy.newDatabase())
		cpy.ext.SetCachedNodeTTL(t.ext.CachedNodeTTL())
		cpy.noFillCache = false
	}
	return &cpy
}

// Cache caches the current root node.
// Returns true if it is properly cached.
func (t *Trie) Cache() bool {
	if t.err != nil {
		return false
	}
	return t.cache.AddRootNode(t.name, t.ext.RootNode())
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	if t.err != nil {
		return nil, nil, t.err
	}
	if t.secure {
		h := hasherPool.Get().(hash.Hash)
		defer hasherPool.Put(h)
		b := bufferPool.Get().(*buffer)
		defer bufferPool.Put(b)

		h.Reset()
		h.Write(key)
		b.b = h.Sum(b.b[:0])
		key = b.b
	}

	if t.leafBank != nil {
		// setup fast leaf getter
		var (
			leaf *Leaf
			err  error
		)
		t.fastLeafGet = func(nodeCommitNum uint32) (*trie.Leaf, error) {
			if leaf == nil && err == nil {
				leaf, err = t.leafBank.Lookup(t.name, key)
			}
			if err != nil {
				return nil, err
			}

			// see VIP-212 for detail.
			rootCommitNum := t.commitNum
			if rootCommitNum >= leaf.CommitNum && nodeCommitNum <= leaf.CommitNum {
				// good, that's the leaf!
				return &trie.Leaf{Value: leaf.Value, Meta: leaf.Meta}, nil
			}
			return nil, nil
		}
		defer func() { t.fastLeafGet = nil }()
	}

	val, meta, err := t.ext.Get(key)
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
	if t.err != nil {
		return t.err
	}
	t.dirty = true
	if t.secure {
		h := hasherPool.Get().(hash.Hash)
		defer hasherPool.Put(h)
		b := bufferPool.Get().(*buffer)
		defer bufferPool.Put(b)

		h.Reset()
		h.Write(key)
		b.b = h.Sum(b.b[:0])
		key = b.b
	}
	return t.ext.Update(key, val, meta)
}

// Hash returns the root hash of the trie.
func (t *Trie) Hash() thor.Bytes32 {
	if t.err != nil {
		return t.root
	}
	return t.ext.Hash()
}

// Commit writes all nodes to the trie database.
func (t *Trie) Commit(commitNum uint32) (root thor.Bytes32, err error) {
	if t.err != nil {
		err = t.err
		return
	}
	defer func() {
		if err == nil {
			t.root = root
			t.commitNum = commitNum
			t.dirty = false
		}
	}()

	err = t.store.Batch(func(putter kv.Putter) error {
		var (
			histPutter = kv.Bucket(string(HistSpace) + t.name).NewPutter(putter)
			cErr       error
		)
		// commit the trie
		root, cErr = t.ext.CommitTo(&struct {
			trie.DatabaseWriter
			trie.DatabaseKeyEncoder
		}{
			kv.PutFunc(func(key, blob []byte) error {
				if err := histPutter.Put(key, blob); err != nil {
					return err
				}
				if !t.noFillCache {
					t.cache.AddNodeBlob(t.name, key, blob)
				}
				return nil
			}),
			func() databaseKeyEncodeFunc {
				var histKey HistNodeKey
				return func(hash []byte, commitNum uint32, path []byte) []byte {
					return histKey.Encode(t.histPtnFactor, hash, commitNum, path)
				}
			}(),
		}, commitNum)
		return cErr
	})
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

// SetNoFillCache enable or disable cache filling.
func (t *Trie) SetNoFillCache(b bool) {
	t.noFillCache = b
}

// DisableFastLeaf disable fast leaf getter.
func (t *Trie) DisableFastLeaf() {
	t.leafBank = nil
}

// Optimize optimizes the trie.
// Trie optimization involves two things:
// 1. update the leaf bank (skipped if fast leaf disabled)
// 2. prune the trie (skipped if the prune flag is false)
func (t *Trie) Optimize(ctx context.Context, baseCommitNum uint32, prune bool) error {
	if t.err != nil {
		return t.err
	}

	if t.dirty {
		return errors.New("dirty trie")
	}

	// nothing to do
	if !prune && t.leafBank == nil {
		return nil
	}

	// disable cache filling before start node iteration
	t.SetNoFillCache(true)
	defer t.SetNoFillCache(false)

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

	return t.store.Batch(func(putter kv.Putter) error {
		var (
			rootCommitNum = t.commitNum
			// wrap LeafBank.Update
			updateLeafBank = func(batch func(save SaveLeaf) error) error {
				if t.leafBank != nil {
					return t.leafBank.Update(t.name, rootCommitNum, batch)
				}
				// no leafbank, pass a noop saveLeaf function
				return batch(func(key, value, meta []byte) error { return nil })
			}
		)

		if err := updateLeafBank(func(saveLeaf SaveLeaf) error {
			var (
				dedupedKey    DedupedNodeKey
				dedupedPutter = kv.Bucket(string(DedupedSpace) + t.name).NewPutter(putter)
				it            = t.NodeIterator(nil, func(path []byte, commitNum uint32) bool {
					return commitNum >= baseCommitNum
				})
			)
			for it.Next(true) {
				if err := checkContext(); err != nil {
					return err
				}
				// save all new leaves into leafbank
				if leaf := it.Leaf(); leaf != nil {
					if err := saveLeaf(it.LeafKey(), leaf.Value, leaf.Meta); err != nil {
						return err
					}
				}
				if prune {
					// save all new nodes into deduped space
					if err := it.Node(true, func(blob []byte) error {
						key := dedupedKey.Encode(t.dedupedPtnFactor, it.CommitNum(), it.Path())
						return dedupedPutter.Put(key, blob)
					}); err != nil {
						return err
					}
				}
			}
			return it.Error()
		}); err != nil {
			return err
		}

		if prune {
			var (
				bkt = kv.Bucket(string(HistSpace) + t.name)
				rng = kv.Range{
					Start: appendUint32(nil, t.histPtnFactor.Which(baseCommitNum)),
					Limit: appendUint32(nil, t.histPtnFactor.Which(rootCommitNum)+1),
				}
			)

			histPutter := bkt.NewPutter(putter)
			// clean hist nodes
			if err := bkt.NewStore(t.store).Iterate(rng, func(pair kv.Pair) (bool, error) {
				if err := checkContext(); err != nil {
					return false, err
				}
				if cn := HistNodeKey(pair.Key()).CommitNum(); cn >= baseCommitNum && cn <= rootCommitNum {
					if err := histPutter.Delete(pair.Key()); err != nil {
						return false, err
					}
				}
				return true, nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func verifyNodeHash(blob, expectedHash []byte) (bool, error) {
	// strip the trailing
	_, _, trailing, err := rlp.Split(blob)
	if err != nil {
		return false, err
	}

	node := blob[:len(blob)-len(trailing)]

	h := hasherPool.Get().(hash.Hash)
	defer hasherPool.Put(h)
	b := bufferPool.Get().(*buffer)
	defer bufferPool.Put(b)

	h.Reset()
	h.Write(node)
	b.b = h.Sum(b.b[:0])

	return bytes.Equal(expectedHash, b.b), nil
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
