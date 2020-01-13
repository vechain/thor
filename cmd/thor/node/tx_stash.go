// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"container/list"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// to stash non-executable txs.
// it uses a FIFO queue to limit the size of stash.
type txStash struct {
	db      *leveldb.DB
	fifo    *list.List
	maxSize int
}

func newTxStash(db *leveldb.DB, maxSize int) *txStash {
	return &txStash{db, list.New(), maxSize}
}

func (ts *txStash) Save(tx *tx.Transaction) error {
	has, err := ts.db.Has(tx.Hash().Bytes(), nil)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	data, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return err
	}

	if err := ts.db.Put(tx.Hash().Bytes(), data, nil); err != nil {
		return err
	}
	ts.fifo.PushBack(tx.Hash())
	for ts.fifo.Len() > ts.maxSize {
		keyToDelete := ts.fifo.Remove(ts.fifo.Front()).(thor.Bytes32).Bytes()
		if err := ts.db.Delete(keyToDelete, nil); err != nil {
			return err
		}
	}
	return nil
}

func (ts *txStash) LoadAll() tx.Transactions {
	var (
		txs   tx.Transactions
		batch leveldb.Batch
	)

	it := ts.db.NewIterator(util.BytesPrefix(nil), nil)
	defer it.Release()

	for it.Next() {
		var tx tx.Transaction
		if err := rlp.DecodeBytes(it.Value(), &tx); err != nil {
			log.Warn("decode stashed tx", "err", err)
			batch.Delete(it.Key())
		} else {
			txs = append(txs, &tx)
			ts.fifo.PushBack(tx.Hash())

			// Keys were tx ids.
			// Here to remap values using tx hashes.
			if !bytes.Equal(it.Key(), tx.Hash().Bytes()) {
				batch.Delete(it.Key())
				batch.Put(tx.Hash().Bytes(), it.Value())
			}
		}
	}

	if err := ts.db.Write(&batch, nil); err != nil {
		log.Warn("remap stashed txs", "err", err)
	}
	return txs
}
