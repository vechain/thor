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

type cachedAccount struct {
	isDirty     bool
	balance     *big.Int
	codeHash    cry.Hash
	storageRoot cry.Hash
	storage     storage
}

type cachedCode struct {
	isDirty  bool
	code     []byte
	codeHash cry.Hash
}

//State manage account list
type State struct {
	trie           *Trie.SecureTrie
	kv             kv.GetPutter
	storageTries   map[cry.Hash]*Trie.SecureTrie
	cachedAccounts map[acc.Address]*cachedAccount
	cachedCode     map[acc.Address]*cachedCode
	stateErr       error
}

//New create new state
func New(root cry.Hash, kv kv.GetPutter) (s *State, err error) {
	hash := common.Hash(root)
	secureTrie, err := Trie.NewSecure(hash, kv, 0)
	if err != nil {
		return nil, err
	}
	return &State{
		trie:           secureTrie,
		kv:             kv,
		storageTries:   make(map[cry.Hash]*Trie.SecureTrie),
		cachedAccounts: make(map[acc.Address]*cachedAccount),
		cachedCode:     make(map[acc.Address]*cachedCode),
	}, nil
}

//Error return an Unhandled error
func (s *State) Error() error {
	return s.stateErr
}

//GetBalance return balance from account address
func (s *State) GetBalance(addr acc.Address) *big.Int {
	a, err := s.getAccount(addr)
	if err != nil {
		s.stateErr = err
		return new(big.Int)
	}
	return a.balance
}

//SetBalance Set account balance by address
func (s *State) SetBalance(addr acc.Address, balance *big.Int) {
	if _, err := s.getAccount(addr); err != nil {
		s.stateErr = err
		return
	}
	if s.cachedAccounts[addr].balance != balance {
		s.cachedAccounts[addr].isDirty = true
		s.cachedAccounts[addr].balance = balance
	}
}

//SetStorage set storage by address and key with value
func (s *State) SetStorage(addr acc.Address, key cry.Hash, value cry.Hash) {
	if _, err := s.getAccount(addr); err != nil {
		s.stateErr = err
		return
	}
	if s.cachedAccounts[addr].storage[key] != value {
		s.cachedAccounts[addr].isDirty = true
		s.cachedAccounts[addr].storage[key] = value
	}
}

//GetStorage return storage by address and key
func (s *State) GetStorage(addr acc.Address, key cry.Hash) cry.Hash {
	if account, ok := s.cachedAccounts[addr]; ok {
		if value, ok := account.storage[key]; ok {
			return value
		}
	}
	a, err := s.getAccount(addr)
	if err != nil {
		s.stateErr = err
		return cry.Hash{}
	}
	st, err := s.getOrCreateNewTrie(a.storageRoot)
	if err != nil {
		s.stateErr = err
		return cry.Hash{}
	}
	enc, err := st.TryGet(key[:])
	if err != nil {
		s.stateErr = err
		return cry.Hash{}
	}
	_, content, _, err := rlp.Split(enc)
	if err != nil {
		s.stateErr = err
		return cry.Hash{}
	}
	value := cry.BytesToHash(content)
	s.cachedAccounts[addr].storage[key] = value
	return value
}

//GetCode return code from account address
func (s *State) GetCode(addr acc.Address) []byte {
	if cm, ok := s.cachedCode[addr]; ok {
		return cm.code
	}
	if _, err := s.getAccount(addr); err != nil {
		s.stateErr = err
		return []byte{}
	}
	return s.cachedCode[addr].code
}

//SetCode set code by address
func (s *State) SetCode(addr acc.Address, code []byte) {
	if _, err := s.getAccount(addr); err != nil {
		s.stateErr = err
		return
	}
	if cachedCode, ok := s.cachedCode[addr]; ok {
		if !bytes.Equal(cachedCode.code, code) {
			s.cachedCode[addr].code = code
			s.cachedCode[addr].isDirty = true
			s.cachedAccounts[addr].codeHash = cry.BytesToHash(code)
			s.cachedAccounts[addr].isDirty = true
		}
	}
}

// Delete removes any existing value for key from the trie.
func (s *State) Delete(address acc.Address) error {
	return s.trie.TryDelete(address[:])
}

//if storagte trie exists returned else return a new trie from root
func (s *State) getOrCreateNewTrie(root cry.Hash) (trie *Trie.SecureTrie, err error) {
	trie, ok := s.storageTries[root]
	if !ok {
		hash := common.Hash(root)
		secureTrie, err := Trie.NewSecure(hash, s.kv, 0)
		if err != nil {
			return nil, err
		}
		s.storageTries[root] = secureTrie.Copy()
	}
	return s.storageTries[root], nil
}

func (s *State) localizeCode(cachedCode *cachedCode) {
	if err := s.kv.Put(cachedCode.codeHash[:], cachedCode.code); err != nil {
		s.stateErr = err
	}
}

func (s *State) localizeStorage(cachedAccount *cachedAccount) {
	st, err := s.getOrCreateNewTrie(cachedAccount.storageRoot)
	if err != nil {
		s.stateErr = err
		return
	}
	for key, value := range cachedAccount.storage {
		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		e := st.TryUpdate(key[:], v)
		if e != nil {
			s.stateErr = err
			return
		}
	}
	root, err := st.CommitTo(s.kv)
	if err != nil {
		s.stateErr = err
		return
	}
	cachedAccount.storageRoot = cry.Hash(root)
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
		s.stateErr = err
		return err
	}
	err = s.trie.TryUpdate(address[:], enc)
	if err != nil {
		s.stateErr = err
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
			isDirty:     false,
			balance:     new(big.Int),
			codeHash:    cry.BytesToHash(crypto.Keccak256(nil)),
			storageRoot: cry.Hash{},
			storage:     make(storage),
		}
		s.cachedCode[addr] = &cachedCode{
			isDirty: false,
			code:    []byte{},
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
		codeHash:    data.CodeHash,
		storageRoot: data.StorageRoot,
		storage:     make(storage),
	}
	if bytes.Equal(dirtyAcc.codeHash[:], crypto.Keccak256(nil)) {
		s.cachedCode[addr] = &cachedCode{
			isDirty: false,
			code:    []byte{},
		}
	} else {
		code, err := s.kv.Get(dirtyAcc.codeHash[:])
		if err != nil {
			return nil, err
		}
		s.cachedCode[addr] = &cachedCode{
			isDirty: false,
			code:    code,
		}
	}
	s.cachedAccounts[addr] = dirtyAcc
	return s.cachedAccounts[addr], nil
}

//Commit commit data to update
func (s *State) Commit() {
	for addr, account := range s.cachedAccounts {
		if cachedCode, ok := s.cachedCode[addr]; ok {
			if cachedCode.isDirty {
				s.localizeCode(cachedCode)
				delete(s.cachedCode, addr)
			}
		}
		if account.isDirty {
			s.localizeStorage(account)
			s.updateAccount(addr, account)
		}
		_, err := s.trie.CommitTo(s.kv)
		if err != nil {
			s.stateErr = err
			return
		}
		if s.stateErr != nil {
			return
		}
		delete(s.cachedAccounts, addr)
	}
}

//Root get storage trie root hash
func (s *State) Root() cry.Hash {
	return cry.Hash(s.trie.Hash())
}
