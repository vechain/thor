// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
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
	defer func() {
		if err != nil {
			err = &Error{err}
		}
	}()
	// write codes
	codeBulk := s.db.NewStore(codeStoreName).Bulk()
	for hash, code := range s.codes {
		if err = codeBulk.Put(hash[:], code); err != nil {
			return
		}
	}
	if err = codeBulk.Flush(); err != nil {
		return
	}

	// commit storage tries
	for _, t := range s.storageTries {
		if _, err = t.Commit(s.newCommitNum); err != nil {
			return
		}
	}

	// commit accounts trie
	return s.trie.Commit(s.newCommitNum)
}
