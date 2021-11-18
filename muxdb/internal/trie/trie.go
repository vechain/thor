// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

var log = log15.New("pkg", "muxdb.trie")

// Backend is the backend of the trie.
type Backend struct {
	Store    kv.Store
	Cache    *Cache
	LeafBank *LeafBank
	HistSpace,
	DedupedSpace byte
	HistPtnFactor,
	DedupedPtnFactor PartitionFactor
	CachedNodeTTL int
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
		if rootNode := t.back.Cache.GetRootNode(name, root, commitNum, t.noFillCache); rootNode != nil {
			ext = trie.NewExtendedCached(rootNode, t.newDatabase())
		} else {
			ext, err = trie.NewExtended(root, commitNum, t.newDatabase())
		}
		if ext != nil {
			ext.SetCachedNodeTTL(t.back.CachedNodeTTL)
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

func (t *Trie) makeHistNodeKey(dst []byte, hash []byte, commitNum uint32, path []byte) []byte {
	dst = append(dst, t.back.HistSpace)                            // space
	dst = append(dst, t.name...)                                   // trie name
	dst = appendUint32(dst, t.back.HistPtnFactor.Which(commitNum)) // partition id
	dst = encodePath(dst, path)                                    // path
	dst = appendUint32(dst, commitNum)                             // commit num
	dst = append(dst, hash...)                                     // node hash
	return dst
}

func (t *Trie) makeDedupedNodeKey(dst []byte, commitNum uint32, path []byte) []byte {
	dst = append(dst, t.back.DedupedSpace)                            // space
	dst = append(dst, t.name...)                                      // trie name
	dst = appendUint32(dst, t.back.DedupedPtnFactor.Which(commitNum)) // partition id
	dst = encodePath(dst, path)                                       // path
	return dst
}

// newDatabase creates a database instance for low-level trie construction.
func (t *Trie) newDatabase() trie.Database {
	var (
		thisHash      []byte
		thisCommitNum uint32
		thisPath      []byte
	)

	return &struct {
		trie.DatabaseReader
		trie.DatabaseWriter
		trie.DatabaseKeyEncoder
	}{
		kv.GetFunc(func(_ []byte) (blob []byte, err error) {
			// get from cache
			if blob = t.back.Cache.GetNodeBlob(t.name, thisHash, thisCommitNum, thisPath, t.noFillCache); len(blob) > 0 {
				return
			}
			defer func() {
				if err == nil && !t.noFillCache {
					t.back.Cache.AddNodeBlob(t.name, thisHash, thisCommitNum, thisPath, blob, false)
				}
			}()

			// if cache missed, try fast leaf get
			if t.fastLeafGet != nil {
				if leaf, err := t.fastLeafGet(thisCommitNum); err != nil {
					return nil, err
				} else if leaf != nil {
					// good, leaf got. returns a special error to short-circuit further node lookups.
					return nil, &leafAvailable{leaf}
				}
			}

			// have to lookup nodes
			snapshot := t.back.Store.Snapshot()
			defer snapshot.Release()

			k := hasherPool.Get().(*hasher)
			defer hasherPool.Put(k)

			// Get node from hist space first, then from deduped space.
			// Don't change the order, or the trie might be broken after pruning.
			k.buf = t.makeHistNodeKey(k.buf[:0], thisHash, thisCommitNum, thisPath)
			if blob, err = snapshot.Get(k.buf); err != nil {
				if !snapshot.IsNotFound(err) {
					return
				}
				// not found in hist space, fallback to deduped space
				// get from deduped space
				k.buf = t.makeDedupedNodeKey(k.buf[:0], thisCommitNum, thisPath)
				if blob, err = snapshot.Get(k.buf); err != nil {
					return
				}
				// the deduped node key uses path as db key.
				// to ensure the node is correct, we need to verify the node hash.
				if ok, err := verifyNodeHash(blob, thisHash); err != nil {
					return nil, err
				} else if !ok {
					return nil, errors.New("node hash checksum error")
				}
			}
			return
		}),
		nil, // nil is ok
		databaseKeyEncodeFunc(func(hash []byte, commitNum uint32, path []byte) []byte {
			thisHash = hash
			thisCommitNum = commitNum
			thisPath = path
			return nil
		}),
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
func (t *Trie) FastGet(key []byte, steadyCommitNum uint32) ([]byte, []byte, error) {
	if t.back.LeafBank == nil {
		return t.Get(key)
	}
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
			if leaf, leafCommitNum, err = t.back.LeafBank.Lookup(t.name, key); err != nil {
				return nil, err
			}
			gotLeaf = true
		}
		// not ready
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
			// got empty leaf
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
func (t *Trie) Commit(newCommitNum uint32) (thor.Bytes32, error) {
	ext, err := t.init()
	if err != nil {
		return thor.Bytes32{}, err
	}

	var (
		thisHash []byte
		thisPath []byte
		bulk     = t.back.Store.Bulk()
		k        = hasherPool.Get().(*hasher)
	)
	defer hasherPool.Put(k)

	// commit the trie
	newRoot, err := ext.CommitTo(&struct {
		trie.DatabaseWriter
		trie.DatabaseKeyEncoder
	}{
		kv.PutFunc(func(_, blob []byte) error {
			k.buf = t.makeHistNodeKey(k.buf[:0], thisHash, newCommitNum, thisPath)
			if err := bulk.Put(k.buf, blob); err != nil {
				return err
			}
			if !t.noFillCache {
				t.back.Cache.AddNodeBlob(t.name, thisHash, newCommitNum, thisPath, blob, true)
			}
			return nil
		}),
		databaseKeyEncodeFunc(func(hash []byte, commitNum uint32, path []byte) []byte {
			thisHash = hash
			thisPath = path
			return nil
		}),
	}, newCommitNum)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if err := bulk.Flush(); err != nil {
		return thor.Bytes32{}, err
	}

	t.root = newRoot
	t.commitNum = newCommitNum
	t.dirty = false
	if !t.noFillCache {
		t.back.Cache.AddRootNode(t.name, ext.RootNode())
	}
	return newRoot, nil
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

// DumpLeaves dumps leaves in the range of [baseCommitNum, thisCommitNum] into leaf bank.
// transform transforms leaves before passing into leaf bank.
func (t *Trie) DumpLeaves(ctx context.Context, baseCommitNum uint32, transform func(*trie.Leaf) *trie.Leaf) error {
	if t.dirty {
		return errors.New("dirty trie")
	}
	if t.back.LeafBank == nil {
		return errors.New("nil leaf bank")
	}

	var (
		checkContext = newContextChecker(ctx, 500)
		leafUpdater  = t.back.LeafBank.NewUpdater(t.name, t.commitNum)
		iter         = t.NodeIterator(nil, baseCommitNum)
	)

	for iter.Next(true) {
		if err := checkContext(); err != nil {
			return err
		}

		if leaf := iter.Leaf(); leaf != nil {
			if err := leafUpdater.Update(iter.LeafKey(), transform(leaf)); err != nil {
				return err
			}
		}
	}
	if err := iter.Error(); err != nil {
		return err
	}
	return leafUpdater.Commit()
}

// DumpAndCleanNodes dumps historical nodes into deduped space and cleanup redundant historical nodes.
func (t *Trie) DumpAndCleanNodes(ctx context.Context, baseCommitNum uint32) error {
	if t.dirty {
		return errors.New("dirty trie")
	}
	var (
		checkContext = newContextChecker(ctx, 500)
		bulk         = t.back.Store.Bulk()
		k            = hasherPool.Get().(*hasher)
	)
	defer hasherPool.Put(k)

	{
		// save all new nodes into deduped space
		iter := t.NodeIterator(nil, baseCommitNum)
		for iter.Next(true) {
			if err := checkContext(); err != nil {
				return err
			}

			if err := iter.Node(true, func(blob []byte) error {
				k.buf = t.makeDedupedNodeKey(k.buf[:0], iter.CommitNum(), iter.Path())
				return bulk.Put(k.buf, blob)
			}); err != nil {
				return err
			}
		}
		if err := iter.Error(); err != nil {
			return err
		}
	}

	{
		// then clean up redundant hist nodes
		pidStart, pidLimit := t.back.HistPtnFactor.Which(baseCommitNum), t.back.HistPtnFactor.Which(t.commitNum)+1
		iter := t.back.Store.Iterate(kv.Range{
			Start: appendUint32(append([]byte{t.back.HistSpace}, t.name...), pidStart),
			Limit: appendUint32(append([]byte{t.back.HistSpace}, t.name...), pidLimit),
		})
		defer iter.Release()
		for iter.Next() {
			histKey := iter.Key()
			// TODO: better way to extract commit number
			nodeCommitNum := binary.BigEndian.Uint32(histKey[len(histKey)-4-32:])
			if nodeCommitNum <= t.commitNum {
				if err := bulk.Delete(histKey); err != nil {
					return err
				}
			}
		}
		if err := iter.Error(); err != nil {
			return err
		}
	}
	return bulk.Flush()
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

// verifyNodeHash verifies the hash of the node blob (trailing excluded).
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

// individual functions of trie database interface.
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
