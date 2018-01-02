package state

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	Trie "github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/kv"
)

type storage map[cry.Hash]cry.Hash

type account struct {
	Balance     *big.Int
	CodeHash    cry.Hash
	StorageRoot cry.Hash // merkle root of the storage trie
}

//cachedStorage saved all cached storages and the corresponding storage trie,
// if storage is set,both storage and storageTrie would be dirty,
//when Root() called,storage would not be dirty, but the storageTrie is still dirty until Commit() called
type cachedStorage struct {
	storage        storage
	isDirtyStorage bool
	storageTrie    *Trie.SecureTrie //this trie manages account storage data and it's root is storageRoot
	isDirtyTrie    bool
}

//cachedAccount it's for cache account
type cachedAccount struct {
	isDirty     bool //whether cached account should be updated to trie
	balance     *big.Int
	code        []byte   //contract code
	codeHash    cry.Hash //contract code hash
	storageRoot cry.Hash //storage root
}

//State manage account list
type State struct {
	trie           *Trie.SecureTrie //this trie manages all accounts data
	kv             kv.GetPutter
	cachedAccounts map[acc.Address]*cachedAccount
	cachedStorages map[acc.Address]*cachedStorage
	err            error
}

//New create new state
func New(root cry.Hash, kv kv.GetPutter) (s *State, err error) {
	hash := common.Hash(root)
	secureTrie, err := Trie.NewSecure(hash, kv, 0)
	if err != nil {
		return nil, err
	}
	return &State{
		secureTrie,
		kv,
		make(map[acc.Address]*cachedAccount),
		make(map[acc.Address]*cachedStorage),
		nil,
	}, nil
}

//Error return an Unhandled error
func (s *State) Error() error {
	return s.err
}

//GetBalance return balance from account address
func (s *State) GetBalance(addr acc.Address) *big.Int {
	a, err := s.getAccount(addr)
	if err != nil {
		s.err = err
		return new(big.Int)
	}
	return a.balance
}

//SetBalance Set account balance by address
func (s *State) SetBalance(addr acc.Address, balance *big.Int) {
	a, err := s.getAccount(addr)
	if err != nil {
		s.err = err
		return
	}
	a.isDirty = true
	a.balance = balance
}

//SetStorage set storage by address and key with value
func (s *State) SetStorage(addr acc.Address, key cry.Hash, value cry.Hash) {
	cs, err := s.getCachedStorage(addr)
	if err != nil {
		s.err = err
		return
	}
	cs.storage[key] = value
	cs.isDirtyStorage = true
}

//GetStorage return storage by address and key
func (s *State) GetStorage(addr acc.Address, key cry.Hash) cry.Hash {
	if cs, ok := s.cachedStorages[addr]; ok {
		if v, ok := cs.storage[key]; ok {
			return v
		}
	}
	cs, err := s.getCachedStorage(addr)
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	enc, err := cs.storageTrie.TryGet(key[:])
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	if len(enc) == 0 {
		return cry.Hash{}
	}
	_, content, _, err := rlp.Split(enc)
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	value := cry.BytesToHash(content)
	cs.storage[key] = value
	return value
}

//GetCode return code from account address
func (s *State) GetCode(addr acc.Address) []byte {
	a, err := s.getAccount(addr)
	if err != nil {
		s.err = err
		return nil
	}
	return a.code
}

//SetCode set code by address
func (s *State) SetCode(addr acc.Address, code []byte) {
	a, err := s.getAccount(addr)
	if err != nil {
		s.err = err
		return
	}
	codeHash := cry.BytesToHash(code)
	if err := s.kv.Put(codeHash[:], code); err != nil {
		s.err = err
	}
	a.isDirty = true
	a.codeHash = codeHash
	a.code = code
}

//Exists return whether account exists
func (s *State) Exists(addr acc.Address) bool {
	if _, ok := s.cachedAccounts[addr]; ok {
		return true
	}
	enc, err := s.trie.TryGet(addr[:])
	if err != nil {
		s.err = err
		return false
	}
	if len(enc) == 0 {
		return false
	}
	return true
}

// Delete removes any existing value for key from the trie.
func (s *State) Delete(address acc.Address) {
	delete(s.cachedAccounts, address)
	if err := s.trie.TryDelete(address[:]); err != nil {
		s.err = err
		return
	}
}

//if storagte trie exists returned else return a new trie from root
func (s *State) getCachedStorage(addr acc.Address) (*cachedStorage, error) {
	if cs, ok := s.cachedStorages[addr]; ok {
		return cs, nil
	}
	a, err := s.getAccount(addr)
	if err != nil {
		return nil, err
	}
	hash := common.Hash(a.storageRoot)
	secureTrie, err := Trie.NewSecure(hash, s.kv, 0)
	if err != nil {
		return nil, err
	}
	cs := &cachedStorage{
		make(storage),
		false,
		secureTrie,
		false,
	}
	s.cachedStorages[addr] = cs
	return cs, nil
}

func (s *State) updateStorage(cs *cachedStorage) error {
	for key, value := range cs.storage {
		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		if err := cs.storageTrie.TryUpdate(key[:], v); err != nil {
			s.err = err
			return err
		}
	}
	return nil
}

//update an account by address
func (s *State) updateAccount(address acc.Address, cachedAccount *cachedAccount) (err error) {
	a := &account{
		Balance:     cachedAccount.balance,
		CodeHash:    cachedAccount.codeHash,
		StorageRoot: cachedAccount.storageRoot,
	}
	enc, err := rlp.EncodeToBytes(a)
	if err != nil {
		return err
	}
	err = s.trie.TryUpdate(address[:], enc)
	if err != nil {
		s.err = err
		return
	}
	return nil
}

//getAccount return an account from address
func (s *State) getAccount(addr acc.Address) (*cachedAccount, error) {
	if a, ok := s.cachedAccounts[addr]; ok {
		return a, nil
	}
	enc, err := s.trie.TryGet(addr[:])
	if err != nil {
		s.err = err
		return nil, err
	}
	if len(enc) == 0 {
		s.cachedAccounts[addr] = &cachedAccount{
			isDirty:     false,
			balance:     new(big.Int),
			code:        nil,
			codeHash:    cry.BytesToHash(crypto.Keccak256(nil)),
			storageRoot: cry.Hash{},
		}
		return s.cachedAccounts[addr], nil
	}
	var data account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		return nil, err
	}
	dirtyAcc := &cachedAccount{
		isDirty:     false,
		balance:     data.Balance,
		code:        nil,
		codeHash:    data.CodeHash,
		storageRoot: data.StorageRoot,
	}
	if !bytes.Equal(dirtyAcc.codeHash[:], crypto.Keccak256(nil)) {
		code, err := s.kv.Get(dirtyAcc.codeHash[:])
		if err != nil {
			return nil, err
		}
		dirtyAcc.code = code
	}
	s.cachedAccounts[addr] = dirtyAcc
	return s.cachedAccounts[addr], nil
}

//whether an empty account
func isEmpty(a *cachedAccount) bool {
	return a.balance.Sign() == 0 && a.code == nil
}

//Commit commit data to update
func (s *State) Commit() cry.Hash {
	s.Root()
	for addr, cs := range s.cachedStorages {
		if cs.isDirtyTrie {
			if _, err := cs.storageTrie.Commit(); err != nil {
				s.err = err
				return cry.Hash{}
			}
		}
		delete(s.cachedStorages, addr)
	}
	for addr := range s.cachedAccounts {
		delete(s.cachedAccounts, addr)
	}
	root, err := s.trie.Commit()
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	return cry.Hash(root)
}

//Root get state trie root hash
func (s *State) Root() cry.Hash {
	//update dirty storage to storage trie
	for addr, cs := range s.cachedStorages {
		if cs.isDirtyStorage {
			a, err := s.getAccount(addr)
			if err != nil {
				s.err = err
				return cry.Hash{}
			}
			if isEmpty(a) {
				s.Delete(addr)
				continue
			}
			err = s.updateStorage(cs)
			if err != nil {
				s.err = err
				return cry.Hash{}
			}
			a.storageRoot = cry.Hash(cs.storageTrie.Hash())
			a.isDirty = true
			cs.isDirtyStorage = false
			cs.isDirtyTrie = true
		}
	}
	//update dirty account data to state trie
	for addr, a := range s.cachedAccounts {
		if isEmpty(a) {
			s.Delete(addr)
			continue
		}
		if a.isDirty {
			if err := s.updateAccount(addr, a); err != nil {
				s.err = err
				return cry.Hash{}
			}
			a.isDirty = false
		}
	}
	return cry.Hash(s.trie.Hash())
}
