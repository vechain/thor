// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package lvldb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevelDB(t *testing.T) {

	var lvldbs []*LevelDB
	var (
		key        = []byte("123")
		value      = []byte("456")
		inValidKey = []byte("abc")
	)
	//TODO
	lvldb, err := New("/tmp/lvldbDB.tmp", Options{16, 16})

	defer lvldb.Close()
	assert.Equal(t, err, nil)
	lvldbs = append(lvldbs, lvldb)

	memlvldb, err := NewMem()
	defer memlvldb.Close()
	assert.Equal(t, err, nil)

	lvldbs = append(lvldbs, memlvldb)

	for _, leveldb := range lvldbs {

		err = leveldb.Put(key, value)
		assert.Equal(t, err, nil)

		ret1, err := leveldb.Get(key)
		assert.Equal(t, err, nil)

		ret2, err := leveldb.Has(key)
		assert.Equal(t, err, nil)

		ret3, err := leveldb.Has(inValidKey)
		assert.Equal(t, err, nil)

		err = leveldb.Delete(key)
		assert.Equal(t, err, nil)

		_, ret4 := leveldb.Get(key)

		tests := []struct {
			ret      interface{}
			expected interface{}
		}{
			{ret1, value},
			{ret2, true},
			{ret3, false},
			{leveldb.IsNotFound(ret4), true},
		}

		for _, tt := range tests {
			assert.Equal(t, tt.expected, tt.ret)
		}
	}
}

func TestLevelDBBach(t *testing.T) {
	var (
		key   = []byte("123")
		value = []byte("456")
	)
	lvldb, err := New("/tmp/lvldbDBBatch.tmp", Options{16, 16})

	defer lvldb.Close()
	assert.Equal(t, err, nil)

	dbBatch := lvldb.NewBatch()

	err = dbBatch.Put(key, value)
	assert.Equal(t, err, nil)

	ret1 := dbBatch.Len()
	err = dbBatch.Write()
	assert.Equal(t, err, nil)

	ret2, err := lvldb.Get(key)
	assert.Equal(t, err, nil)

	dbBatch = dbBatch.NewBatch()
	err = dbBatch.Put(key, value)
	assert.Equal(t, err, nil)

	err = dbBatch.Delete(key)
	assert.Equal(t, err, nil)

	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{ret1, 1},
		{ret2, value},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}
