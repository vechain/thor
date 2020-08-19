// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

// individual functions of trie.DatabaseXXXEx interface.
type (
	getEncodedFunc func(key *trie.NodeKey) ([]byte, error)
	getDecodedFunc func(key *trie.NodeKey) (interface{}, func(interface{}))
	putEncodedFunc func(key *trie.NodeKey, enc []byte) error
)

func (f getEncodedFunc) GetEncoded(key *trie.NodeKey) ([]byte, error)                  { return f(key) }
func (f getDecodedFunc) GetDecoded(key *trie.NodeKey) (interface{}, func(interface{})) { return f(key) }
func (f putEncodedFunc) PutEncoded(key *trie.NodeKey, enc []byte) error                { return f(key, enc) }

var (
	_ trie.DatabaseReaderEx = (*struct {
		getEncodedFunc
		getDecodedFunc
	})(nil)
	_ trie.DatabaseWriterEx = putEncodedFunc(nil)
)

// Trie is the managed trie.
type Trie struct {
	store        kv.Store
	name         string
	originalRoot thor.Bytes32
	cache        *trieCache
	keyBuf       trieNodeKeyBuf
	secure       bool
	liveSpace    *trieLiveSpace
	lazyInit     func() (*trie.Trie, error)
	secureKeys   map[thor.Bytes32][]byte
	permanent    bool
}

func newTrie(
	store kv.Store,
	name string,
	root thor.Bytes32,
	cache *trieCache,
	secure bool,
	liveSpace *trieLiveSpace,
	permanent bool,
) *Trie {
	var (
		tr = &Trie{
			store:        store,
			name:         name,
			originalRoot: root,
			cache:        cache,
			keyBuf:       newTrieNodeKeyBuf(name),
			secure:       secure,
			liveSpace:    liveSpace,
			permanent:    permanent,
		}
		trieObj *trie.Trie // the real trie object
		initErr error
	)

	tr.lazyInit = func() (*trie.Trie, error) {
		if trieObj == nil && initErr == nil {
			trieObj, initErr = trie.New(root, &struct {
				trie.Database
				getEncodedFunc
				getDecodedFunc
			}{
				nil, // leave out trie.Database, since here provides trie.DatabaseReaderEx impl
				tr.getEncoded,
				tr.getDecoded,
			})
		}
		return trieObj, initErr
	}
	return tr
}

// Name returns trie name.
// Tries with different names have different key spaces.
func (t *Trie) Name() string {
	return t.name
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
func (t *Trie) Get(key []byte) ([]byte, error) {
	obj, err := t.lazyInit()
	if err != nil {
		return nil, err
	}
	return obj.TryGet(t.hashKey(key, false))
}

// Update associates key with value in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value bytes must not be modified by the caller while they are
// stored in the trie.
func (t *Trie) Update(key, val []byte) error {
	obj, err := t.lazyInit()
	if err != nil {
		return err
	}
	return obj.TryUpdate(t.hashKey(key, true), val)
}

// Hash returns the root hash of the trie.
func (t *Trie) Hash() thor.Bytes32 {
	obj, err := t.lazyInit()
	if err != nil {
		// here return original root is ok, since we can
		// confirm that there's no preceding successful update.
		return t.originalRoot
	}
	return obj.Hash()
}

// Commit writes all nodes to the trie's database.
func (t *Trie) Commit() (thor.Bytes32, error) {
	return t.commit(t.permanent)
}

// CommitPermanently writes all nodes directly into permanent space.
// All nodes committed in this way can not be pruned. It's for test purpose only.
func (t *Trie) CommitPermanently() (thor.Bytes32, error) {
	return t.commit(true)
}

func (t *Trie) commit(permanent bool) (root thor.Bytes32, err error) {
	obj, err := t.lazyInit()
	if err != nil {
		return
	}

	space := trieSpaceP
	if !permanent {
		space = t.liveSpace.Active()
	}

	err = t.store.Batch(func(putter kv.PutFlusher) error {
		root, err = t.doCommit(putter, obj, space)
		return err
	})
	return
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key
func (t *Trie) NodeIterator(start []byte) trie.NodeIterator {
	obj, err := t.lazyInit()
	if err != nil {
		return &errorIterator{err}
	}
	return obj.NodeIterator(start)
}

// GetKeyPreimage returns the blake2b preimage of a hashed key that was
// previously used to store a value.
func (t *Trie) GetKeyPreimage(hash thor.Bytes32) []byte {
	if key, ok := t.secureKeys[hash]; ok {
		return key
	}

	dbKey := [1 + 32]byte{trieSecureKeySpace}
	copy(dbKey[1:], hash[:])
	key, _ := t.store.Get(dbKey[:])
	return key
}

func (t *Trie) hashKey(key []byte, save bool) []byte {
	// short circute for non-secure trie.
	if !t.secure {
		return key
	}

	h := thor.Blake2b(key)
	if save {
		if t.secureKeys == nil {
			t.secureKeys = make(map[thor.Bytes32][]byte)
		}
		// have to make a copy because the key can be modified later.
		t.secureKeys[h] = append([]byte(nil), key...)
	}
	return h[:]
}

func (t *Trie) getEncoded(key *trie.NodeKey) (enc []byte, err error) {
	// retrieve from cache
	enc = t.cache.GetEncoded(key.Hash, len(key.Path), key.Scaning)
	if len(enc) > 0 {
		return
	}

	// It's important to use snapshot here.
	// Getting an encoded node from db may have at most 3 get ops. Snapshot
	// can prevent parallel node deletions by trie pruner.
	if err = t.store.Snapshot(func(getter kv.Getter) error {
		enc, err = t.keyBuf.Get(getter.Get, key)
		return err
	}); err != nil {
		return
	}

	// skip caching when scaning(iterating) a trie, to prevent the cache from
	// being over filled.
	if !key.Scaning {
		t.cache.SetEncoded(key.Hash, enc, len(key.Path))
	}
	return
}

func (t *Trie) getDecoded(key *trie.NodeKey) (interface{}, func(interface{})) {
	if cached := t.cache.GetDecoded(key.Hash, len(key.Path), key.Scaning); cached != nil {
		return cached, nil
	}
	if !key.Scaning {
		// fill cache only if not iterating
		return nil, func(dec interface{}) { t.cache.SetDecoded(key.Hash, dec, len(key.Path)) }
	}
	return nil, nil
}

func (t *Trie) doCommit(putter kv.Putter, trieObj *trie.Trie, space byte) (root thor.Bytes32, err error) {
	// save secure key preimages
	if len(t.secureKeys) > 0 {
		buf := [1 + 32]byte{trieSecureKeySpace}
		for h, p := range t.secureKeys {
			copy(buf[1:], h[:])
			if err = putter.Put(buf[:], p); err != nil {
				return
			}
		}
		t.secureKeys = nil
	}

	return trieObj.CommitTo(&struct {
		trie.DatabaseWriter
		putEncodedFunc
	}{
		nil, // leave out trie.DatabaseWriter, because here provides trie.DatabaseWriterEx
		// implements trie.DatabaseWriterEx.PutEncoded
		func(key *trie.NodeKey, enc []byte) error {
			t.cache.SetEncoded(key.Hash, enc, len(key.Path))
			return t.keyBuf.Put(putter.Put, key, enc, space)
		},
	})
}

// errorIterator an iterator always in error state.
type errorIterator struct {
	err error
}

func (i *errorIterator) Next(bool) bool        { return false }
func (i *errorIterator) Error() error          { return i.err }
func (i *errorIterator) Hash() thor.Bytes32    { return thor.Bytes32{} }
func (i *errorIterator) Node() ([]byte, error) { return nil, i.err }
func (i *errorIterator) Parent() thor.Bytes32  { return thor.Bytes32{} }
func (i *errorIterator) Path() []byte          { return nil }
func (i *errorIterator) Leaf() bool            { return false }
func (i *errorIterator) LeafKey() []byte       { return nil }
func (i *errorIterator) LeafBlob() []byte      { return nil }
func (i *errorIterator) LeafProof() [][]byte   { return nil }
