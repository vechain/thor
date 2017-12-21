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

//GetStorage return account storage from storage root
func (s *Storage) GetStorage(root cry.Hash, key cry.Hash) (value cry.Hash) {
	trie, err := s.getOrCreateNewTrie(root)
	if err != nil {
		return cry.Hash{}
	}
	enc, err := trie.TryGet(key[:])
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

//UpdateStorage update account storage
func (s *Storage) UpdateStorage(root cry.Hash, key cry.Hash, value cry.Hash) error {
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
func (s *Storage) Commit(root cry.Hash) (r cry.Hash, err error) {
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

//Hash get current root hash from trie which create by root
func (s *Storage) Hash(root cry.Hash) cry.Hash {
	trie, err := s.getOrCreateNewTrie(root)
	if err != nil {
		return cry.Hash{}
	}
	return cry.Hash(trie.Hash())
}
