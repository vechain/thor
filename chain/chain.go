// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"encoding/binary"
	"sort"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
	"github.com/vechain/thor/tx"
)

const (
	// IndexTrieName is the name of index trie.
	// The index tire is used to store mappings from block number to block id, and tx id to tx meta.
	IndexTrieName = "i"
)

type storageTxMeta struct {
	Index    uint64
	Reverted bool
}

// TxMeta contains tx location and reversal state.
type TxMeta struct {
	// The block id this tx is involved.
	BlockID thor.Bytes32

	// Index the position of the tx in block's txs.
	Index uint64 // rlp require uint64.

	Reverted bool
}

// Chain presents the linked block chain, with the range from genesis to given head block.
//
// It provides reliable methods to access block by number, tx by id, etc...
type Chain struct {
	repo     *Repository
	headID   thor.Bytes32
	lazyInit func() (*muxdb.Trie, error)
}

func newChain(repo *Repository, headID thor.Bytes32) *Chain {
	var (
		indexTrie *muxdb.Trie
		initErr   error
	)

	return &Chain{
		repo,
		headID,
		func() (*muxdb.Trie, error) {
			if indexTrie == nil && initErr == nil {
				if summary, err := repo.GetBlockSummary(headID); err == nil {
					indexTrie = repo.db.NewNonCryptoTrie(IndexTrieName, trie.NonCryptoNodeHash, summary.Header.Number(), summary.Conflicts)
				} else {
					initErr = errors.Wrap(err, "lazy init chain")
				}
			}
			return indexTrie, initErr
		},
	}
}

// GenesisID returns genesis id.
func (c *Chain) GenesisID() thor.Bytes32 {
	return c.repo.GenesisBlock().Header().ID()
}

// HeadID returns the head block id.
func (c *Chain) HeadID() thor.Bytes32 {
	return c.headID
}

// GetBlockID returns block id by given block number.
func (c *Chain) GetBlockID(num uint32) (thor.Bytes32, error) {
	trie, err := c.lazyInit()
	if err != nil {
		return thor.Bytes32{}, err
	}

	var key [4]byte
	binary.BigEndian.PutUint32(key[:], num)

	data, _, err := trie.Get(key[:])
	if err != nil {
		return thor.Bytes32{}, err
	}
	if len(data) == 0 {
		return thor.Bytes32{}, errNotFound
	}
	return thor.BytesToBytes32(data), nil
}

// GetTransactionMeta returns tx meta by given tx id.
func (c *Chain) GetTransactionMeta(id thor.Bytes32) (*TxMeta, error) {
	// precheck. point access is faster than range access.
	if has, err := c.repo.txIndexer.Has(id[:]); err != nil {
		return nil, err
	} else if !has {
		return nil, errNotFound
	}

	iter := c.repo.txIndexer.Iterate(kv.Range(*util.BytesPrefix(id[:])))
	defer iter.Release()
	for iter.Next() {
		if len(iter.Key()) != 64 { // skip the pure txid key
			continue
		}

		blockID := thor.BytesToBytes32(iter.Key()[32:])

		has, err := c.HasBlock(blockID)
		if err != nil {
			return nil, err
		}
		if has {
			var sMeta storageTxMeta
			if err := rlp.DecodeBytes(iter.Value(), &sMeta); err != nil {
				return nil, err
			}
			return &TxMeta{
				BlockID:  blockID,
				Index:    sMeta.Index,
				Reverted: sMeta.Reverted,
			}, nil
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return nil, errNotFound
}

// GetBlockHeader returns block header by given block number.
func (c *Chain) GetBlockHeader(num uint32) (*block.Header, error) {
	summary, err := c.GetBlockSummary(num)
	if err != nil {
		return nil, err
	}
	return summary.Header, nil
}

// GetBlockSummary returns block summary by given block number.
func (c Chain) GetBlockSummary(num uint32) (*BlockSummary, error) {
	id, err := c.GetBlockID(num)
	if err != nil {
		return nil, err
	}
	return c.repo.GetBlockSummary(id)
}

// GetBlock returns block by given block number.
func (c *Chain) GetBlock(num uint32) (*block.Block, error) {
	id, err := c.GetBlockID(num)
	if err != nil {
		return nil, err
	}
	return c.repo.GetBlock(id)
}

// GetTransaction returns tx along with meta by given tx id.
func (c *Chain) GetTransaction(id thor.Bytes32) (*tx.Transaction, *TxMeta, error) {
	txMeta, err := c.GetTransactionMeta(id)
	if err != nil {
		return nil, nil, err
	}

	key := makeTxKey(txMeta.BlockID, txInfix)
	key.SetIndex(txMeta.Index)
	tx, err := c.repo.getTransaction(key)
	if err != nil {
		return nil, nil, err
	}
	return tx, txMeta, nil
}

// GetTransactionReceipt returns tx receipt by given tx id.
func (c *Chain) GetTransactionReceipt(txID thor.Bytes32) (*tx.Receipt, error) {
	txMeta, err := c.GetTransactionMeta(txID)
	if err != nil {
		return nil, err
	}

	key := makeTxKey(txMeta.BlockID, receiptInfix)
	key.SetIndex(txMeta.Index)
	receipt, err := c.repo.getReceipt(key)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

// HasBlock check if the block with given id belongs to the chain.
func (c *Chain) HasBlock(id thor.Bytes32) (bool, error) {
	foundID, err := c.GetBlockID(block.Number(id))
	if err != nil {
		if c.repo.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return id == foundID, nil
}

// Exclude returns ids of blocks belongs to this chain, but not belongs to other.
//
// The returned ids are in ascending order.
func (c *Chain) Exclude(other *Chain) ([]thor.Bytes32, error) {
	oHeadID := other.headID
	oHeadNum := block.Number(oHeadID)
	var ids []thor.Bytes32

	id := c.headID
	for {
		n := block.Number(id)
		if n == 0 {
			break
		}

		if n > oHeadNum {
			ids = append(ids, id)
		} else if n == oHeadNum {
			if id == oHeadID {
				break
			}
			ids = append(ids, id)
		} else {
			has, err := other.HasBlock(id)
			if err != nil {
				return nil, err
			}
			if has {
				break
			}
			ids = append(ids, id)
		}
		var err error
		id, err = c.GetBlockID(n - 1)
		if err != nil {
			return nil, err
		}
	}

	// reverse
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids, nil
}

// IsNotFound returns if the given error means not found.
func (c *Chain) IsNotFound(err error) bool {
	return c.repo.IsNotFound(err)
}

// FindBlockHeaderByTimestamp find the block whose timestamp matches the given timestamp.
//
// When flag == 0, exact match is performed (may return error not found)
// flag > 0, matches the lowest block whose timestamp >= ts
// flag < 0, matches the highest block whose timestamp <= ts.
func (c *Chain) FindBlockHeaderByTimestamp(ts uint64, flag int) (header *block.Header, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()
	headNum := block.Number(c.headID)
	if flag >= 0 {
		n := uint32(sort.Search(int(headNum), func(i int) bool {
			h, err := c.GetBlockHeader(uint32(i))
			if err != nil {
				panic(err)
			}
			return h.Timestamp() >= ts
		}))
		if header, err = c.GetBlockHeader(n); err != nil {
			return
		}
		if flag == 0 && header.Timestamp() != ts { // exact match
			return nil, errNotFound
		}
		return
	}

	// flag < 0
	n := headNum - uint32(sort.Search(int(headNum), func(i int) bool {
		h, err := c.GetBlockHeader(headNum - uint32(i))
		if err != nil {
			panic(err)
		}
		return h.Timestamp() <= ts
	}))
	return c.GetBlockHeader(n)
}

// NewBestChain create a chain with best block as head.
func (r *Repository) NewBestChain() *Chain {
	return newChain(r, r.BestBlockSummary().Header.ID())
}

// NewChain create a chain with head block specified by headID.
func (r *Repository) NewChain(headID thor.Bytes32) *Chain {
	return newChain(r, headID)
}

func (r *Repository) indexBlock(parentConflicts uint32, newBlockID thor.Bytes32, newConflicts uint32) error {
	var (
		newNum = block.Number(newBlockID)
		root   thor.Bytes32
	)

	if newNum != 0 { // not a genesis block
		root = trie.NonCryptoNodeHash
	}

	trie := r.db.NewNonCryptoTrie(IndexTrieName, root, newNum-1, parentConflicts)
	// map block number to block ID
	if err := trie.Update(newBlockID[:4], newBlockID[:], nil); err != nil {
		return err
	}

	_, commit := trie.Stage(newNum, newConflicts)
	return commit()
}
