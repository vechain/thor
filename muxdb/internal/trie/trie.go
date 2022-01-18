// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"context"
	"encoding/binary"
	"errors"
	"math"

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
	root        thor.Bytes32
	commitNum   uint32
	distinctNum uint32
	init        func() (*trie.ExtendedTrie, error)
	dirty       bool
	bulk        kv.Bulk // pending ops
	noFillCache bool
	fastLeafGet func(nodeCommitNum uint32) (*trie.Leaf, error)
}

// New creates a managed trie.
func New(
	back *Backend,
	name string,
	root thor.Bytes32,
	commitNum uint32,
	distinctNum uint32,
) *Trie {
	t := &Trie{
		back:        back,
		name:        name,
		root:        root,
		commitNum:   commitNum,
		distinctNum: distinctNum,
	}

	var (
		ext *trie.ExtendedTrie
		err error
	)
	t.init = func() (*trie.ExtendedTrie, error) {
		if ext != nil || err != nil {
			return ext, err
		}
		if rootNode := t.back.Cache.GetRootNode(name, commitNum, distinctNum, t.noFillCache); rootNode != nil {
			ext = trie.NewExtendedCached(rootNode, t.newDatabase())
		} else {
			ext, err = trie.NewExtended(root, commitNum, distinctNum, t.newDatabase())
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

func (t *Trie) makeHistNodeKey(dst []byte, commitNum, distinctNum uint32, path []byte) []byte {
	dst = append(dst, t.back.HistSpace)                            // space
	dst = appendUint32(dst, t.back.HistPtnFactor.Which(commitNum)) // partition id
	dst = append(dst, t.name...)                                   // trie name
	dst = encodePath(dst, path)                                    // path
	dst = appendUint32(dst, commitNum)                             // commit num
	dst = appendUint32(dst, distinctNum)                           // distinct num
	return dst
}

func (t *Trie) makeDedupedNodeKey(dst []byte, commitNum uint32, path []byte) []byte {
	dst = append(dst, t.back.DedupedSpace)                            // space
	dst = appendUint32(dst, t.back.DedupedPtnFactor.Which(commitNum)) // partition id
	dst = append(dst, t.name...)                                      // trie name
	dst = encodePath(dst, path)                                       // path
	return dst
}

// newDatabase creates a database instance for low-level trie construction.
func (t *Trie) newDatabase() trie.Database {
	var (
		thisHash                       []byte
		thisCommitNum, thisDistinctNum uint32
		thisPath                       []byte
		keyBuf                         []byte
	)

	return &struct {
		trie.DatabaseReaderTo
		trie.DatabaseKeyEncoder
		trie.DatabaseReader
		trie.DatabaseWriter
	}{
		kv.GetToFunc(func(_ []byte, dst []byte) (blob []byte, err error) {
			// get from cache
			if blob = t.back.Cache.GetNodeBlob(t.name, thisCommitNum, thisDistinctNum, thisPath, t.noFillCache, dst); len(blob) > 0 {
				return
			}
			defer func() {
				if err == nil && !t.noFillCache {
					t.back.Cache.AddNodeBlob(t.name, thisCommitNum, thisDistinctNum, thisPath, blob, false)
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

			// Get node from hist space first, then from deduped space.
			// Don't change the order, or the trie might be broken after pruning.
			keyBuf = t.makeHistNodeKey(keyBuf[:0], thisCommitNum, thisDistinctNum, thisPath)
			if blob, err = snapshot.GetTo(keyBuf, dst); err != nil {
				if !snapshot.IsNotFound(err) {
					return
				}
				// not found in hist space, fallback to deduped space
				// get from deduped space
				keyBuf = t.makeDedupedNodeKey(keyBuf[:0], thisCommitNum, thisPath)
				if blob, err = snapshot.GetTo(keyBuf, dst); err != nil {
					return
				}
			}

			// to ensure the node is correct, we need to verify the node hash.
			// TODO: later can skip this step
			if ok, err := trie.VerifyNodeHash(blob[len(dst):], thisHash); err != nil {
				return nil, err
			} else if !ok {
				return nil, errors.New("node hash checksum error")
			}
			return
		}),
		databaseKeyEncodeFunc(func(hash []byte, commitNum, distinctNum uint32, path []byte) []byte {
			thisHash = hash
			thisCommitNum = commitNum
			thisDistinctNum = distinctNum
			thisPath = path
			return nil
		}),
		nil,
		nil,
	}
}

// Copy make a copy of this trie.
// It returns error if the trie is dirty.
func (t *Trie) Copy() (*Trie, error) {
	if t.dirty || t.bulk != nil {
		return nil, errors.New("dirty trie")
	}
	ext, err := t.init()
	cpy := *t
	if ext != nil {
		extCpy := trie.NewExtendedCached(ext.RootNode(), cpy.newDatabase())
		extCpy.SetCachedNodeTTL(cpy.back.CachedNodeTTL)
		cpy.init = func() (*trie.ExtendedTrie, error) {
			return extCpy, nil
		}
		cpy.noFillCache = false
	} else {
		cpy.init = func() (*trie.ExtendedTrie, error) { return nil, err }
	}
	return &cpy, nil
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	ext, err := t.init()
	if err != nil {
		return nil, nil, err
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

	// setup fast leaf getter
	var leafRec *LeafRecord
	t.fastLeafGet = func(nodeCommitNum uint32) (*trie.Leaf, error) {
		// short circuit if the node is too new
		if nodeCommitNum > steadyCommitNum {
			return nil, nil
		}
		if leafRec == nil {
			var err error
			if leafRec, err = t.back.LeafBank.Lookup(t.name, key); err != nil {
				return nil, err
			}
		}

		if leafRec.CommitNum == math.MaxUint32 {
			// ever seen but can't be located now (touched).
			return nil, nil
		}

		if nodeCommitNum <= leafRec.CommitNum {
			if leafRec.Leaf != nil {
				if leafRec.CommitNum <= steadyCommitNum {
					return leafRec.Leaf, nil
				}
			} else {
				// never seen till leafRec.CommitNum.
				return &trie.Leaf{}, nil
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
	if len(val) == 0 { // deletion
		if t.back.LeafBank != nil {
			if t.bulk == nil {
				t.bulk = t.back.Store.Bulk()
			}

			if err := t.back.LeafBank.Touch(t.bulk, t.name, key); err != nil {
				return err
			}
		}
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
func (t *Trie) Commit(newCommitNum, newDistinctNum uint32) (thor.Bytes32, error) {
	ext, err := t.init()
	if err != nil {
		return thor.Bytes32{}, err
	}

	var (
		thisCommitNum, thisDistinctNum uint32
		thisPath                       []byte
		bulk                           = t.bulk
		buf                            []byte
	)

	if bulk == nil {
		bulk = t.back.Store.Bulk()
	}

	// commit the trie
	newRoot, err := ext.CommitTo(&struct {
		trie.DatabaseWriter
		trie.DatabaseKeyEncoder
	}{
		kv.PutFunc(func(_, blob []byte) error {
			buf = t.makeHistNodeKey(buf[:0], thisCommitNum, thisDistinctNum, thisPath)
			if err := bulk.Put(buf, blob); err != nil {
				return err
			}
			if !t.noFillCache {
				t.back.Cache.AddNodeBlob(t.name, thisCommitNum, thisDistinctNum, thisPath, blob, true)
			}
			return nil
		}),
		databaseKeyEncodeFunc(func(hash []byte, commitNum, distinctNum uint32, path []byte) []byte {
			thisCommitNum = commitNum
			thisDistinctNum = distinctNum
			thisPath = path
			return nil
		}),
	}, newCommitNum, newDistinctNum)
	if err != nil {
		return thor.Bytes32{}, err
	}

	if err := bulk.Write(); err != nil {
		return thor.Bytes32{}, err
	}

	t.root = newRoot
	t.commitNum = newCommitNum
	t.distinctNum = newDistinctNum
	t.dirty = false
	t.bulk = nil
	if !t.noFillCache {
		t.back.Cache.AddRootNode(t.name, ext.RootNode())
	}
	return newRoot, nil
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key
func (t *Trie) NodeIterator(start []byte, baseCommitNum uint32) trie.NodeIterator {
	ext, err := t.init()
	if err != nil {
		return &errorIterator{err}
	}
	return ext.NodeIterator(start, baseCommitNum)
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
		checkContext = newContextChecker(ctx, 5000)
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

// DumpNodes dumps referenced nodes committed within [baseCommitNum, thisCommitNum], into the deduped space.
func (t *Trie) DumpNodes(ctx context.Context, baseCommitNum uint32, handleLeaf func(*trie.Leaf)) error {
	if t.dirty {
		return errors.New("dirty trie")
	}
	var (
		checkContext = newContextChecker(ctx, 5000)
		bulk         = t.back.Store.Bulk()
		iter         = t.NodeIterator(nil, baseCommitNum)
		buf          []byte
	)
	bulk.EnableAutoFlush()

	for iter.Next(true) {
		if err := checkContext(); err != nil {
			return err
		}

		if err := iter.Node(true, func(blob []byte) error {
			buf = t.makeDedupedNodeKey(buf[:0], iter.CommitNum(), iter.Path())
			return bulk.Put(buf, blob)
		}); err != nil {
			return err
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

// CleanHistory cleans history nodes within [startCommitNum, limitCommitNum).
func CleanHistory(ctx context.Context, back *Backend, startCommitNum, limitCommitNum uint32) error {
	if limitCommitNum == 0 {
		return nil
	}
	var (
		checkContext = newContextChecker(ctx, 5000)
		iter         = back.Store.Iterate(kv.Range{
			Start: appendUint32([]byte{back.HistSpace}, back.HistPtnFactor.Which(startCommitNum)),
			Limit: appendUint32([]byte{back.HistSpace}, back.HistPtnFactor.Which(limitCommitNum-1)+1),
		})
		bulk = back.Store.Bulk()
	)
	defer iter.Release()
	bulk.EnableAutoFlush()

	for iter.Next() {
		if err := checkContext(); err != nil {
			return err
		}
		key := iter.Key()
		// TODO: better way to extract commit number
		nodeCommitNum := binary.BigEndian.Uint32(key[len(key)-8:])
		if nodeCommitNum >= startCommitNum && nodeCommitNum < limitCommitNum {
			if err := bulk.Delete(key); err != nil {
				return err
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
	databaseKeyEncodeFunc func(hash []byte, commitNum, distinctNum uint32, path []byte) []byte
)

func (f databaseKeyEncodeFunc) Encode(hash []byte, commitNum, distinctNum uint32, path []byte) []byte {
	return f(hash, commitNum, distinctNum, path)
}

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
func (i *errorIterator) DistinctNum() uint32                      { return 0 }
func (i *errorIterator) Parent() thor.Bytes32                     { return thor.Bytes32{} }
func (i *errorIterator) Path() []byte                             { return nil }
func (i *errorIterator) Leaf() *trie.Leaf                         { return nil }
func (i *errorIterator) LeafKey() []byte                          { return nil }
func (i *errorIterator) LeafProof() [][]byte                      { return nil }
