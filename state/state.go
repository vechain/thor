package state

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/stackedmap"
)

// State manages the main accounts trie.
type State struct {
	root      cry.Hash // root of initial accounts trie
	kv        kv.GetPutter
	trie      trieReader                    // the accounts trie reader
	trieCache map[acc.Address]*cachedObject // cache of accounts trie
	sm        *stackedmap.StackedMap        // keeps revisions of accounts state
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
func New(root cry.Hash, kv kv.GetPutter) (*State, error) {
	trie, err := trie.NewSecure(common.Hash(root), kv, 0)
	if err != nil {
		return nil, err
	}

	state := State{
		root:      root,
		kv:        kv,
		trie:      trie,
		trieCache: make(map[acc.Address]*cachedObject),
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
	case acc.Address: // get balance
		return s.getCachedObject(k).data.Balance, true
	case codeKey: // get code
		co := s.getCachedObject(acc.Address(k))
		code, err := co.GetCode()
		if err != nil {
			s.setError(err)
			return []byte(nil), true
		}
		return code, true
	case codeHashKey:
		return s.getCachedObject(acc.Address(k)).data.CodeHash, true
	case storageKey: // get storage
		v, err := s.getCachedObject(k.addr).GetStorage(k.key)
		if err != nil {
			s.setError(err)
			return cry.Hash{}, true
		}
		return v, true
	}
	panic(fmt.Errorf("unexpected key type %+v", key))
}

// build changes via journal of stackedMap.
func (s *State) changes() map[acc.Address]*changedObject {
	changes := make(map[acc.Address]*changedObject)

	// get or create changedObject
	getObj := func(addr acc.Address) *changedObject {
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
		case acc.Address:
			getObj(key).data.Balance = v.(*big.Int)
		case codeKey:
			getObj(acc.Address(key)).code = v.([]byte)
		case codeHashKey:
			getObj(acc.Address(key)).data.CodeHash = v.([]byte)
		case storageKey:
			o := getObj(key.addr)
			if o.storage == nil {
				o.storage = make(map[cry.Hash]cry.Hash)
			}
			o.storage[key.key] = v.(cry.Hash)
		}
		// abort if error occurred
		return s.err == nil
	})
	return changes
}

func (s *State) getCachedObject(addr acc.Address) *cachedObject {
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
func (s *State) ForEachStorage(addr acc.Address, cb func(key, value cry.Hash) bool) {
	// skip if no code
	if (s.GetCodeHash(addr) == cry.Hash{}) {
		return
	}

	// build ongoing key-value pairs
	ongoing := make(map[cry.Hash]cry.Hash)
	s.sm.Journal(func(k, v interface{}) bool {
		if sk, ok := k.(storageKey); ok {
			if sk.addr == addr {
				ongoing[sk.key] = v.(cry.Hash)
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
		key := cry.BytesToHash(strie.GetKey(iter.Key))
		if _, ok := ongoing[key]; !ok {
			if !cb(key, cry.BytesToHash(iter.Value)) {
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
func (s *State) GetBalance(addr acc.Address) *big.Int {
	v, _ := s.sm.Get(addr)
	return v.(*big.Int)
}

// SetBalance set balance for the given address.
func (s *State) SetBalance(addr acc.Address, balance *big.Int) {
	s.sm.Put(addr, balance)
}

// GetStorage returns storage value for the given address and key.
// It always returns empty value, if the account at address is empty.
func (s *State) GetStorage(addr acc.Address, key cry.Hash) cry.Hash {
	if (s.GetCodeHash(addr) == cry.Hash{}) {
		return cry.Hash{}
	}

	v, _ := s.sm.Get(storageKey{addr, key})
	return v.(cry.Hash)
}

// SetStorage set storage value for the given address and key.
// It will do nothing if call on the address where the account not exists.
func (s *State) SetStorage(addr acc.Address, key, value cry.Hash) {
	if (s.GetCodeHash(addr) == cry.Hash{}) {
		return
	}
	s.sm.Put(storageKey{addr, key}, value)
}

// GetCode returns code for the given address.
func (s *State) GetCode(addr acc.Address) []byte {
	v, _ := s.sm.Get(codeKey(addr))
	return v.([]byte)
}

// GetCodeHash returns code hash for the given address.
func (s *State) GetCodeHash(addr acc.Address) cry.Hash {
	v, _ := s.sm.Get(codeHashKey(addr))
	return cry.BytesToHash(v.([]byte))
}

// SetCode set code for the given address.
func (s *State) SetCode(addr acc.Address, code []byte) {
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
func (s *State) Exists(addr acc.Address) bool {
	return s.GetBalance(addr).Sign() != 0 || (s.GetCodeHash(addr) != cry.Hash{})
}

// Delete delete an account at the given address.
// That's set both balance and code to zero value.
func (s *State) Delete(addr acc.Address) {
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
		addr acc.Address
		key  cry.Hash
	}

	codeKey     acc.Address
	codeHashKey acc.Address

	changedObject struct {
		data    Account
		storage map[cry.Hash]cry.Hash
		code    []byte
	}
)
