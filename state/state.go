// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/stackedmap"
	"github.com/vechain/thor/thor"
)

// State manages the main accounts trie.
type State struct {
	root     thor.Bytes32 // root of initial accounts trie
	kv       kv.GetPutter
	trie     trieReader                     // the accounts trie reader
	cache    map[thor.Address]*cachedObject // cache of accounts trie
	sm       *stackedmap.StackedMap         // keeps revisions of accounts state
	err      error
	setError func(err error)
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
func New(root thor.Bytes32, kv kv.GetPutter) (*State, error) {
	trie, err := trCache.Get(root, kv, false)
	if err != nil {
		return nil, err
	}

	state := State{
		root:  root,
		kv:    kv,
		trie:  trie,
		cache: make(map[thor.Address]*cachedObject),
	}
	state.setError = func(err error) {
		if state.err == nil {
			state.err = err
		}
	}
	state.sm = stackedmap.New(func(key interface{}) (value interface{}, exist bool) {
		return state.cacheGetter(key)
	})
	return &state, nil
}

// Spawn create a new state object shares current state's underlying db.
// Also errors will be reported to current state.
func (s *State) Spawn(root thor.Bytes32) *State {
	newState, err := New(root, s.kv)
	if err != nil {
		s.setError(err)
		newState, _ = New(thor.Bytes32{}, s.kv)
	}
	newState.setError = s.setError
	return newState
}

// implements stackedmap.MapGetter
func (s *State) cacheGetter(key interface{}) (value interface{}, exist bool) {
	switch k := key.(type) {
	case thor.Address: // get account
		return &s.getCachedObject(k).data, true
	case codeKey: // get code
		co := s.getCachedObject(thor.Address(k))
		code, err := co.GetCode()
		if err != nil {
			s.setError(err)
			return []byte(nil), true
		}
		return code, true
	case storageKey: // get storage
		v, err := s.getCachedObject(k.addr).GetStorage(k.key)
		if err != nil {
			s.setError(err)
			return rlp.RawValue(nil), true
		}
		return v, true
	}
	panic(fmt.Errorf("unexpected key type %+v", key))
}

// build changes via journal of stackedMap.
func (s *State) changes() map[thor.Address]*changedObject {
	changes := make(map[thor.Address]*changedObject)

	// get or create changedObject
	getOrNewObj := func(addr thor.Address) *changedObject {
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
			getOrNewObj(key).data = *(v.(*Account))
		case codeKey:
			getOrNewObj(thor.Address(key)).code = v.([]byte)
		case storageKey:
			o := getOrNewObj(key.addr)
			if o.storage == nil {
				o.storage = make(map[thor.Bytes32]rlp.RawValue)
			}
			o.storage[key.key] = v.(rlp.RawValue)
		}
		// abort if error occurred
		return s.err == nil
	})
	return changes
}

func (s *State) getCachedObject(addr thor.Address) *cachedObject {
	if co, ok := s.cache[addr]; ok {
		return co
	}
	a, err := loadAccount(s.trie, addr)
	if err != nil {
		s.setError(err)
		return newCachedObject(s.kv, emptyAccount())
	}
	co := newCachedObject(s.kv, a)
	s.cache[addr] = co
	return co
}

// the returned account should not be modified
func (s *State) getAccount(addr thor.Address) *Account {
	v, _ := s.sm.Get(addr)
	return v.(*Account)
}

func (s *State) getAccountCopy(addr thor.Address) Account {
	return *s.getAccount(addr)
}

func (s *State) updateAccount(addr thor.Address, acc *Account) {
	s.sm.Put(addr, acc)
}

// ForEachStorage iterates all storage key-value pairs for given address.
// It's for debug purpose.
// func (s *State) ForEachStorage(addr thor.Address, cb func(key thor.Bytes32, value []byte) bool) {
// 	// skip if no code
// 	if (s.GetCodeHash(addr) == thor.Bytes32{}) {
// 		return
// 	}

// 	// build ongoing key-value pairs
// 	ongoing := make(map[thor.Bytes32][]byte)
// 	s.sm.Journal(func(k, v interface{}) bool {
// 		if key, ok := k.(storageKey); ok {
// 			if key.addr == addr {
// 				ongoing[key.key] = v.([]byte)
// 			}
// 		}
// 		return true
// 	})

// 	for k, v := range ongoing {
// 		if !cb(k, v) {
// 			return
// 		}
// 	}

// 	co := s.getCachedObject(addr)
// 	strie, err := trie.NewSecure(thor.BytesToBytes32(co.data.StorageRoot), s.kv, 0)
// 	if err != nil {
// 		s.setError(err)
// 		return
// 	}

// 	iter := trie.NewIterator(strie.NodeIterator(nil))
// 	for iter.Next() {
// 		if s.err != nil {
// 			return
// 		}
// 		// skip cached values
// 		key := thor.BytesToBytes32(strie.GetKey(iter.Key))
// 		if _, ok := ongoing[key]; !ok {
// 			if !cb(key, iter.Value) {
// 				return
// 			}
// 		}
// 	}
// }

// Err returns first occurred error.
func (s *State) Err() error {
	return s.err
}

// GetBalance returns balance for the given address.
func (s *State) GetBalance(addr thor.Address) *big.Int {
	return s.getAccount(addr).Balance
}

// SetBalance set balance for the given address.
func (s *State) SetBalance(addr thor.Address, balance *big.Int) {
	cpy := s.getAccountCopy(addr)
	cpy.Balance = balance
	s.updateAccount(addr, &cpy)
}

// GetEnergy get energy for the given address at block number specified.
func (s *State) GetEnergy(addr thor.Address, blockTime uint64) *big.Int {
	return s.getAccount(addr).CalcEnergy(blockTime)
}

// SetEnergy set energy at block number for the given address.
func (s *State) SetEnergy(addr thor.Address, energy *big.Int, blockTime uint64) {
	cpy := s.getAccountCopy(addr)
	cpy.Energy, cpy.BlockTime = energy, blockTime
	s.updateAccount(addr, &cpy)
}

// GetMaster get master for the given address.
// Master can move energy, manage users...
func (s *State) GetMaster(addr thor.Address) thor.Address {
	return thor.BytesToAddress(s.getAccount(addr).Master)
}

// SetMaster set master for the given address.
func (s *State) SetMaster(addr thor.Address, master thor.Address) {
	cpy := s.getAccountCopy(addr)
	if master.IsZero() {
		cpy.Master = nil
	} else {
		cpy.Master = master[:]
	}
	s.updateAccount(addr, &cpy)
}

// GetStorage returns storage value for the given address and key.
func (s *State) GetStorage(addr thor.Address, key thor.Bytes32) thor.Bytes32 {
	raw := s.GetRawStorage(addr, key)
	if len(raw) == 0 {
		return thor.Bytes32{}
	}
	kind, content, _, err := rlp.Split(raw)
	if err != nil {
		s.setError(err)
		return thor.Bytes32{}
	}
	if kind == rlp.List {
		// special case for rlp list, it should be customized storage value
		// return hash of raw data
		return thor.Blake2b(raw)
	}
	return thor.BytesToBytes32(content)
}

// SetStorage set storage value for the given address and key.
func (s *State) SetStorage(addr thor.Address, key, value thor.Bytes32) {
	if value.IsZero() {
		s.SetRawStorage(addr, key, nil)
		return
	}
	v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
	s.SetRawStorage(addr, key, v)
}

// GetRawStorage returns storage value in rlp raw for given address and key.
func (s *State) GetRawStorage(addr thor.Address, key thor.Bytes32) rlp.RawValue {
	data, _ := s.sm.Get(storageKey{addr, key})
	return data.(rlp.RawValue)
}

// SetRawStorage set storage value in rlp raw.
func (s *State) SetRawStorage(addr thor.Address, key thor.Bytes32, raw rlp.RawValue) {
	s.sm.Put(storageKey{addr, key}, raw)
}

// EncodeStorage set storage value encoded by given enc method.
// Error returned by end will be absorbed by State instance.
func (s *State) EncodeStorage(addr thor.Address, key thor.Bytes32, enc func() ([]byte, error)) {
	raw, err := enc()
	if err != nil {
		s.setError(err)
		return
	}
	s.SetRawStorage(addr, key, raw)
}

// DecodeStorage get and decode storage value.
// Error returned by dec will be absorbed by State instance.
func (s *State) DecodeStorage(addr thor.Address, key thor.Bytes32, dec func([]byte) error) {
	raw := s.GetRawStorage(addr, key)
	if err := dec(raw); err != nil {
		s.setError(err)
	}
}

// GetCode returns code for the given address.
func (s *State) GetCode(addr thor.Address) []byte {
	v, _ := s.sm.Get(codeKey(addr))
	return v.([]byte)
}

// GetCodeHash returns code hash for the given address.
func (s *State) GetCodeHash(addr thor.Address) thor.Bytes32 {
	return thor.BytesToBytes32(s.getAccount(addr).CodeHash)
}

// SetCode set code for the given address.
func (s *State) SetCode(addr thor.Address, code []byte) {
	var codeHash []byte
	if len(code) > 0 {
		s.sm.Put(codeKey(addr), code)
		codeHash = crypto.Keccak256(code)
	} else {
		s.sm.Put(codeKey(addr), []byte(nil))
	}
	cpy := s.getAccountCopy(addr)
	cpy.CodeHash = codeHash
	s.updateAccount(addr, &cpy)
}

// Exists returns whether an account exists at the given address.
// See Account.IsEmpty()
func (s *State) Exists(addr thor.Address) bool {
	return !s.getAccount(addr).IsEmpty()
}

// Delete delete an account at the given address.
// That's set balance, energy and code to zero value.
func (s *State) Delete(addr thor.Address) {
	s.sm.Put(codeKey(addr), []byte(nil))
	s.updateAccount(addr, emptyAccount())
}

// NewCheckpoint makes a checkpoint of current state.
// It returns revision of the checkpoint.
func (s *State) NewCheckpoint() int {
	return s.sm.Push()
}

// RevertTo revert to checkpoint specified by revision.
func (s *State) RevertTo(revision int) {
	s.sm.PopTo(revision)
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
		key  thor.Bytes32
	}
	codeKey       thor.Address
	changedObject struct {
		data    Account
		storage map[thor.Bytes32]rlp.RawValue
		code    []byte
	}
)
