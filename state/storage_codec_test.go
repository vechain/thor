package state_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"

	"github.com/vechain/thor/lvldb"
)

func TestHashStorageCodec(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))
	key := thor.BytesToHash([]byte("key"))
	value := thor.BytesToHash([]byte("value"))

	st.SetBalance(addr, big.NewInt(1))
	st.SetStructedStorage(addr, state.HashStorageCodec, key, value)
	v, ok := st.GetStructedStorage(addr, state.HashStorageCodec, key).(thor.Hash)
	assert.True(t, ok)
	assert.Equal(t, value, v)

	root, _ := st.Stage().Commit()

	st, _ = state.New(root, kv)
	v, ok = st.GetStructedStorage(addr, state.HashStorageCodec, key).(thor.Hash)
	assert.True(t, ok)
	assert.Equal(t, value, v)

}

func TestAddressStorageCodec(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))
	key := thor.BytesToHash([]byte("key"))
	value := thor.BytesToAddress([]byte("value"))

	st.SetBalance(addr, big.NewInt(1))
	st.SetStructedStorage(addr, state.AddressStorageCodec, key, value)
	v, ok := st.GetStructedStorage(addr, state.AddressStorageCodec, key).(thor.Address)
	assert.True(t, ok)
	assert.Equal(t, value, v)

	root, _ := st.Stage().Commit()

	st, _ = state.New(root, kv)
	v, ok = st.GetStructedStorage(addr, state.AddressStorageCodec, key).(thor.Address)
	assert.True(t, ok)
	assert.Equal(t, value, v)
}
