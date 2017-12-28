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

//cachedAccount it's for cache account
type cachedAccount struct {
	isDirty        bool //is cached account should update
	isStorageDirty bool
	balance        *big.Int
	code           []byte
	codeHash       cry.Hash
	storageRoot    cry.Hash
	storage        storage          //dirty storage
	storageTrie    *Trie.SecureTrie //this trie manages account storage data and it's root is storageRoot
}

//State manage account list
type State struct {
	trie           *Trie.SecureTrie //this trie manages all accounts data
	kv             kv.GetPutter
	cachedAccounts map[acc.Address]*cachedAccount
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
	a, err := s.getAccount(addr)
	if err != nil {
		s.err = err
		return
	}
	a.isStorageDirty = true
	a.storage[key] = value
}

//GetStorage return storage by address and key
func (s *State) GetStorage(addr acc.Address, key cry.Hash) cry.Hash {
	if a, ok := s.cachedAccounts[addr]; ok {
		if value, ok := a.storage[key]; ok {
			return value
		}
	}
	a, err := s.getAccount(addr)
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	st, err := s.getTrie(addr)
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	enc, err := st.TryGet(key[:])
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	_, content, _, err := rlp.Split(enc)
	if err != nil {
		s.err = err
		return cry.Hash{}
	}
	value := cry.BytesToHash(content)
	a.storage[key] = value
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
		if _, ok := err.(*Trie.MissingNodeError); !ok {
			s.err = err
			return
		}
	}
}

//if storagte trie exists returned else return a new trie from root
func (s *State) getTrie(addr acc.Address) (*Trie.SecureTrie, error) {
	trie := s.cachedAccounts[addr].storageTrie
	if trie != nil {
		return trie, nil
	}
	hash := common.Hash(s.cachedAccounts[addr].storageRoot)
	secureTrie, err := Trie.NewSecure(hash, s.kv, 0)
	if err != nil {
		return nil, err
	}
	s.cachedAccounts[addr].storageTrie = secureTrie
	return secureTrie, nil
}

func (s *State) updateStorage(addr acc.Address, cachedAccount *cachedAccount) (*Trie.SecureTrie, error) {
	st, err := s.getTrie(addr)
	if err != nil {
		return nil, err
	}
	for key, value := range cachedAccount.storage {
		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		e := st.TryUpdate(key[:], v)
		if e != nil {
			return nil, err
		}
		delete(cachedAccount.storage, key)
	}

	return st, nil
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
		return err
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
		return nil, err
	}
	if len(enc) == 0 {
		s.cachedAccounts[addr] = &cachedAccount{
			isDirty:        false,
			isStorageDirty: false,
			balance:        new(big.Int),
			code:           nil,
			codeHash:       cry.BytesToHash(crypto.Keccak256(nil)),
			storageRoot:    cry.Hash{},
			storage:        make(storage),
		}
		return s.cachedAccounts[addr], nil
	}
	var data account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		return nil, err
	}
	dirtyAcc := &cachedAccount{
		isDirty:        false,
		isStorageDirty: false,
		balance:        data.Balance,
		code:           nil,
		codeHash:       data.CodeHash,
		storageRoot:    data.StorageRoot,
		storage:        make(storage),
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
	for addr, account := range s.cachedAccounts {
		if isEmpty(account) { //delete empty(account)
			s.Delete(addr)
			continue
		}
		if account.isStorageDirty { //storage is still dirty , should update to trie
			trie, err := s.updateStorage(addr, account)
			if err != nil {
				s.err = err
				return cry.Hash{}
			}
			account.storageRoot = cry.Hash(trie.Hash())
			account.isDirty = true
		}
		if account.storageTrie != nil { //has been updated to trie
			if _, err := account.storageTrie.Commit(); err != nil {
				s.err = err
				return cry.Hash{}
			}
		}
		if account.isDirty { //if account dirty,update it to trie
			if err := s.updateAccount(addr, account); err != nil {
				s.err = err
				return cry.Hash{}
			}
		}
		//commit account data to kv
		if _, err := s.trie.Commit(); err != nil {
			s.err = err
			return cry.Hash{}
		}
		delete(s.cachedAccounts, addr)
	}
	return cry.Hash(s.trie.Hash())
}

//Root get state trie root hash
func (s *State) Root() cry.Hash {
	for addr, account := range s.cachedAccounts {
		if isEmpty(account) {
			s.Delete(addr)
			continue
		}
		if account.isStorageDirty { //storage has been set,should update to trie
			trie, err := s.updateStorage(addr, account)
			if err != nil {
				s.err = err
				return cry.Hash{}
			}
			account.storageRoot = cry.Hash(trie.Hash())
			account.isDirty = true
		}
		if account.isDirty { //if account still dirty,update it to trie
			if err := s.updateAccount(addr, account); err != nil {
				s.err = err
				return cry.Hash{}
			}
			account.isDirty = false
		}
	}
	return cry.Hash(s.trie.Hash())
}
