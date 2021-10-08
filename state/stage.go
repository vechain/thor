// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

// Stage abstracts changes on the main accounts trie.
type Stage struct {
	db           *muxdb.MuxDB
	trie         *muxdb.Trie
	storageTries []*muxdb.Trie
	codes        map[thor.Bytes32][]byte
	newCommitNum uint32
}

// Hash computes hash of the main accounts trie.
func (s *Stage) Hash() thor.Bytes32 {
	return s.trie.Hash()
}

// Commit commits all changes into main accounts trie and storage tries.
func (s *Stage) Commit() (root thor.Bytes32, err error) {
	codeStore := s.db.NewStore(codeStoreName)

	// write codes
	if err = codeStore.Batch(func(w kv.Putter) error {
		for hash, code := range s.codes {
			if err := w.Put(hash[:], code); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		err = &Error{err}
		return
	}

	// commit storage tries
	for _, t := range s.storageTries {
		if _, err = t.Commit(s.newCommitNum); err != nil {
			err = &Error{err}
			return
		}
	}

	// commit accounts trie
	if root, err = s.trie.Commit(s.newCommitNum); err != nil {
		err = &Error{err}
		return
	}
	return
}
