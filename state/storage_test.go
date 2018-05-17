// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestStorageCodec(t *testing.T) {
	tests := []interface{}{
		thor.BytesToBytes32([]byte("Bytes32")),
		thor.BytesToAddress([]byte("address")),
		"foo",
		uint(10),
		uint8(20),
		uint16(257),
		uint32(65537),
		uint64(5e9),
	}
	for _, tt := range tests {
		{
			data, err := encodeStorage(tt)
			assert.Nil(t, err)
			cpy := reflect.New(reflect.TypeOf(tt))
			err = decodeStorage(data, cpy.Interface())
			assert.Nil(t, err)
			assert.Equal(t, tt, cpy.Elem().Interface())
		}
		// test pointer type encoding
		{
			ptr := reflect.New(reflect.TypeOf(tt))
			ptr.Elem().Set(reflect.ValueOf(tt))
			data, err := encodeStorage(ptr.Interface())
			assert.Nil(t, err)
			cpy := reflect.New(reflect.TypeOf(tt))
			err = decodeStorage(data, cpy.Interface())
			assert.Nil(t, err)
			assert.Equal(t, tt, cpy.Elem().Interface())
		}
		// test zero value encoding
		{
			data, err := encodeStorage(reflect.Zero(reflect.TypeOf(tt)).Interface())
			assert.Nil(t, err)
			assert.Zero(t, len(data), reflect.TypeOf(tt).String())
		}
		// test zero value decoding
		{
			cpy := reflect.New(reflect.TypeOf(tt))
			err := decodeStorage(nil, cpy.Interface())
			assert.Nil(t, err)
			assert.Equal(t, reflect.Zero(reflect.TypeOf(tt)).Interface(), cpy.Elem().Interface())
		}
	}

	// special test for big.Int
	{
		data, err := encodeStorage(&big.Int{})
		assert.Nil(t, err)
		assert.Zero(t, len(data))

		bi := big.NewInt(10)
		err = decodeStorage(nil, bi)
		assert.Nil(t, err)
		assert.Equal(t, uint64(0), bi.Uint64())

		data, _ = encodeStorage(big.NewInt(10))
		err = decodeStorage(data, bi)
		assert.Nil(t, err)
		assert.Equal(t, uint64(10), bi.Uint64())
	}
}

func BenchmarkStorageSet(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := New(thor.Bytes32{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))
	key := thor.BytesToBytes32([]byte("key"))
	for i := 0; i < b.N; i++ {
		st.SetStorage(addr, key, thor.BytesToBytes32([]byte{1}))
	}
}

func BenchmarkStorageGet(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := New(thor.Bytes32{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))
	key := thor.BytesToBytes32([]byte("key"))
	st.SetStructuredStorage(addr, key, thor.Bytes32{1})
	for i := 0; i < b.N; i++ {
		st.GetStorage(addr, key)
	}
}
