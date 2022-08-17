// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	dataStoreName    = "chain.data"
	propStoreName    = "chain.props"
	headStoreName    = "chain.heads"
	txIndexStoreName = "chain.txi"
)

var (
	errNotFound      = errors.New("not found")
	bestBlockIDKey   = []byte("best-block-id")
	steadyBlockIDKey = []byte("steady-block-id")
)

// Repository stores block headers, txs and receipts.
//
// It's thread-safe.
type Repository struct {
	db        *muxdb.MuxDB
	data      kv.Store
	head      kv.Store
	props     kv.Store
	txIndexer kv.Store

	genesis     *block.Block
	bestSummary atomic.Value
	steadyID    atomic.Value
	tag         byte
	tick        co.Signal

	caches struct {
		summaries *cache
		txs       *cache
		receipts  *cache
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
		data:      db.NewStore(dataStoreName),
		head:      db.NewStore(headStoreName),
		props:     db.NewStore(propStoreName),
		txIndexer: db.NewStore(txIndexStoreName),
		genesis:   genesis,
		tag:       genesisID[31],
	}

	repo.caches.summaries = newCache(512)
	repo.caches.txs = newCache(2048)
	repo.caches.receipts = newCache(2048)

	if val, err := repo.props.Get(bestBlockIDKey); err != nil {
		if !repo.props.IsNotFound(err) {
			return nil, err
		}

		if err := repo.indexBlock(0, genesis.Header().ID(), 0); err != nil {
			return nil, err
		}
		if summary, err := repo.saveBlock(genesis, nil, 0, 0); err != nil {
			return nil, err
		} else if err := repo.setBestBlockSummary(summary); err != nil {
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

		if summary, err := repo.GetBlockSummary(bestID); err != nil {
			return nil, errors.Wrap(err, "get best block")
		} else {
			repo.bestSummary.Store(summary)
		}
	}

	if val, err := repo.props.Get(steadyBlockIDKey); err != nil {
		if !repo.props.IsNotFound(err) {
			return nil, err
		}
		repo.steadyID.Store(genesis.Header().ID())
	} else {
		repo.steadyID.Store(thor.BytesToBytes32(val))
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
	if err := r.props.Put(bestBlockIDKey, summary.Header.ID().Bytes()); err != nil {
		return err
	}
	r.bestSummary.Store(summary)
	return nil
}

// SteadyBlockID return the head block id of the steady chain.
func (r *Repository) SteadyBlockID() thor.Bytes32 {
	return r.steadyID.Load().(thor.Bytes32)
}

// SetSteadyBlockID set the given block id as the head block id of the steady chain.
func (r *Repository) SetSteadyBlockID(id thor.Bytes32) error {
	prev := r.steadyID.Load().(thor.Bytes32)

	if has, err := r.NewChain(id).HasBlock(prev); err != nil {
		return err
	} else if !has {
		// the previous steady id is not on the chain of the new id.
		return errors.New("invalid new steady block id")
	}
	if err := r.props.Put(steadyBlockIDKey, id[:]); err != nil {
		return err
	}
	r.steadyID.Store(id)
	return nil
}

func (r *Repository) saveBlock(block *block.Block, receipts tx.Receipts, conflicts, steadyNum uint32) (*BlockSummary, error) {
	var (
		header      = block.Header()
		id          = header.ID()
		txs         = block.Transactions()
		summary     = BlockSummary{header, []thor.Bytes32{}, uint64(block.Size()), conflicts, steadyNum}
		bulk        = r.db.NewStore("").Bulk()
		indexPutter = kv.Bucket(txIndexStoreName).NewPutter(bulk)
		dataPutter  = kv.Bucket(dataStoreName).NewPutter(bulk)
		headPutter  = kv.Bucket(headStoreName).NewPutter(bulk)
	)

	if len(txs) > 0 {
		// index txs
		buf := make([]byte, 64)
		copy(buf[32:], id[:])
		for i, tx := range txs {
			txid := tx.ID()
			summary.Txs = append(summary.Txs, txid)

			// to accelerate point access
			if err := indexPutter.Put(txid[:], nil); err != nil {
				return nil, err
			}

			copy(buf, txid[:])
			if err := saveRLP(indexPutter, buf, &storageTxMeta{
				Index:    uint64(i),
				Reverted: receipts[i].Reverted,
			}); err != nil {
				return nil, err
			}
		}

		// save tx & receipt data
		key := makeTxKey(id, txInfix)
		for i, tx := range txs {
			key.SetIndex(uint64(i))
			if err := saveTransaction(dataPutter, key, tx); err != nil {
				return nil, err
			}
			r.caches.txs.Add(key, tx)
		}
		key = makeTxKey(id, receiptInfix)
		for i, receipt := range receipts {
			key.SetIndex(uint64(i))
			if err := saveReceipt(dataPutter, key, receipt); err != nil {
				return nil, err
			}
			r.caches.receipts.Add(key, receipt)
		}
	}
	if err := indexChainHead(headPutter, header); err != nil {
		return nil, err
	}

	if err := saveBlockSummary(dataPutter, &summary); err != nil {
		return nil, err
	}
	r.caches.summaries.Add(id, &summary)
	return &summary, bulk.Write()
}

// AddBlock add a new block with its receipts into repository.
func (r *Repository) AddBlock(newBlock *block.Block, receipts tx.Receipts, conflicts uint32) error {
	parentSummary, err := r.GetBlockSummary(newBlock.Header().ParentID())
	if err != nil {
		if r.IsNotFound(err) {
			return errors.New("parent missing")
		}
		return err
	}
	if err := r.indexBlock(parentSummary.Conflicts, newBlock.Header().ID(), conflicts); err != nil {
		return err
	}
	steadyNum := parentSummary.SteadyNum // initially inherits parent's steady num.
	newSteadyID := r.steadyID.Load().(thor.Bytes32)
	if newSteadyNum := block.Number(newSteadyID); steadyNum != newSteadyNum {
		if has, err := r.NewChain(parentSummary.Header.ID()).HasBlock(newSteadyID); err != nil {
			return err
		} else if has {
			// the chain of the new block contains the new steady id,
			steadyNum = newSteadyNum
		}
	}

	if _, err := r.saveBlock(newBlock, receipts, conflicts, steadyNum); err != nil {
		return err
	}
	return nil
}

// ScanConflicts returns the count of saved blocks with the given blockNum.
func (r *Repository) ScanConflicts(blockNum uint32) (uint32, error) {
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], blockNum)

	iter := r.data.Iterate(kv.Range(*util.BytesPrefix(prefix[:])))
	defer iter.Release()

	count := uint32(0)
	for iter.Next() {
		if len(iter.Key()) == 32 {
			count++
		}
	}
	return count, iter.Error()
}

// ScanHeads returns all head blockIDs from the given blockNum(included) in descending order.
func (r *Repository) ScanHeads(from uint32) ([]thor.Bytes32, error) {
	var start [4]byte
	binary.BigEndian.PutUint32(start[:], from)

	iter := r.head.Iterate(kv.Range{Start: start[:]})
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
	iter := r.data.Iterate(kv.Range{})
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
		return loadBlockSummary(r.data, id)
	}); err != nil {
		return
	}
	return cached.(*BlockSummary), nil
}

func (r *Repository) getTransaction(key txKey) (*tx.Transaction, error) {
	cached, err := r.caches.txs.GetOrLoad(key, func() (interface{}, error) {
		return loadTransaction(r.data, key)
	})
	if err != nil {
		return nil, err
	}
	return cached.(*tx.Transaction), nil
}

// GetBlockTransactions get all transactions of the block for given block id.
func (r *Repository) GetBlockTransactions(id thor.Bytes32) (tx.Transactions, error) {
	summary, err := r.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}

	if n := len(summary.Txs); n > 0 {
		txs := make(tx.Transactions, n)
		key := makeTxKey(id, txInfix)
		for i := range summary.Txs {
			key.SetIndex(uint64(i))
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

func (r *Repository) getReceipt(key txKey) (*tx.Receipt, error) {
	cached, err := r.caches.receipts.GetOrLoad(key, func() (interface{}, error) {
		return loadReceipt(r.data, key)
	})
	if err != nil {
		return nil, err
	}
	return cached.(*tx.Receipt), nil
}

// GetBlockReceipts get all tx receipts of the block for given block id.
func (r *Repository) GetBlockReceipts(id thor.Bytes32) (tx.Receipts, error) {
	summary, err := r.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}

	if n := len(summary.Txs); n > 0 {
		receipts := make(tx.Receipts, n)
		key := makeTxKey(id, receiptInfix)
		for i := range summary.Txs {
			key.SetIndex(uint64(i))
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
