// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func M(args ...interface{}) []interface{} {
	return args
}

func newMemDB() engine {
	ldb, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return newLevelEngine(ldb)
}

func Test_bucket_ProxyGetterPutter(t *testing.T) {
	db := newMemDB()
	originalKey := []byte("key")
	value := []byte("value")
	bkt := bucket("bucket")
	getter := bkt.ProxyGetter(db)
	putter := bkt.ProxyPutter(db)

	assert.Nil(t, putter.Put(originalKey, value))
	assert.Equal(t, M([]byte(nil), leveldb.ErrNotFound), M(db.Get(originalKey)))

	assert.Equal(t, M(value, nil), M(getter.Get(originalKey)))
	assert.Equal(t, M(true, nil), M(getter.Has(originalKey)))

	assert.Nil(t, putter.Delete(originalKey))
	assert.Equal(t, M([]byte(nil), leveldb.ErrNotFound), M(getter.Get(originalKey)))
	assert.Equal(t, M(false, nil), M(getter.Has(originalKey)))

}
