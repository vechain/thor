package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	Trie "github.com/ethereum/go-ethereum/trie"
	log "github.com/sirupsen/logrus"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/kv"
)

//State manage account list
type State struct {
	trie *Trie.SecureTrie
	db   kv.Store
}

//New create new state
func New(root cry.Hash, db kv.Store) (s *State, err error) {
	hash := common.Hash(root)
	secureTrie, err := Trie.NewSecure(hash, db, 0)
	if err != nil {
		return nil, err
	}
	return &State{
		trie: secureTrie,
		db:   db,
	}, nil
}

//GetAccount return account from address
func (s *State) GetAccount(address acc.Address) *acc.Account {
	account, err := s.getAccount(address)
	if err != nil {
		return nil
	}
	return account
}

//GetAccountByAddress get account by address
func (s *State) getAccount(address acc.Address) (account *acc.Account, err error) {
	enc, err := s.Get(address[:])
	if err != nil {
		log.Error("GetState error nil enc:", err)
		return nil, err
	}
	var data acc.Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		log.Error("GetState error decode enc:", err)
		return nil, err
	}
	return &data, nil
}

//UpdateAccount update account by address
func (s *State) UpdateAccount(address acc.Address, account *acc.Account) (err error) {
	enc, err := rlp.EncodeToBytes(*account)
	if err != nil {
		log.Error("UpdateAccount error:", err)
		return err
	}
	return s.Put(address[:], enc)
}

//UpdateStorage update account storage
func (s *State) UpdateStorage(key cry.Hash, value cry.Hash) error {
	return s.Put(key[:], value[:])
}

//GetStorage get account storage
func (s *State) GetStorage(key cry.Hash) (value cry.Hash) {
	enc, err := s.Get(key[:])
	if err != nil {
		return cry.Hash{}
	}
	_, content, _, err := rlp.Split(enc)
	if err != nil {
		return cry.Hash{}
	}
	value = cry.BytesToHash(content)
	return value
}

//Get key value
func (s *State) Get(key []byte) ([]byte, error) {
	return s.trie.TryGet(key)
}

//Put key value
func (s *State) Put(key []byte, value []byte) error {
	return s.trie.TryUpdate(key, value)
}

// Delete removes any existing value for key from the trie.
func (s *State) Delete(key []byte) error {
	return s.trie.TryDelete(key)
}

//Commit commit data to update
func (s *State) Commit() (root cry.Hash, err error) {
	hash, err := s.trie.CommitTo(s.db)
	if err != nil {
		return cry.Hash(common.Hash{}), err
	}
	return cry.Hash(hash), nil
}

//Root get storage trie root
func (s *State) Root() []byte {
	return s.trie.Root()
}

//Hash get storage trie root hash
func (s *State) Hash() cry.Hash {
	return cry.Hash(s.trie.Hash())
}
