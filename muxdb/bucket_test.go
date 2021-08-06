// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
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

	got, err := db.Get(originalKey)
	assert.Equal(t, []byte(nil), got)
	assert.True(t, true, db.IsNotFound(err))

	assert.Equal(t, M(value, nil), M(getter.Get(originalKey)))
	assert.Equal(t, M(true, nil), M(getter.Has(originalKey)))

	assert.Nil(t, putter.Delete(originalKey))
	assert.Equal(t, M([]byte(nil), leveldb.ErrNotFound), M(getter.Get(originalKey)))
	assert.Equal(t, M(false, nil), M(getter.Has(originalKey)))
}

func Test_bucket_MakeRange(t *testing.T) {
	bkt := bucket("bucket")

	tests := []struct {
		name string
		arg  kv.Range
		want kv.Range
	}{
		{"empty", kv.Range{}, kv.Range{Start: bkt, Limit: util.BytesPrefix(bkt).Limit}},
		{"empty start", kv.Range{Limit: []byte("limit")}, kv.Range{Start: bkt, Limit: []byte(string(bkt) + "limit")}},
		{"empty limit", kv.Range{Start: []byte("start")}, kv.Range{Start: []byte(string(bkt) + "start"), Limit: util.BytesPrefix(bkt).Limit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bkt.MakeRange(tt.arg))
		})
	}
}
