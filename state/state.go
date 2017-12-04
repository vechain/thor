package state

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	Trie "github.com/ethereum/go-ethereum/trie"
	log "github.com/sirupsen/logrus"
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
	"github.com/vechain/vecore/kv"
)

//State manage account list
type State struct {
	trie *Trie.SecureTrie
	db   kv.Store
}

//NewState create new storage trie
func NewState(root cry.Hash, db kv.Store) (state *State, err error) {
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
func (state *State) GetAccount(address acc.Address) acc.Account {
	account, err := state.getAccountByAddress(address)
	if err != nil {
		return acc.Account{
			Balance:     new(big.Int),
			CodeHash:    cry.Hash{},
			StorageRoot: cry.Hash{},
		}
	}
	return *account
}

//GetAccountByAddress get account by address
func (state *State) getAccountByAddress(address acc.Address) (account *acc.Account, err error) {
	enc, err := state.Get(address[:])
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

//GetAccountByAddressHash get account by addressHash
func (state *State) getAccountByAddressHash(addressHash cry.Hash) (account *acc.Account, err error) {
	enc, err := state.Get(addressHash[:])
	if err != nil || len(enc) == 0 {
		log.Error("GetState error nil enc:", err)
		return nil, err
	}
	var data acc.Account
	err = rlp.DecodeBytes(enc, &data)
	if err != nil {
		log.Error("GetState error decode enc:", err)
		return nil, err
	}
	return &data, nil
}

//UpdateAccount update account by address
func (state *State) UpdateAccount(address acc.Address, account *acc.Account) (err error) {
	enc, err := rlp.EncodeToBytes(*account)
	if err != nil {
		log.Error("UpdateAccount error:", err)
		return err
	}
	return state.Put(address[:], enc)
}

//UpdateStorage update account storage
func (state *State) UpdateStorage(key cry.Hash, value cry.Hash) error {
	return state.Put(key[:], value[:])
}

//GetStorage get account storage
func (state *State) GetStorage(key cry.Hash) (value cry.Hash) {
	enc, err := state.Get(key[:])
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
func (state *State) Get(key []byte) ([]byte, error) {
	return state.trie.TryGet(key)
}

//Put key value
func (state *State) Put(key []byte, value []byte) error {
	return state.trie.TryUpdate(key, value)
}

// Delete removes any existing value for key from the trie.
func (state *State) Delete(key []byte) error {
	return state.trie.TryDelete(key)
}

//Commit commit data to update
func (state *State) Commit() (root cry.Hash, err error) {
	hash, err := state.trie.CommitTo(state.db)
	if err != nil {
		return cry.Hash(common.Hash{}), err
	}
	return cry.Hash(hash), nil
}

//Root get storage trie root
func (state *State) Root() []byte {
	return state.trie.Root()
}

//Hash get storage trie root hash
func (state *State) Hash() cry.Hash {
	return cry.Hash(state.trie.Hash())
}
