package state

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/stackedmap"
	"github.com/vechain/thor/thor"
)

// State manages the main accounts trie.
type State struct {
	root      thor.Hash // root of initial accounts trie
	kv        kv.GetPutter
	trie      trieReader                     // the accounts trie reader
	trieCache map[thor.Address]*cachedObject // cache of accounts trie
	sm        *stackedmap.StackedMap         // keeps revisions of accounts state
	err       error
}

// to constrain ability of trie
type trieReader interface {
	TryGet(key []byte) ([]byte, error)
}

// to constrain ability of trie
type trieWriter interface {
	TryUpdate(key, value []byte) error
	TryDelete(key []byte) error
}

// New create an state object.
func New(root thor.Hash, kv kv.GetPutter) (*State, error) {
	trie, err := trie.NewSecure(common.Hash(root), kv, 0)
	if err != nil {
		return nil, err
	}

	state := State{
		root:      root,
		kv:        kv,
		trie:      trie,
		trieCache: make(map[thor.Address]*cachedObject),
	}

	state.sm = stackedmap.New(func(key interface{}) (value interface{}, exist bool) {
		return state.cacheGetter(key)
	})

	// initially has 1 stack depth
	state.sm.Push()
	return &state, nil
}

// implements stackedmap.MapGetter
func (s *State) cacheGetter(key interface{}) (value interface{}, exist bool) {
	switch k := key.(type) {
	case thor.Address: // get balance
		return s.getCachedObject(k).data.Balance, true
	case codeKey: // get code
		co := s.getCachedObject(thor.Address(k))
		code, err := co.GetCode()
		if err != nil {
			s.setError(err)
			return []byte(nil), true
		}
		return code, true
	case codeHashKey:
		return s.getCachedObject(thor.Address(k)).data.CodeHash, true
	case storageKey: // get storage
		v, err := s.getCachedObject(k.addr).GetStorage(k.key)
		if err != nil {
			s.setError(err)
			return thor.Hash{}, true
		}
		return v, true
	}
	panic(fmt.Errorf("unexpected key type %+v", key))
}

// build changes via journal of stackedMap.
func (s *State) changes() map[thor.Address]*changedObject {
	changes := make(map[thor.Address]*changedObject)

	// get or create changedObject
	getObj := func(addr thor.Address) *changedObject {
		if obj, ok := changes[addr]; ok {
			return obj
		}
		obj := &changedObject{data: s.getCachedObject(addr).data}
		changes[addr] = obj
		return obj
	}

	// traverse journal to build changes
	s.sm.Journal(func(k, v interface{}) bool {
		switch key := k.(type) {
		case thor.Address:
			getObj(key).data.Balance = v.(*big.Int)
		case codeKey:
			getObj(thor.Address(key)).code = v.([]byte)
		case codeHashKey:
			getObj(thor.Address(key)).data.CodeHash = v.([]byte)
		case storageKey:
			o := getObj(key.addr)
			if o.storage == nil {
				o.storage = make(map[thor.Hash]thor.Hash)
			}
			o.storage[key.key] = v.(thor.Hash)
		}
		// abort if error occurred
		return s.err == nil
	})
	return changes
}

func (s *State) getCachedObject(addr thor.Address) *cachedObject {
	if co, ok := s.trieCache[addr]; ok {
		return co
	}
	a, err := loadAccount(s.trie, addr)
	if err != nil {
		s.setError(err)
		return newCachedObject(s.kv, emptyAccount)
	}
	co := newCachedObject(s.kv, a)
	s.trieCache[addr] = co
	return co
}

// ForEachStorage iterates all storage key-value pairs for given address.
// It's for debug purpose.
func (s *State) ForEachStorage(addr thor.Address, cb func(key, value thor.Hash) bool) {
	// skip if no code
	if (s.GetCodeHash(addr) == thor.Hash{}) {
		return
	}

	// build ongoing key-value pairs
	ongoing := make(map[thor.Hash]thor.Hash)
	s.sm.Journal(func(k, v interface{}) bool {
		if sk, ok := k.(storageKey); ok {
			if sk.addr == addr {
				ongoing[sk.key] = v.(thor.Hash)
			}
		}
		return true
	})

	for k, v := range ongoing {
		if !cb(k, v) {
			return
		}
	}

	co := s.getCachedObject(addr)
	strie, err := trie.NewSecure(common.BytesToHash(co.data.StorageRoot), s.kv, 0)
	if err != nil {
		s.setError(err)
		return
	}

	iter := trie.NewIterator(strie.NodeIterator(nil))
	for iter.Next() {
		if s.err != nil {
			return
		}
		// skip cached values
		key := thor.BytesToHash(strie.GetKey(iter.Key))
		if _, ok := ongoing[key]; !ok {
			if !cb(key, thor.BytesToHash(iter.Value)) {
				return
			}
		}
	}
}

func (s *State) setError(err error) {
	if s.err == nil {
		s.err = err
	}
}

// Error returns first occurred error.
func (s *State) Error() error {
	return s.err
}

// GetBalance returns balance for the given address.
func (s *State) GetBalance(addr thor.Address) *big.Int {
	v, _ := s.sm.Get(addr)
	return v.(*big.Int)
}

// SetBalance set balance for the given address.
func (s *State) SetBalance(addr thor.Address, balance *big.Int) {
	s.sm.Put(addr, balance)
}

// GetStorage returns storage value for the given address and key.
func (s *State) GetStorage(addr thor.Address, key thor.Hash) thor.Hash {
	v, _ := s.sm.Get(storageKey{addr, key})
	return v.(thor.Hash)
}

// SetStorage set storage value for the given address and key.
func (s *State) SetStorage(addr thor.Address, key, value thor.Hash) {
	s.sm.Put(storageKey{addr, key}, value)
}

// GetCode returns code for the given address.
func (s *State) GetCode(addr thor.Address) []byte {
	v, _ := s.sm.Get(codeKey(addr))
	return v.([]byte)
}

// GetCodeHash returns code hash for the given address.
func (s *State) GetCodeHash(addr thor.Address) thor.Hash {
	v, _ := s.sm.Get(codeHashKey(addr))
	return thor.BytesToHash(v.([]byte))
}

// SetCode set code for the given address.
func (s *State) SetCode(addr thor.Address, code []byte) {
	if len(code) > 0 {
		s.sm.Put(codeKey(addr), code)
		hash := cry.HashSum(code)
		s.sm.Put(codeHashKey(addr), hash[:])
	} else {
		s.sm.Put(codeKey(addr), []byte(nil))
		s.sm.Put(codeHashKey(addr), []byte(nil))
	}
}

// Exists returns whether an account exists at the given address.
// See Account.IsEmpty()
func (s *State) Exists(addr thor.Address) bool {
	return s.GetBalance(addr).Sign() != 0 || (s.GetCodeHash(addr) != thor.Hash{})
}

// Delete delete an account at the given address.
// That's set both balance and code to zero value.
func (s *State) Delete(addr thor.Address) {
	s.SetBalance(addr, &big.Int{})
	s.SetCode(addr, nil)
}

// NewCheckpoint makes a checkpoint of current state.
// It returns revision of the checkpoint.
func (s *State) NewCheckpoint() int {
	return s.sm.Push()
}

// Revert revert to last checkpoint and drop all subsequent changes.
func (s *State) Revert() {
	s.sm.Pop()
	// ensure depth 1
	if s.sm.Depth() == 0 {
		s.sm.Push()
	}
}

// RevertTo revert to checkpoint specified by revision.
func (s *State) RevertTo(revision int) {
	s.sm.PopTo(revision)
	// ensure depth 1
	if s.sm.Depth() == 0 {
		s.sm.Push()
	}
}

// Stage makes a stage object to compute hash of trie or commit all changes.
func (s *State) Stage() *Stage {
	if s.err != nil {
		return &Stage{err: s.err}
	}
	changes := s.changes()
	if s.err != nil {
		return &Stage{err: s.err}
	}
	return newStage(s.root, s.kv, changes)
}

type (
	storageKey struct {
		addr thor.Address
		key  thor.Hash
	}

	codeKey     thor.Address
	codeHashKey thor.Address

	changedObject struct {
		data    Account
		storage map[thor.Hash]thor.Hash
		code    []byte
	}
)
