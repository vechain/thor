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
	balance     *big.Int
	codeHash    cry.Hash
	storageRoot cry.Hash
	code        []byte
	storage     storage
}

//State manage account list
type State struct {
	trie          *Trie.SecureTrie
	kv            kv.GetPutter
	storageTries  map[cry.Hash]*Trie.SecureTrie
	dirtyAccounts map[acc.Address]*cachedAccount
	stateErr      error
}

//New create new state
func New(root cry.Hash, kv kv.GetPutter) (s *State, err error) {
	hash := common.Hash(root)
	secureTrie, err := Trie.NewSecure(hash, kv, 0)
	if err != nil {
		return nil, err
	}
	return &State{
		trie:          secureTrie,
		kv:            kv,
		storageTries:  make(map[cry.Hash]*Trie.SecureTrie),
		dirtyAccounts: make(map[acc.Address]*cachedAccount),
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
	s.dirtyAccounts[addr].balance = balance
}

//SetStorage set storage by address and key with value
func (s *State) SetStorage(addr acc.Address, key cry.Hash, value cry.Hash) {
	if _, err := s.getAccount(addr); err != nil {
		s.stateErr = err
		return
	}
	s.dirtyAccounts[addr].storage[key] = value
}

//GetStorage return storage by address and key
func (s *State) GetStorage(addr acc.Address, key cry.Hash) cry.Hash {
	if account, ok := s.dirtyAccounts[addr]; ok {
		if _, ok := account.storage[key]; ok {
			return account.storage[key]
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
	return value
}

//GetCode return code from account address
func (s *State) GetCode(addr acc.Address) []byte {
	if a, ok := s.dirtyAccounts[addr]; ok {
		return a.code
	}
	a, err := s.getAccount(addr)
	if err != nil {
		s.stateErr = err
		return []byte{}
	}
	if bytes.Equal(a.codeHash[:], crypto.Keccak256(nil)) {
		return []byte{}
	}
	code, err := s.kv.Get(a.codeHash[:])
	if err != nil {
		s.stateErr = err
		return []byte{}
	}
	return code[:]
}

//SetCode set code by address
func (s *State) SetCode(addr acc.Address, code []byte) {
	if _, err := s.getAccount(addr); err != nil {
		s.stateErr = err
		return
	}
	s.dirtyAccounts[addr].code = code
	s.dirtyAccounts[addr].codeHash = cry.BytesToHash(code)
}

// Delete removes any existing value for key from the trie.
func (s *State) Delete(address acc.Address) error {
	return s.trie.TryDelete(address[:])
}

//if trie exists returned else return a new trie from root
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

func (s *State) localizeCode(dirtyAccount *cachedAccount) {
	if err := s.kv.Put(dirtyAccount.codeHash[:], dirtyAccount.code); err != nil {
		s.stateErr = err
	}
}

func (s *State) localizeStorage(dirtyAccount *cachedAccount) {
	st, err := s.getOrCreateNewTrie(dirtyAccount.storageRoot)
	if err != nil {
		s.stateErr = err
		return
	}
	for key, value := range dirtyAccount.storage {
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
	dirtyAccount.storageRoot = cry.Hash(root)
}

//update an account by address
func (s *State) updateAccount(address acc.Address, dirtyAccount *cachedAccount) (err error) {
	a := &account{
		Balance:     dirtyAccount.balance,
		CodeHash:    dirtyAccount.codeHash,
		StorageRoot: dirtyAccount.storageRoot,
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
	if a, ok := s.dirtyAccounts[addr]; ok {
		return a, nil
	}
	enc, err := s.trie.TryGet(addr[:])
	if err != nil {
		return nil, err
	}
	if len(enc) == 0 {
		s.dirtyAccounts[addr] = &cachedAccount{
			balance:     new(big.Int),
			codeHash:    cry.Hash{},
			storageRoot: cry.Hash{},
			storage:     make(storage),
			code:        []byte{},
		}
		return s.dirtyAccounts[addr], nil
	}
	var data account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		return nil, err
	}
	dirtyAcc := &cachedAccount{
		balance:     data.Balance,
		codeHash:    data.CodeHash,
		storageRoot: data.StorageRoot,
		storage:     make(storage),
		code:        []byte{},
	}
	if !bytes.Equal(dirtyAcc.codeHash[:], crypto.Keccak256(nil)) {
		code, err := s.kv.Get(dirtyAcc.codeHash[:])
		if err != nil {
			return nil, err
		}
		dirtyAcc.code = code
	}
	s.dirtyAccounts[addr] = dirtyAcc
	return s.dirtyAccounts[addr], nil
}

//Commit commit data to update
func (s *State) Commit() {
	for addr, account := range s.dirtyAccounts {
		s.localizeCode(account)
		s.localizeStorage(account)
		s.updateAccount(addr, account)
		_, err := s.trie.CommitTo(s.kv)
		if err != nil {
			s.stateErr = err
			return
		}
		if s.stateErr != nil {
			return
		}
		delete(s.dirtyAccounts, addr)
	}
}

//Root get storage trie root hash
func (s *State) Root() cry.Hash {
	return cry.Hash(s.trie.Hash())
}
