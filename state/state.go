package state

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/stackedmap"
	"github.com/vechain/thor/thor"
)

// State manages the main accounts trie.
type State struct {
	root  thor.Bytes32 // root of initial accounts trie
	db    *trie.Database
	trie  trieReader                     // the accounts trie reader
	cache map[thor.Address]*cachedObject // cache of accounts trie
	sm    *stackedmap.StackedMap         // keeps revisions of accounts state
	err   error
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
	db := newTrieDatabase(kv)
	trie, err := trie.NewSecure(common.Hash(root), db, 0)
	if err != nil {
		return nil, err
	}

	state := State{
		root:  root,
		db:    db,
		trie:  trie,
		cache: make(map[thor.Address]*cachedObject),
	}

	state.sm = stackedmap.New(func(key interface{}) (value interface{}, exist bool) {
		return state.cacheGetter(key)
	})
	return &state, nil
}

// implements stackedmap.MapGetter
func (s *State) cacheGetter(key interface{}) (value interface{}, exist bool) {
	switch k := key.(type) {
	case thor.Address: // get balance
		return s.getCachedObject(k).data.Balance, true
	case energyKey: // get energy
		data := s.getCachedObject(thor.Address(k)).data
		return energyState{data.Energy, data.BlockNum}, true
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
			return []byte(nil), true
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
			getOrNewObj(key).data.Balance = v.(*big.Int)
		case energyKey:
			obj := getOrNewObj(thor.Address(key))
			es := v.(energyState)
			obj.data.Energy, obj.data.BlockNum = es.energy, es.blockNum
		case codeKey:
			getOrNewObj(thor.Address(key)).code = v.([]byte)
		case codeHashKey:
			getOrNewObj(thor.Address(key)).data.CodeHash = v.([]byte)
		case storageKey:
			o := getOrNewObj(key.addr)
			if o.storage == nil {
				o.storage = make(map[thor.Bytes32][]byte)
			}
			o.storage[key.key] = v.([]byte)
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
		return newCachedObject(s.db, emptyAccount())
	}
	co := newCachedObject(s.db, a)
	s.cache[addr] = co
	return co
}

// ForEachStorage iterates all storage key-value pairs for given address.
// It's for debug purpose.
func (s *State) ForEachStorage(addr thor.Address, cb func(key thor.Bytes32, value []byte) bool) {
	// skip if no code
	if (s.GetCodeHash(addr) == thor.Bytes32{}) {
		return
	}

	// build ongoing key-value pairs
	ongoing := make(map[thor.Bytes32][]byte)
	s.sm.Journal(func(k, v interface{}) bool {
		if key, ok := k.(storageKey); ok {
			if key.addr == addr {
				ongoing[key.key] = v.([]byte)
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
	strie, err := trie.NewSecure(common.BytesToHash(co.data.StorageRoot), s.db, 0)
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
		key := thor.BytesToBytes32(strie.GetKey(iter.Key))
		if _, ok := ongoing[key]; !ok {
			if !cb(key, iter.Value) {
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

// GetEnergy get energy for the given address at block number specified.
func (s *State) GetEnergy(addr thor.Address, blockNum uint32) *big.Int {
	v, _ := s.sm.Get(energyKey(addr))
	es := v.(energyState)
	return es.CalcEnergy(s.GetBalance(addr), blockNum)
}

// SetEnergy set energy at block number for the given address.
func (s *State) SetEnergy(addr thor.Address, blockNum uint32, energy *big.Int) {
	s.sm.Put(energyKey(addr), energyState{energy, blockNum})
}

// GetStorage returns storage value for the given address and key.
func (s *State) GetStorage(addr thor.Address, key thor.Bytes32) (value thor.Bytes32) {
	s.GetStructedStorage(addr, key, &value)
	return
}

// SetStorage set storage value for the given address and key.
func (s *State) SetStorage(addr thor.Address, key, value thor.Bytes32) {
	s.SetStructedStorage(addr, key, value)
}

// GetStructedStorage get and decode raw storage for given address and key.
// 'val' should either implements StorageDecoder, or int type of
// [
//   *thor.Bytes32, *thor.Address,
//   *string
//   *uintx
//   *big.Int
// ]
func (s *State) GetStructedStorage(addr thor.Address, key thor.Bytes32, val interface{}) {
	data, _ := s.sm.Get(storageKey{addr, key})
	if dec, ok := val.(StorageDecoder); ok {
		s.setError(dec.Decode(data.([]byte)))
		return
	}
	s.setError(decodeStorage(data.([]byte), val))
}

// SetStructedStorage encode val and set as raw storage value for given address and key.
// 'val' should ether implements StorageEncoder, or in type of
// [
//	  thor.Bytes32, thor.Address,
//    string
//    uintx
//    *big.Int
// ]
// If 'val' is nil, the storage is cleared.
func (s *State) SetStructedStorage(addr thor.Address, key thor.Bytes32, val interface{}) {
	if val == nil {
		s.sm.Put(storageKey{addr, key}, []byte(nil))
		return
	}
	if enc, ok := val.(StorageEncoder); ok {
		data, err := enc.Encode()
		if err != nil {
			s.setError(err)
			return
		}
		s.sm.Put(storageKey{addr, key}, data)
		return
	}

	data, err := encodeStorage(val)
	if err != nil {
		s.setError(err)
		return
	}
	s.sm.Put(storageKey{addr, key}, data)
}

// GetCode returns code for the given address.
func (s *State) GetCode(addr thor.Address) []byte {
	v, _ := s.sm.Get(codeKey(addr))
	return v.([]byte)
}

// GetCodeHash returns code hash for the given address.
func (s *State) GetCodeHash(addr thor.Address) thor.Bytes32 {
	v, _ := s.sm.Get(codeHashKey(addr))
	return thor.BytesToBytes32(v.([]byte))
}

// SetCode set code for the given address.
func (s *State) SetCode(addr thor.Address, code []byte) {
	if len(code) > 0 {
		s.sm.Put(codeKey(addr), code)
		s.sm.Put(codeHashKey(addr), crypto.Keccak256(code))
	} else {
		s.sm.Put(codeKey(addr), []byte(nil))
		s.sm.Put(codeHashKey(addr), []byte(nil))
	}
}

// Exists returns whether an account exists at the given address.
// See Account.IsEmpty()
func (s *State) Exists(addr thor.Address) bool {
	return s.GetBalance(addr).Sign() != 0 || (s.GetCodeHash(addr) != thor.Bytes32{})
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
	return newStage(s.root, s.db, changes)
}

type (
	energyKey thor.Address

	storageKey struct {
		addr thor.Address
		key  thor.Bytes32
	}

	codeKey     thor.Address
	codeHashKey thor.Address

	changedObject struct {
		data    Account
		storage map[thor.Bytes32][]byte
		code    []byte
	}
)
