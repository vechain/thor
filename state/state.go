package state

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	Trie "github.com/ethereum/go-ethereum/trie"
	log "github.com/sirupsen/logrus"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/kv"
)

var (
	errNotFound = errors.New("not found")
)

//State manage account list
type State struct {
	trie *Trie.SecureTrie
	kv   kv.GetPutter
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
	}, nil
}

//Get return account from address
func (s *State) Get(address acc.Address) (*acc.Account, error) {
	enc, err := s.trie.TryGet(address[:])
	if err != nil {
		return nil, errNotFound
	}
	var data acc.Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

//Update update account by address
func (s *State) Update(address acc.Address, account *acc.Account) (err error) {
	enc, err := rlp.EncodeToBytes(*account)
	if err != nil {
		log.Error("UpdateAccount error:", err)
		return err
	}
	return s.trie.TryUpdate(address[:], enc)
}

// Delete removes any existing value for key from the trie.
func (s *State) Delete(address acc.Address) error {
	return s.trie.TryDelete(address[:])
}

//Commit commit data to update
func (s *State) Commit() (root cry.Hash, err error) {
	hash, err := s.trie.CommitTo(s.kv)
	if err != nil {
		return cry.Hash(common.Hash{}), err
	}
	return cry.Hash(hash), nil
}

//IsNotFound return is the err an ErrorNotFound error
func (s *State) IsNotFound(err error) bool {
	return err == errNotFound
}

//Root get storage trie root hash
func (s *State) Root() cry.Hash {
	return cry.Hash(s.trie.Hash())
}
