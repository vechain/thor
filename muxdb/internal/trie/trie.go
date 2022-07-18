// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"context"
	"encoding/binary"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
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
	DedupedPtnFactor uint32
	CachedNodeTTL uint16
}

// sequence helps convert sequence number from/to commitNum & distinctNum.
type sequence uint64

func makeSequence(commitNum, distinctNum uint32) sequence {
	return sequence(commitNum) | (sequence(distinctNum) << 32)
}

func (s sequence) CommitNum() uint32   { return uint32(s) }
func (s sequence) DistinctNum() uint32 { return uint32(s >> 32) }

// Trie is the managed trie.
type Trie struct {
	back *Backend
	name string
	ext  *trie.ExtendedTrie

	dirty       bool
	deletions   []string
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
	nonCrypto bool,
) *Trie {
	t := &Trie{
		back: back,
		name: name,
	}

	seq := makeSequence(commitNum, distinctNum)
	if rootNode, ok := back.Cache.GetRootNode(name, uint64(seq), false); ok {
		t.ext = trie.NewExtendedCached(rootNode, t.newDatabase(), nonCrypto)
	} else {
		t.ext = trie.NewExtended(root, uint64(seq), t.newDatabase(), nonCrypto)
	}
	t.ext.SetCacheTTL(t.back.CachedNodeTTL)
	return t
}

// Name returns the trie name.
func (t *Trie) Name() string {
	return t.name
}

func (t *Trie) makeHistNodeKey(dst []byte, seq sequence, path []byte) []byte {
	commitNum, distinctNum := seq.CommitNum(), seq.DistinctNum()
	dst = append(dst, t.back.HistSpace)                     // space
	dst = appendUint32(dst, commitNum/t.back.HistPtnFactor) // partition id
	dst = append(dst, t.name...)                            // trie name
	dst = encodePath(dst, path)                             // path
	dst = appendUint32(dst, commitNum%t.back.HistPtnFactor) // commit num mod
	dst = appendUint32(dst, distinctNum)                    // distinct num
	return dst
}

func (t *Trie) makeDedupedNodeKey(dst []byte, seq sequence, path []byte) []byte {
	commitNum := seq.CommitNum()
	dst = append(dst, t.back.DedupedSpace)                     // space
	dst = appendUint32(dst, commitNum/t.back.DedupedPtnFactor) // partition id
	dst = append(dst, t.name...)                               // trie name
	dst = encodePath(dst, path)                                // path
	return dst
}

// newDatabase creates a database instance for low-level trie construction.
func (t *Trie) newDatabase() trie.Database {
	var (
		thisHash []byte
		thisSeq  sequence
		thisPath []byte
		keyBuf   []byte
	)

	return &struct {
		trie.DatabaseReaderTo
		trie.DatabaseKeyEncoder
		trie.DatabaseReader
		trie.DatabaseWriter
	}{
		databaseGetToFunc(func(_ []byte, dst []byte) (blob []byte, err error) {
			// get from cache
			if blob = t.back.Cache.GetNodeBlob(t.name, thisSeq, thisPath, t.noFillCache, dst); len(blob) > 0 {
				return
			}
			defer func() {
				if err == nil && !t.noFillCache {
					t.back.Cache.AddNodeBlob(t.name, thisSeq, thisPath, blob, false)
				}
			}()

			// if cache missed, try fast leaf get
			if t.fastLeafGet != nil {
				if leaf, err := t.fastLeafGet(thisSeq.CommitNum()); err != nil {
					return nil, err
				} else if leaf != nil {
					// good, leaf got. returns a special error to short-circuit further node lookups.
					return nil, &leafAvailable{leaf}
				}
			}

			defer func() {
				if err == nil && !t.ext.IsNonCrypto() {
					// to ensure the node is correct, we need to verify the node hash.
					// TODO: later can skip this step
					if ok, err1 := trie.VerifyNodeHash(blob[len(dst):], thisHash); err1 != nil {
						err = errors.Wrap(err1, "verify node hash")
					} else if !ok {
						err = errors.New("node hash checksum error")
					}
				}
			}()

			// query in db
			snapshot := t.back.Store.Snapshot()
			defer snapshot.Release()

			// get from hist space first
			keyBuf = t.makeHistNodeKey(keyBuf[:0], thisSeq, thisPath)
			if val, err := snapshot.Get(keyBuf); err == nil {
				// found
				return append(dst, val...), nil
			} else if !snapshot.IsNotFound(err) {
				// error
				if !snapshot.IsNotFound(err) {
					return nil, err
				}
			}

			// then from deduped space
			keyBuf = t.makeDedupedNodeKey(keyBuf[:0], thisSeq, thisPath)
			if val, err := snapshot.Get(keyBuf); err == nil {
				return append(dst, val...), nil
			} else {
				return nil, err
			}
		}),
		databaseKeyEncodeFunc(func(hash []byte, seq uint64, path []byte) []byte {
			thisHash = hash
			thisSeq = sequence(seq)
			thisPath = path
			return nil
		}),
		nil,
		nil,
	}
}

// Copy make a copy of this trie.
func (t *Trie) Copy() *Trie {
	cpy := *t
	cpy.ext = trie.NewExtendedCached(t.ext.RootNode(), cpy.newDatabase(), t.ext.IsNonCrypto())
	cpy.ext.SetCacheTTL(cpy.back.CachedNodeTTL)
	cpy.fastLeafGet = nil

	if len(t.deletions) > 0 {
		cpy.deletions = append([]string(nil), t.deletions...)
	} else {
		cpy.deletions = nil
	}
	return &cpy
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, []byte, error) {
	return t.ext.Get(key)
}

// FastGet uses a fast way to query the value for key stored in the trie.
// See VIP-212 for detail.
func (t *Trie) FastGet(key []byte, steadyCommitNum uint32) ([]byte, []byte, error) {
	if t.back.LeafBank == nil {
		return t.ext.Get(key)
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

		// can't be determined
		if leafRec.Leaf == nil {
			return nil, nil
		}

		// if [nodeCN, steadyCN] and [leafCN, slotCN] have intersection,
		// the leaf will be the correct one.
		if nodeCommitNum <= leafRec.SlotCommitNum && leafRec.CommitNum <= steadyCommitNum {
			return leafRec.Leaf, nil
		}
		return nil, nil
	}
	defer func() { t.fastLeafGet = nil }()

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
	t.dirty = true
	if len(val) == 0 { // deletion
		if t.back.LeafBank != nil {
			t.deletions = append(t.deletions, string(key))
		}
	}
	return t.ext.Update(key, val, meta)
}

// Stage processes trie updates and calculates the new root hash.
func (t *Trie) Stage(newCommitNum, newDistinctNum uint32) (root thor.Bytes32, commit func() error) {
	var (
		thisPath []byte
		bulk     = t.back.Store.Bulk()
		buf      []byte
	)

	// make a copy of the original trie to perform commit.
	// so later if real commit is discarded, the original trie will be in
	// correct state.
	extCpy := *t.ext
	newSeq := makeSequence(newCommitNum, newDistinctNum)

	db := &struct {
		trie.DatabaseWriter
		trie.DatabaseKeyEncoder
	}{
		kv.PutFunc(func(_, blob []byte) error {
			buf = t.makeHistNodeKey(buf[:0], newSeq, thisPath)
			if err := bulk.Put(buf, blob); err != nil {
				return err
			}
			if !t.noFillCache {
				t.back.Cache.AddNodeBlob(t.name, newSeq, thisPath, blob, true)
			}
			return nil
		}),
		databaseKeyEncodeFunc(func(hash []byte, seq uint64, path []byte) []byte {
			thisPath = path
			return nil
		}),
	}

	// commit the copied trie without flush to db
	root, err := extCpy.CommitTo(db, uint64(newSeq))
	if err != nil {
		return root, func() error { return err }
	}

	commit = func() error {
		if t.back.LeafBank != nil {
			if err := t.back.LeafBank.LogDeletions(bulk, t.name, t.deletions, newCommitNum); err != nil {
				return err
			}
		}
		// real-commit, flush to db
		if err := bulk.Write(); err != nil {
			return err
		}

		t.dirty = false
		t.deletions = t.deletions[:0]

		// replace with the new root node after the copied trie committed
		newRootNode := extCpy.RootNode()
		t.ext.SetRootNode(newRootNode)
		if !t.noFillCache {
			t.back.Cache.AddRootNode(t.name, newRootNode)
		}
		return nil
	}
	return
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key
func (t *Trie) NodeIterator(start []byte, baseCommitNum uint32) trie.NodeIterator {
	return t.ext.NodeIterator(start, func(seq uint64) bool {
		return sequence(seq).CommitNum() >= baseCommitNum
	})
}

// SetNoFillCache enable or disable cache filling.
func (t *Trie) SetNoFillCache(b bool) {
	t.noFillCache = b
}

// DumpLeaves dumps leaves in the range of [baseCommitNum, targetCommitNum] into leaf bank.
// transform transforms leaves before passing into leaf bank.
func (t *Trie) DumpLeaves(ctx context.Context, baseCommitNum, targetCommitNum uint32, transform func(*trie.Leaf) *trie.Leaf) error {
	if t.dirty {
		return errors.New("dirty trie")
	}
	if t.back.LeafBank == nil {
		return nil
	}

	leafUpdater, err := t.back.LeafBank.NewUpdater(t.name, baseCommitNum, targetCommitNum)
	if err != nil {
		return err
	}
	var (
		checkContext = newContextChecker(ctx, 5000)
		iter         = t.NodeIterator(nil, baseCommitNum)
	)

	for iter.Next(true) {
		if err := checkContext(); err != nil {
			return err
		}

		if leaf := iter.Leaf(); leaf != nil {
			seq := sequence(iter.SeqNum())
			if err := leafUpdater.Update(iter.LeafKey(), transform(leaf), seq.CommitNum()); err != nil {
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

		if err := iter.Node(func(blob []byte) error {
			buf = t.makeDedupedNodeKey(buf[:0], sequence(iter.SeqNum()), iter.Path())
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
			Start: appendUint32([]byte{back.HistSpace}, startCommitNum/back.HistPtnFactor),
			Limit: appendUint32([]byte{back.HistSpace}, (limitCommitNum-1)/back.HistPtnFactor+1),
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
		ptn := binary.BigEndian.Uint32(key[1:])
		mod := binary.BigEndian.Uint32(key[len(key)-8:])
		nodeCommitNum := ptn*back.HistPtnFactor + mod
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
	databaseKeyEncodeFunc func(hash []byte, seq uint64, path []byte) []byte
	databaseGetToFunc     func(key, dst []byte) ([]byte, error)
)

func (f databaseKeyEncodeFunc) Encode(hash []byte, seq uint64, path []byte) []byte {
	return f(hash, seq, path)
}

func (f databaseGetToFunc) GetTo(key, dst []byte) ([]byte, error) {
	return f(key, dst)
}

// leafAvailable is a special error type to short circuit trie get method.
type leafAvailable struct {
	*trie.Leaf
}

func (*leafAvailable) Error() string {
	return "leaf available"
}
