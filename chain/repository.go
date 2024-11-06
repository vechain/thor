// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

const (
	hdrStoreName     = "chain.hdr"  // for block headers
	bodyStoreName    = "chain.body" // for block bodies and receipts
	propStoreName    = "chain.props"
	headStoreName    = "chain.heads"
	txIndexStoreName = "chain.txi"

	txFlag         = byte(0) // flag byte of the key for saving tx blob
	receiptFlag    = byte(1) // flag byte fo the key for saving receipt blob
	txFilterKeyLen = 8
)

var (
	errNotFound    = errors.New("not found")
	bestBlockIDKey = []byte("best-block-id")
)

// Repository stores block headers, txs and receipts.
//
// It's thread-safe.
type Repository struct {
	db        *muxdb.MuxDB
	hdrStore  kv.Store
	bodyStore kv.Store
	propStore kv.Store
	headStore kv.Store
	txIndexer kv.Store

	genesis *block.Block
	tag     byte

	bestSummary atomic.Value
	tick        co.Signal

	caches struct {
		summaries *cache
	}
}

// NewRepository create an instance of repository.
func NewRepository(db *muxdb.MuxDB, genesis *block.Block) (*Repository, error) {
	if genesis.Header().Number() != 0 {
		return nil, errors.New("genesis number != 0")
	}
	if len(genesis.Transactions()) != 0 {
		return nil, errors.New("genesis block should not have transactions")
	}

	genesisID := genesis.Header().ID()
	repo := &Repository{
		db:        db,
		hdrStore:  db.NewStore(hdrStoreName),
		bodyStore: db.NewStore(bodyStoreName),
		propStore: db.NewStore(propStoreName),
		headStore: db.NewStore(headStoreName),
		txIndexer: db.NewStore(txIndexStoreName),
		genesis:   genesis,
		tag:       genesisID[31],
	}

	repo.caches.summaries = newCache(512)
	if val, err := repo.propStore.Get(bestBlockIDKey); err != nil {
		if !repo.propStore.IsNotFound(err) {
			return nil, err
		}

		if err := repo.indexBlock(trie.Root{}, genesis.Header().ID(), 0); err != nil {
			return nil, err
		}
		if _, err := repo.saveBlock(genesis, nil, 0, true); err != nil {
			return nil, err
		}
	} else {
		bestID := thor.BytesToBytes32(val)
		existingGenesisID, err := repo.NewChain(bestID).GetBlockID(0)
		if err != nil {
			return nil, errors.Wrap(err, "get existing genesis id")
		}
		if existingGenesisID != genesisID {
			return nil, errors.New("genesis mismatch")
		}

		summary, err := repo.GetBlockSummary(bestID)
		if err != nil {
			return nil, errors.Wrap(err, "get best block")
		}
		repo.bestSummary.Store(summary)
	}

	return repo, nil
}

// ChainTag returns chain tag, which is the last byte of genesis id.
func (r *Repository) ChainTag() byte {
	return r.tag
}

// GenesisBlock returns genesis block.
func (r *Repository) GenesisBlock() *block.Block {
	return r.genesis
}

// BestBlockSummary returns the summary of the best block, which is the newest block of canonical chain.
func (r *Repository) BestBlockSummary() *BlockSummary {
	return r.bestSummary.Load().(*BlockSummary)
}

// SetBestBlockID set the given block id as best block id.
func (r *Repository) SetBestBlockID(id thor.Bytes32) (err error) {
	defer func() {
		if err == nil {
			r.tick.Broadcast()
		}
	}()
	summary, err := r.GetBlockSummary(id)
	if err != nil {
		return err
	}
	return r.setBestBlockSummary(summary)
}

func (r *Repository) setBestBlockSummary(summary *BlockSummary) error {
	if err := r.propStore.Put(bestBlockIDKey, summary.Header.ID().Bytes()); err != nil {
		return err
	}
	r.bestSummary.Store(summary)
	return nil
}

func (r *Repository) saveBlock(block *block.Block, receipts tx.Receipts, conflicts uint32, asBest bool) (*BlockSummary, error) {
	var (
		header        = block.Header()
		id            = header.ID()
		num           = header.Number()
		txs           = block.Transactions()
		txIDs         = []thor.Bytes32{}
		bulk          = r.db.NewStore("").Bulk()
		hdrPutter     = kv.Bucket(hdrStoreName).NewPutter(bulk)
		bodyPutter    = kv.Bucket(bodyStoreName).NewPutter(bulk)
		propPutter    = kv.Bucket(propStoreName).NewPutter(bulk)
		headPutter    = kv.Bucket(headStoreName).NewPutter(bulk)
		txIndexPutter = kv.Bucket(txIndexStoreName).NewPutter(bulk)
		keyBuf        []byte
	)

	if len(txs) > 0 {
		// index and save txs
		for i, tx := range txs {
			txid := tx.ID()
			txIDs = append(txIDs, txid)

			// write the filter key
			if err := txIndexPutter.Put(txid[:txFilterKeyLen], nil); err != nil {
				return nil, err
			}
			// write tx metadata
			keyBuf = append(keyBuf[:0], txid[:]...)
			keyBuf = binary.AppendUvarint(keyBuf, uint64(header.Number()))
			keyBuf = binary.AppendUvarint(keyBuf, uint64(conflicts))

			if err := saveRLP(txIndexPutter, keyBuf, &storageTxMeta{
				Index:    uint64(i),
				Reverted: receipts[i].Reverted,
			}); err != nil {
				return nil, err
			}

			// write the tx blob
			keyBuf = appendTxKey(keyBuf[:0], num, conflicts, uint64(i), txFlag)
			if err := saveRLP(bodyPutter, keyBuf[:], tx); err != nil {
				return nil, err
			}
		}

		// save receipts
		for i, receipt := range receipts {
			keyBuf = appendTxKey(keyBuf[:0], num, conflicts, uint64(i), receiptFlag)
			if err := saveRLP(bodyPutter, keyBuf, receipt); err != nil {
				return nil, err
			}
		}
	}
	if err := indexChainHead(headPutter, header); err != nil {
		return nil, err
	}

	summary := BlockSummary{header, txIDs, uint64(block.Size()), conflicts}
	if err := saveBlockSummary(hdrPutter, &summary); err != nil {
		return nil, err
	}

	if asBest {
		if err := propPutter.Put(bestBlockIDKey, id[:]); err != nil {
			return nil, err
		}
	}

	if err := bulk.Write(); err != nil {
		return nil, err
	}
	r.caches.summaries.Add(id, &summary)
	if asBest {
		r.bestSummary.Store(&summary)
		r.tick.Broadcast()
	}
	return &summary, nil
}

// AddBlock add a new block with its receipts into repository.
func (r *Repository) AddBlock(newBlock *block.Block, receipts tx.Receipts, conflicts uint32, asBest bool) error {
	parentSummary, err := r.GetBlockSummary(newBlock.Header().ParentID())
	if err != nil {
		if r.IsNotFound(err) {
			return errors.New("parent missing")
		}
		return err
	}
	if err := r.indexBlock(parentSummary.IndexRoot(), newBlock.Header().ID(), conflicts); err != nil {
		return err
	}

	if _, err := r.saveBlock(newBlock, receipts, conflicts, asBest); err != nil {
		return err
	}
	return nil
}

// ScanConflicts returns the count of saved blocks with the given blockNum.
func (r *Repository) ScanConflicts(blockNum uint32) (uint32, error) {
	prefix := binary.BigEndian.AppendUint32(nil, blockNum)

	iter := r.hdrStore.Iterate(kv.Range(*util.BytesPrefix(prefix)))
	defer iter.Release()

	count := uint32(0)
	for iter.Next() {
		count++
	}
	return count, iter.Error()
}

// ScanHeads returns all head blockIDs from the given blockNum(included) in descending order.
func (r *Repository) ScanHeads(from uint32) ([]thor.Bytes32, error) {
	start := binary.BigEndian.AppendUint32(nil, from)

	iter := r.headStore.Iterate(kv.Range{Start: start})
	defer iter.Release()

	heads := make([]thor.Bytes32, 0, 16)

	for ok := iter.Last(); ok; ok = iter.Prev() {
		heads = append(heads, thor.BytesToBytes32(iter.Key()))
	}

	if iter.Error() != nil {
		return nil, iter.Error()
	}

	return heads, nil
}

// GetMaxBlockNum returns the max committed block number.
func (r *Repository) GetMaxBlockNum() (uint32, error) {
	iter := r.hdrStore.Iterate(kv.Range{})
	defer iter.Release()

	if iter.Last() {
		return binary.BigEndian.Uint32(iter.Key()), iter.Error()
	}
	return 0, iter.Error()
}

// GetBlockSummary get block summary by block id.
func (r *Repository) GetBlockSummary(id thor.Bytes32) (summary *BlockSummary, err error) {
	var cached interface{}
	if cached, err = r.caches.summaries.GetOrLoad(id, func() (interface{}, error) {
		return loadBlockSummary(r.hdrStore, id)
	}); err != nil {
		return
	}
	return cached.(*BlockSummary), nil
}

func (r *Repository) getTransaction(key []byte) (*tx.Transaction, error) {
	var tx tx.Transaction
	if err := loadRLP(r.bodyStore, key, &tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

// GetBlockTransactions get all transactions of the block for given block id.
func (r *Repository) GetBlockTransactions(id thor.Bytes32) (tx.Transactions, error) {
	summary, err := r.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}

	if n := len(summary.Txs); n > 0 {
		txs := make(tx.Transactions, n)
		var key []byte
		for i := range summary.Txs {
			key := appendTxKey(key[:0], summary.Header.Number(), summary.Conflicts, uint64(i), txFlag)
			txs[i], err = r.getTransaction(key)
			if err != nil {
				return nil, err
			}
		}
		return txs, nil
	}
	return nil, nil
}

// GetBlock get block by id.
func (r *Repository) GetBlock(id thor.Bytes32) (*block.Block, error) {
	summary, err := r.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}
	txs, err := r.GetBlockTransactions(id)
	if err != nil {
		return nil, err
	}
	return block.Compose(summary.Header, txs), nil
}

func (r *Repository) getReceipt(key []byte) (*tx.Receipt, error) {
	var receipt tx.Receipt
	if err := loadRLP(r.bodyStore, key, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

// GetBlockReceipts get all tx receipts of the block for given block id.
func (r *Repository) GetBlockReceipts(id thor.Bytes32) (tx.Receipts, error) {
	summary, err := r.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}

	if n := len(summary.Txs); n > 0 {
		receipts := make(tx.Receipts, n)
		var key []byte
		for i := range summary.Txs {
			key := appendTxKey(key[:0], summary.Header.Number(), summary.Conflicts, uint64(i), receiptFlag)
			receipts[i], err = r.getReceipt(key)
			if err != nil {
				return nil, err
			}
		}
		return receipts, nil
	}
	return nil, nil
}

// IsNotFound returns if the given error means not found.
func (r *Repository) IsNotFound(err error) bool {
	return err == errNotFound || r.db.IsNotFound(err)
}

// NewTicker create a signal Waiter to receive event that the best block changed.
func (r *Repository) NewTicker() co.Waiter {
	return r.tick.NewWaiter()
}
