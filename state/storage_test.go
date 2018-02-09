package state

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestHashStorageCodec(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := New(thor.Hash{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))
	key := thor.BytesToHash([]byte("key"))
	value := thor.BytesToHash([]byte("value"))

	st.SetBalance(addr, big.NewInt(1))
	st.SetStructedStorage(addr, key, stgHash(value))
	var rv stgHash
	st.GetStructedStorage(addr, key, &rv)
	assert.Equal(t, value, thor.Hash(rv))

	root, _ := st.Stage().Commit()

	st, _ = New(root, kv)
	rv = stgHash{}
	st.GetStructedStorage(addr, key, &rv)
	assert.Equal(t, value, thor.Hash(rv))

	var emtpyHashStorage stgHash
	assert.Equal(t, M(emtpyHashStorage.Encode()), []interface{}{[]byte(nil), nil})
	assert.Nil(t, emtpyHashStorage.Decode(nil))
}
