// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

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

// newLevelEngine create leveldb instance which implements engine interface.
func newLevelEngine(db *leveldb.DB) engine {
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

func (ldb *levelEngine) Snapshot(fn func(kv.Getter) error) error {
	s, err := ldb.db.GetSnapshot()
	if err != nil {
		return err
	}
	defer s.Release()

	return fn(&struct {
		kv.GetFunc
		kv.HasFunc
	}{
		func(key []byte) ([]byte, error) { return s.Get(key, &readOpt) },
		func(key []byte) (bool, error) { return s.Has(key, &readOpt) },
	})
}

func (ldb *levelEngine) Batch(fn func(kv.Putter) error) error {
	batch := ldb.batchPool.Get().(*leveldb.Batch)
	batch.Reset()
	defer ldb.batchPool.Put(batch)

	flushIfNeeded := func() error {
		// TODO: ideal batch size?
		if len(batch.Dump()) >= 32*1024 {
			if err := ldb.db.Write(batch, &writeOpt); err != nil {
				return err
			}
			batch.Reset()
		}
		return nil
	}

	if err := fn(&struct {
		kv.PutFunc
		kv.DeleteFunc
	}{
		func(key, val []byte) error {
			batch.Put(key, val)
			return flushIfNeeded()
		},
		func(key []byte) error {
			batch.Delete(key)
			return flushIfNeeded()
		},
	}); err != nil {
		return err
	}
	if batch.Len() == 0 {
		return nil
	}
	return ldb.db.Write(batch, &writeOpt)
}

func (ldb *levelEngine) Iterate(rng kv.Range, fn func(kv.Pair) bool) error {
	it := ldb.db.NewIterator((*util.Range)(&rng), &scanOpt)
	defer it.Release()

	for it.Next() {
		if !fn(it) {
			break
		}
	}
	return it.Error()
}
