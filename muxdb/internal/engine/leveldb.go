// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package engine

import (
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
)

var (
	writeOpt = opt.WriteOptions{}
	readOpt  = opt.ReadOptions{}
	scanOpt  = opt.ReadOptions{DontFillCache: true}
)

type levelEngine struct {
	db        *leveldb.DB
	batchPool *sync.Pool
}

// NewLevelEngine creates leveldb instance which implements the Engine interface.
func NewLevelEngine(db *leveldb.DB) Engine {
	return &levelEngine{
		db,
		&sync.Pool{
			New: func() interface{} {
				return &leveldb.Batch{}
			},
		},
	}
}

func (ldb *levelEngine) Close() error {
	return ldb.db.Close()
}

func (ldb *levelEngine) IsNotFound(err error) bool {
	return err == leveldb.ErrNotFound
}

func (ldb *levelEngine) Get(key []byte) ([]byte, error) {
	val, err := ldb.db.Get(key, &readOpt)
	// val will be []byte{} if error occurs, which is not expected
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (ldb *levelEngine) Has(key []byte) (bool, error) {
	return ldb.db.Has(key, &readOpt)
}

func (ldb *levelEngine) Put(key, val []byte) error {
	return ldb.db.Put(key, val, &writeOpt)
}

func (ldb *levelEngine) Delete(key []byte) error {
	return ldb.db.Delete(key, &writeOpt)
}

func (ldb *levelEngine) Snapshot() kv.Snapshot {
	s, err := ldb.db.GetSnapshot()
	return &struct {
		kv.GetFunc
		kv.HasFunc
		kv.IsNotFoundFunc
		kv.ReleaseFunc
	}{
		func(key []byte) ([]byte, error) {
			if err != nil {
				return nil, err
			}
			val, err := s.Get(key, &readOpt)
			if err != nil {
				return nil, err
			}
			return val, nil
		},
		func(key []byte) (bool, error) {
			if err != nil {
				return false, err
			}
			return s.Has(key, &readOpt)
		},
		ldb.IsNotFound,
		func() {
			if s != nil {
				s.Release()
			}
		},
	}
}

func (ldb *levelEngine) Bulk() kv.Bulk {
	const idealBatchSize = 128 * 1024
	var batch *leveldb.Batch

	getBatch := func() *leveldb.Batch {
		if batch == nil {
			batch = ldb.batchPool.Get().(*leveldb.Batch)
			batch.Reset()
		}
		return batch
	}
	flush := func(minSize int) error {
		if batch != nil && len(batch.Dump()) >= minSize {
			if batch.Len() > 0 {
				if err := ldb.db.Write(batch, &writeOpt); err != nil {
					return err
				}
			}
			ldb.batchPool.Put(batch)
			batch = nil
		}
		return nil
	}
	var autoFlush bool

	return &struct {
		kv.PutFunc
		kv.DeleteFunc
		kv.EnableAutoFlushFunc
		kv.WriteFunc
	}{
		func(key, val []byte) error {
			getBatch().Put(key, val)
			if autoFlush {
				return flush(idealBatchSize)
			}
			return nil
		},
		func(key []byte) error {
			getBatch().Delete(key)
			if autoFlush {
				return flush(idealBatchSize)
			}
			return nil
		},
		func() { autoFlush = true },
		func() error { return flush(0) },
	}
}

func (ldb *levelEngine) Iterate(r kv.Range) kv.Iterator {
	return ldb.db.NewIterator((*util.Range)(&r), &scanOpt)
}
