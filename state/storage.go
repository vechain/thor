package state

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	Trie "github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/kv"
)

//Storage account storage
type Storage struct {
	kv           kv.GetPutter
	storageTries map[cry.Hash]*Trie.SecureTrie
}

//NewStorage return clear storage
func NewStorage(kv kv.GetPutter) *Storage {
	return &Storage{
		kv:           kv,
		storageTries: make(map[cry.Hash]*Trie.SecureTrie),
	}
}

//if trie exists returned else return a new trie from root
func (s *Storage) getOrCreateNewTrie(root cry.Hash) (trie *Trie.SecureTrie, err error) {
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

//Get return account storage from storage root
func (s *Storage) Get(root cry.Hash, key cry.Hash) (value cry.Hash, err error) {
	trie, err := s.getOrCreateNewTrie(root)
	if err != nil {
		return cry.Hash{}, err
	}
	enc, err := trie.TryGet(key[:])
	if err != nil {
		return cry.Hash{}, errNotFound
	}
	_, content, _, err := rlp.Split(enc)
	if err != nil {
		return cry.Hash{}, err
	}
	value = cry.BytesToHash(content)
	return value, nil
}

//Update update account storage
func (s *Storage) Update(root cry.Hash, key cry.Hash, value cry.Hash) error {
	trie, err := s.getOrCreateNewTrie(root)
	if err != nil {
		return err
	}
	v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
	e := trie.TryUpdate(key[:], v)
	if e != nil {
		return e
	}
	return nil
}

//Commit commit data to db
func (s *Storage) Commit(root cry.Hash) (cry.Hash, error) {
	trie, err := s.getOrCreateNewTrie(root)
	if err != nil {
		return cry.Hash{}, err
	}
	hash, err := trie.CommitTo(s.kv)
	if err != nil {
		return cry.Hash(common.Hash{}), err
	}
	return cry.Hash(hash), nil
}

//IsNotFound return is the err an ErrorNotFound error
func (s *Storage) IsNotFound(err error) bool {
	return err == errNotFound
}

//CommitAll commit all trie in the storage , remove the trie from cache if success
func (s *Storage) CommitAll() error {
	for root, trie := range s.storageTries {
		_, err := trie.CommitTo(s.kv)
		if err != nil {
			return err
		}
		delete(s.storageTries, root)
	}
	return nil
}

//Root get current root hash from trie which create by root
func (s *Storage) Root(root cry.Hash) (cry.Hash, error) {
	trie, err := s.getOrCreateNewTrie(root)
	if err != nil {
		return cry.Hash{}, err
	}
	return cry.Hash(trie.Hash()), nil
}
