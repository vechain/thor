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
	"github.com/vechain/thor/lowrlp"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/stackedmap"
	"github.com/vechain/thor/thor"
)

const (
	// AccountTrieName is the name of account trie.
	AccountTrieName       = "a"
	StorageTrieNamePrefix = "s"

	codeStoreName = "state.code"
)

// StorageTrieName converts the storage id into the name of storage trie.
// It'll panic when empty sid passed.
func StorageTrieName(sid []byte) string {
	if len(sid) == 0 {
		panic("empty storage id")
	}
	return StorageTrieNamePrefix + string(sid)
}

// Error is the error caused by state access failure.
type Error struct {
	cause error
}

func (e *Error) Error() string {
	return fmt.Sprintf("state: %v", e.cause)
}

// State manages the world state.
type State struct {
	db             *muxdb.MuxDB
	trie           *muxdb.Trie                    // the accounts trie reader
	cache          map[thor.Address]*cachedObject // cache of accounts trie
	sm             *stackedmap.StackedMap         // keeps revisions of accounts state
	steadyBlockNum uint32
}

// New create state object.
func New(db *muxdb.MuxDB, root thor.Bytes32, blockNum, blockConflicts, steadyBlockNum uint32) *State {
	state := State{
		db:             db,
		trie:           db.NewTrie(AccountTrieName, root, blockNum, blockConflicts),
		cache:          make(map[thor.Address]*cachedObject),
		steadyBlockNum: steadyBlockNum,
	}

	state.sm = stackedmap.New(func(key interface{}) (interface{}, bool, error) {
		return state.cacheGetter(key)
	})
	return &state
}

// Checkout checkouts to another state.
func (s *State) Checkout(root thor.Bytes32, blockNum, blockConflicts, steadyBlockNum uint32) *State {
	return New(s.db, root, blockNum, blockConflicts, steadyBlockNum)
}

// cacheGetter implements stackedmap.MapGetter.
func (s *State) cacheGetter(key interface{}) (value interface{}, exist bool, err error) {
	switch k := key.(type) {
	case thor.Address: // get account
		obj, err := s.getCachedObject(k)
		if err != nil {
			return nil, false, err
		}
		return &obj.data, true, nil
	case codeKey: // get code
		obj, err := s.getCachedObject(thor.Address(k))
		if err != nil {
			return nil, false, err
		}
		code, err := obj.GetCode()
		if err != nil {
			return nil, false, err
		}
		return code, true, nil
	case storageKey: // get storage
		// the address was ever deleted in the life-cycle of this state instance.
		// treat its storage as an empty set.
		if k.barrier != 0 {
			return rlp.RawValue(nil), true, nil
		}

		obj, err := s.getCachedObject(k.addr)
		if err != nil {
			return nil, false, err
		}
		v, err := obj.GetStorage(k.key, s.steadyBlockNum)
		if err != nil {
			return nil, false, err
		}
		return v, true, nil
	case storageBarrierKey: // get barrier, 0 as initial value
		return 0, true, nil
	}
	panic(fmt.Errorf("unexpected key type %+v", key))
}

func (s *State) getCachedObject(addr thor.Address) (*cachedObject, error) {
	if co, ok := s.cache[addr]; ok {
		return co, nil
	}
	a, am, err := loadAccount(s.trie, addr, s.steadyBlockNum)
	if err != nil {
		return nil, err
	}
	co := newCachedObject(s.db, addr, a, am)
	s.cache[addr] = co
	return co, nil
}

// getAccount gets account by address. the returned account should not be modified.
func (s *State) getAccount(addr thor.Address) (*Account, error) {
	v, _, err := s.sm.Get(addr)
	if err != nil {
		return nil, err
	}
	return v.(*Account), nil
}

// getAccountCopy get a copy of account by address.
func (s *State) getAccountCopy(addr thor.Address) (Account, error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return Account{}, err
	}
	return *acc, nil
}

func (s *State) updateAccount(addr thor.Address, acc *Account) {
	s.sm.Put(addr, acc)
}

func (s *State) getStorageBarrier(addr thor.Address) int {
	b, _, _ := s.sm.Get(storageBarrierKey(addr))
	return b.(int)
}

func (s *State) setStorageBarrier(addr thor.Address, barrier int) {
	s.sm.Put(storageBarrierKey(addr), barrier)
}

// GetBalance returns balance for the given address.
func (s *State) GetBalance(addr thor.Address) (*big.Int, error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return nil, &Error{err}
	}
	return acc.Balance, nil
}

// SetBalance set balance for the given address.
func (s *State) SetBalance(addr thor.Address, balance *big.Int) error {
	cpy, err := s.getAccountCopy(addr)
	if err != nil {
		return &Error{err}
	}
	cpy.Balance = balance
	s.updateAccount(addr, &cpy)
	return nil
}

// GetEnergy get energy for the given address at block number specified.
func (s *State) GetEnergy(addr thor.Address, blockTime uint64) (*big.Int, error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return nil, &Error{err}
	}
	return acc.CalcEnergy(blockTime), nil
}

// SetEnergy set energy at block number for the given address.
func (s *State) SetEnergy(addr thor.Address, energy *big.Int, blockTime uint64) error {
	cpy, err := s.getAccountCopy(addr)
	if err != nil {
		return &Error{err}
	}
	cpy.Energy, cpy.BlockTime = energy, blockTime
	s.updateAccount(addr, &cpy)
	return nil
}

// GetMaster get master for the given address.
// Master can move energy, manage users...
func (s *State) GetMaster(addr thor.Address) (thor.Address, error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return thor.Address{}, &Error{err}
	}
	return thor.BytesToAddress(acc.Master), nil
}

// SetMaster set master for the given address.
func (s *State) SetMaster(addr thor.Address, master thor.Address) error {
	cpy, err := s.getAccountCopy(addr)
	if err != nil {
		return &Error{err}
	}
	if master.IsZero() {
		cpy.Master = nil
	} else {
		cpy.Master = master[:]
	}
	s.updateAccount(addr, &cpy)
	return nil
}

// GetStorage returns storage value for the given address and key.
func (s *State) GetStorage(addr thor.Address, key thor.Bytes32) (thor.Bytes32, error) {
	raw, err := s.GetRawStorage(addr, key)
	if err != nil {
		return thor.Bytes32{}, &Error{err}
	}
	if len(raw) == 0 {
		return thor.Bytes32{}, nil
	}
	kind, content, _, err := rlp.Split(raw)
	if err != nil {
		return thor.Bytes32{}, &Error{err}
	}
	if kind == rlp.List {
		// special case for rlp list, it should be customized storage value
		// return hash of raw data
		return thor.Blake2b(raw), nil
	}
	return thor.BytesToBytes32(content), nil
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
func (s *State) GetRawStorage(addr thor.Address, key thor.Bytes32) (rlp.RawValue, error) {
	data, _, err := s.sm.Get(storageKey{addr, s.getStorageBarrier(addr), key})
	if err != nil {
		return nil, &Error{err}
	}
	return data.(rlp.RawValue), nil
}

// SetRawStorage set storage value in rlp raw.
func (s *State) SetRawStorage(addr thor.Address, key thor.Bytes32, raw rlp.RawValue) {
	s.sm.Put(storageKey{addr, s.getStorageBarrier(addr), key}, raw)
}

// EncodeStorage set storage value encoded by given enc method.
// Error returned by end will be absorbed by State instance.
func (s *State) EncodeStorage(addr thor.Address, key thor.Bytes32, enc func() ([]byte, error)) error {
	raw, err := enc()
	if err != nil {
		return &Error{err}
	}
	s.SetRawStorage(addr, key, raw)
	return nil
}

// DecodeStorage get and decode storage value.
// Error returned by dec will be absorbed by State instance.
func (s *State) DecodeStorage(addr thor.Address, key thor.Bytes32, dec func([]byte) error) error {
	raw, err := s.GetRawStorage(addr, key)
	if err != nil {
		return &Error{err}
	}
	if err := dec(raw); err != nil {
		return &Error{err}
	}
	return nil
}

// GetCode returns code for the given address.
func (s *State) GetCode(addr thor.Address) ([]byte, error) {
	v, _, err := s.sm.Get(codeKey(addr))
	if err != nil {
		return nil, &Error{err}
	}
	return v.([]byte), nil
}

// GetCodeHash returns code hash for the given address.
func (s *State) GetCodeHash(addr thor.Address) (thor.Bytes32, error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return thor.Bytes32{}, &Error{err}
	}
	return thor.BytesToBytes32(acc.CodeHash), nil
}

// SetCode set code for the given address.
func (s *State) SetCode(addr thor.Address, code []byte) error {
	var codeHash []byte
	if len(code) > 0 {
		s.sm.Put(codeKey(addr), code)
		codeHash = crypto.Keccak256(code)
		codeCache.Add(string(codeHash), code)
	} else {
		s.sm.Put(codeKey(addr), []byte(nil))
	}
	cpy, err := s.getAccountCopy(addr)
	if err != nil {
		return &Error{err}
	}
	cpy.CodeHash = codeHash
	s.updateAccount(addr, &cpy)
	return nil
}

// Exists returns whether an account exists at the given address.
// See Account.IsEmpty()
func (s *State) Exists(addr thor.Address) (bool, error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return false, &Error{err}
	}
	return !acc.IsEmpty(), nil
}

// Delete delete an account at the given address.
// That's set balance, energy and code to zero value.
func (s *State) Delete(addr thor.Address) {
	s.sm.Put(codeKey(addr), []byte(nil))
	s.updateAccount(addr, emptyAccount())
	// increase the barrier value
	s.setStorageBarrier(addr, s.getStorageBarrier(addr)+1)
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

// BuildStorageTrie build up storage trie for given address with cumulative changes.
func (s *State) BuildStorageTrie(addr thor.Address) (trie *muxdb.Trie, err error) {
	acc, err := s.getAccount(addr)
	if err != nil {
		return nil, &Error{err}
	}

	if len(acc.StorageRoot) > 0 {
		obj, err := s.getCachedObject(addr)
		if err != nil {
			return nil, &Error{err}
		}
		trie = s.db.NewTrie(
			StorageTrieName(obj.meta.StorageID),
			thor.BytesToBytes32(acc.StorageRoot),
			obj.meta.StorageCommitNum,
			obj.meta.StorageDistinctNum)
	} else {
		trie = s.db.NewTrie(
			"",
			thor.Bytes32{},
			0,
			0,
		)
	}

	barrier := s.getStorageBarrier(addr)

	// traverse journal to filter out storage changes for addr
	s.sm.Journal(func(k, v interface{}) bool {
		switch key := k.(type) {
		case storageKey:
			if key.barrier == barrier && key.addr == addr {
				err = saveStorage(trie, key.key, v.(rlp.RawValue))
				if err != nil {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, &Error{err}
	}
	return trie, nil
}

// Stage makes a stage object to compute hash of trie or commit all changes.
func (s *State) Stage(newBlockNum, newBlockConflicts uint32) (*Stage, error) {
	type changed struct {
		data            Account
		meta            AccountMetadata
		storage         map[thor.Bytes32]rlp.RawValue
		baseStorageTrie *muxdb.Trie
	}

	var (
		changes = make(map[thor.Address]*changed)
		codes   = make(map[thor.Bytes32][]byte)

		storageTrieCreationCount uint64
	)

	// get or create changed account
	getChanged := func(addr thor.Address) (*changed, error) {
		if obj, ok := changes[addr]; ok {
			return obj, nil
		}
		co, err := s.getCachedObject(addr)
		if err != nil {
			return nil, &Error{err}
		}

		c := &changed{data: co.data, meta: co.meta, baseStorageTrie: co.cache.storageTrie}
		changes[addr] = c
		return c, nil
	}

	var jerr error
	// traverse journal to build changes
	s.sm.Journal(func(k, v interface{}) bool {
		var c *changed
		switch key := k.(type) {
		case thor.Address:
			if c, jerr = getChanged(key); jerr != nil {
				return false
			}
			c.data = *(v.(*Account))
		case codeKey:
			code := v.([]byte)
			if len(code) > 0 {
				codes[thor.Bytes32(crypto.Keccak256Hash(code))] = code
			}
		case storageKey:
			if c, jerr = getChanged(key.addr); jerr != nil {
				return false
			}
			if c.storage == nil {
				c.storage = make(map[thor.Bytes32]rlp.RawValue)
			}
			c.storage[key.key] = v.(rlp.RawValue)
			if len(c.meta.StorageID) == 0 {
				// generate storage id for the new storage trie.
				var enc lowrlp.Encoder
				enc.EncodeUint(uint64(newBlockNum))
				enc.EncodeUint(uint64(newBlockConflicts))
				enc.EncodeUint(uint64(storageTrieCreationCount))
				storageTrieCreationCount++
				c.meta.StorageID = enc.ToBytes()
			}
		case storageBarrierKey:
			if c, jerr = getChanged(thor.Address(key)); jerr != nil {
				return false
			}
			// discard all storage updates and base storage trie when meet the barrier.
			c.storage = nil
			c.baseStorageTrie = nil
			c.meta = AccountMetadata{}
		}
		return true
	})
	if jerr != nil {
		return nil, &Error{jerr}
	}

	trieCpy := s.trie.Copy()
	commits := make([]func() error, 0, len(changes)+2)

	for addr, c := range changes {
		// skip storage changes if account is empty
		if !c.data.IsEmpty() {
			if len(c.storage) > 0 {
				var sTrie *muxdb.Trie
				if c.baseStorageTrie != nil {
					sTrie = c.baseStorageTrie.Copy()
				} else {
					sTrie = s.db.NewTrie(
						StorageTrieName(c.meta.StorageID),
						thor.BytesToBytes32(c.data.StorageRoot),
						c.meta.StorageCommitNum,
						c.meta.StorageDistinctNum)
				}
				for k, v := range c.storage {
					if err := saveStorage(sTrie, k, v); err != nil {
						return nil, &Error{err}
					}
				}
				sRoot, commit := sTrie.Stage(newBlockNum, newBlockConflicts)
				c.data.StorageRoot = sRoot[:]
				c.meta.StorageCommitNum = newBlockNum
				c.meta.StorageDistinctNum = newBlockConflicts
				commits = append(commits, commit)
			}
		}
		if err := saveAccount(trieCpy, addr, &c.data, &c.meta); err != nil {
			return nil, &Error{err}
		}
	}
	root, commitAcc := trieCpy.Stage(newBlockNum, newBlockConflicts)
	commitCodes := func() error {
		if len(codes) > 0 {
			bulk := s.db.NewStore(codeStoreName).Bulk()
			for hash, code := range codes {
				if err := bulk.Put(hash[:], code); err != nil {
					return err
				}
			}
			return bulk.Write()
		}
		return nil
	}
	commits = append(commits, commitAcc, commitCodes)

	return &Stage{
		root:    root,
		commits: commits,
	}, nil
}

type (
	storageKey struct {
		addr    thor.Address
		barrier int
		key     thor.Bytes32
	}
	codeKey           thor.Address
	storageBarrierKey thor.Address
)
