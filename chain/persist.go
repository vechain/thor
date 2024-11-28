// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

// appendTxKey composes the key to access tx or receipt.
func appendTxKey(buf []byte, blockNum, blockConflicts uint32, index uint64, flag byte) []byte {
	buf = binary.BigEndian.AppendUint32(buf, blockNum)
	buf = binary.AppendUvarint(buf, uint64(blockConflicts))
	buf = append(buf, flag)
	return binary.AppendUvarint(buf, index)
}

// BlockSummary presents block summary.
type BlockSummary struct {
	Header    *block.Header
	Txs       []thor.Bytes32
	Size      uint64
	Conflicts uint32
}

// Root returns state root for accessing state trie.
func (s *BlockSummary) Root() trie.Root {
	return trie.Root{
		Hash: s.Header.StateRoot(),
		Ver: trie.Version{
			Major: s.Header.Number(),
			Minor: s.Conflicts,
		},
	}
}

// IndexRoot returns index root for accessing index trie.
func (s *BlockSummary) IndexRoot() trie.Root {
	return trie.Root{
		// index trie skips hash, so here just provide a non-zero hash
		Hash: thor.BytesToBytes32([]byte{1}),
		Ver: trie.Version{
			Major: s.Header.Number(),
			Minor: s.Conflicts,
		},
	}
}

func saveRLP(w kv.Putter, key []byte, val interface{}) error {
	data, err := rlp.EncodeToBytes(val)
	if err != nil {
		return err
	}
	return w.Put(key, data)
}

func loadRLP(r kv.Getter, key []byte, val interface{}) error {
	data, err := r.Get(key)
	if err != nil {
		return err
	}
	return rlp.DecodeBytes(data, val)
}

func saveBlockSummary(w kv.Putter, summary *BlockSummary) error {
	return saveRLP(w, summary.Header.ID().Bytes(), summary)
}

// indexChainHead puts a header into store, it will put the block id and delete the parent id.
// So there is only one block id stored for every branch(fork). Thus will result we can scan all
// possible fork's head by iterating the index store.
func indexChainHead(w kv.Putter, header *block.Header) error {
	if err := w.Delete(header.ParentID().Bytes()); err != nil {
		return err
	}

	return w.Put(header.ID().Bytes(), nil)
}

func loadBlockSummary(r kv.Getter, id thor.Bytes32) (*BlockSummary, error) {
	var summary BlockSummary
	if err := loadRLP(r, id[:], &summary); err != nil {
		return nil, err
	}
	metricBlockRepositoryCounter().AddWithLabel(1, map[string]string{"type": "read", "target": "db"})
	return &summary, nil
}
