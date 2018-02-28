package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/thor"
)

// Stage abstracts changes on the main accounts trie.
type Stage struct {
	err error

	db           *trie.Database
	accountTrie  *trie.SecureTrie
	storageTries []*trie.SecureTrie
	codes        []codeWithHash
}

type codeWithHash struct {
	code []byte
	hash []byte
}

func newStage(root thor.Hash, db *trie.Database, changes map[thor.Address]*changedObject) *Stage {

	accountTrie, err := trie.NewSecure(common.Hash(root), db, 0)
	if err != nil {
		return &Stage{err: err}
	}

	storageTries := make([]*trie.SecureTrie, 0, len(changes))
	codes := make([]codeWithHash, 0, len(changes))

	for addr, obj := range changes {
		dataCpy := obj.data

		if len(obj.code) > 0 {
			codes = append(codes, codeWithHash{
				code: obj.code,
				hash: dataCpy.CodeHash})
		}

		// skip storage changes if account is empty
		if !dataCpy.IsEmpty() {
			if len(obj.storage) > 0 {
				strie, err := trie.NewSecure(common.BytesToHash(dataCpy.StorageRoot), db, 0)
				if err != nil {
					return &Stage{err: err}
				}
				storageTries = append(storageTries, strie)
				for k, v := range obj.storage {
					if err := saveStorage(strie, k, v); err != nil {
						return &Stage{err: err}
					}
				}
				dataCpy.StorageRoot = strie.Hash().Bytes()
			}
		}

		if err := saveAccount(accountTrie, addr, dataCpy); err != nil {
			return &Stage{err: err}
		}
	}
	return &Stage{
		db:           db,
		accountTrie:  accountTrie,
		storageTries: storageTries,
		codes:        codes,
	}
}

// Hash computes hash of the main accounts trie.
func (s *Stage) Hash() (thor.Hash, error) {
	if s.err != nil {
		return thor.Hash{}, s.err
	}
	return thor.Hash(s.accountTrie.Hash()), nil
}

// Commit commits all changes into main accounts trie and storage tries.
func (s *Stage) Commit() (thor.Hash, error) {
	if s.err != nil {
		return thor.Hash{}, s.err
	}

	// write codes
	for _, code := range s.codes {
		s.db.Insert(common.BytesToHash(code.hash), code.code)
	}

	// commit storage tries
	for _, strie := range s.storageTries {
		if _, err := strie.Commit(nil); err != nil {
			return thor.Hash{}, err
		}
	}

	// commit accounts trie
	root, err := s.accountTrie.Commit(func(leaf []byte, parent common.Hash) error {
		var account Account
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		if len(account.StorageRoot) > 0 {
			s.db.Reference(common.BytesToHash(account.StorageRoot), parent)
		}

		if len(account.CodeHash) > 0 {
			s.db.Reference(common.BytesToHash(account.CodeHash), parent)
		}
		return nil
	})
	if err := s.db.Commit(root, false); err != nil {
		return thor.Hash{}, err
	}
	if err != nil {
		return thor.Hash{}, err
	}
	return thor.Hash(root), nil
}
