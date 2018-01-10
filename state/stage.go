package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
)

// Stage abstracts changes on the main accounts trie.
type Stage struct {
	err error

	kv kv.GetPutter

	accountTrie  *trie.SecureTrie
	storageTries []*trie.SecureTrie
	codes        []codeWithHash
}

type codeWithHash struct {
	code []byte
	hash []byte
}

func newStage(root thor.Hash, kv kv.GetPutter, changes map[thor.Address]*changedObject) *Stage {
	accountTrie, err := trie.NewSecure(common.Hash(root), kv, 0)
	if err != nil {
		return &Stage{err: err}
	}

	storageTries := make([]*trie.SecureTrie, 0, len(changes))
	codes := make([]codeWithHash, 0, len(changes))

	for addr, obj := range changes {
		dataCpy := obj.data

		if len(dataCpy.CodeHash) > 0 {
			codes = append(codes, codeWithHash{
				code: obj.code,
				hash: dataCpy.CodeHash})
		}

		// skip storage changes if account is empty
		if !dataCpy.IsEmpty() {
			if len(obj.storage) > 0 {
				strie, err := trie.NewSecure(common.BytesToHash(dataCpy.StorageRoot), kv, 0)
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
		kv:           kv,
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

	batch := s.kv.NewBatch()
	// write codes
	for _, code := range s.codes {
		if err := batch.Put(code.hash, code.code); err != nil {
			return thor.Hash{}, err
		}
	}

	// commit storage tries
	for _, strie := range s.storageTries {
		if _, err := strie.CommitTo(batch); err != nil {
			return thor.Hash{}, err
		}
	}

	// commit accounts trie
	root, err := s.accountTrie.CommitTo(batch)
	if err != nil {
		return thor.Hash{}, err
	}

	if err := batch.Write(); err != nil {
		return thor.Hash{}, err
	}

	return thor.Hash(root), nil
}
