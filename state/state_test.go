package state

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
)

func TestStateReadWrite(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := New(cry.Hash{}, kv)

	addr := acc.BytesToAddress([]byte("account1"))
	storageKey := cry.BytesToHash([]byte("storageKey"))

	assert.False(t, state.Exists(addr))
	assert.Equal(t, state.GetBalance(addr), &big.Int{})
	assert.Equal(t, state.GetCode(addr), []byte(nil))
	assert.Equal(t, state.GetCodeHash(addr), cry.Hash{})
	assert.Equal(t, state.GetStorage(addr, storageKey), cry.Hash{})

	state.SetStorage(addr, storageKey, cry.BytesToHash([]byte("storageValue")))
	assert.Equal(t,
		state.GetStorage(addr, storageKey),
		cry.Hash{},
		"should be no effect when set storage to an account without code")

	// make account not empty
	state.SetBalance(addr, big.NewInt(1))
	assert.Equal(t, state.GetBalance(addr), big.NewInt(1))

	state.SetCode(addr, []byte("code"))
	assert.Equal(t, state.GetCode(addr), []byte("code"))
	assert.Equal(t, state.GetCodeHash(addr), cry.HashSum([]byte("code")))

	assert.Equal(t, state.GetStorage(addr, storageKey), cry.Hash{})
	state.SetStorage(addr, storageKey, cry.BytesToHash([]byte("storageValue")))
	assert.Equal(t, state.GetStorage(addr, storageKey), cry.BytesToHash([]byte("storageValue")))

	assert.True(t, state.Exists(addr))

	// delete account
	state.Delete(addr)
	assert.False(t, state.Exists(addr))
	assert.Equal(t, state.GetBalance(addr), &big.Int{})
	assert.Equal(t, state.GetCode(addr), []byte(nil))
	assert.Equal(t, state.GetCodeHash(addr), cry.Hash{})
	assert.Equal(t, state.GetStorage(addr, storageKey), cry.Hash{})

	assert.Nil(t, state.Error(), "error is not expected")

}

func TestStateRevert(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := New(cry.Hash{}, kv)

	addr := acc.BytesToAddress([]byte("account1"))
	storageKey := cry.BytesToHash([]byte("storageKey"))

	values := []struct {
		balance *big.Int
		code    []byte
		storage cry.Hash
	}{
		{big.NewInt(1), []byte("code1"), cry.BytesToHash([]byte("v1"))},
		{big.NewInt(2), []byte("code2"), cry.BytesToHash([]byte("v2"))},
		{big.NewInt(3), []byte("code3"), cry.BytesToHash([]byte("v3"))},
	}

	for _, v := range values {
		state.NewCheckpoint()
		state.SetBalance(addr, v.balance)
		state.SetCode(addr, v.code)
		state.SetStorage(addr, storageKey, v.storage)
	}

	for i := range values {
		v := values[len(values)-i-1]
		assert.Equal(t, state.GetBalance(addr), v.balance)
		assert.Equal(t, state.GetCode(addr), v.code)
		assert.Equal(t, state.GetCodeHash(addr), cry.HashSum(v.code))
		assert.Equal(t, state.GetStorage(addr, storageKey), v.storage)
		state.Revert()
	}
	assert.False(t, state.Exists(addr))
	assert.Nil(t, state.Error(), "error is not expected")

	//
	state, _ = New(cry.Hash{}, kv)
	assert.Equal(t, state.NewCheckpoint(), 1)
	state.RevertTo(0)
	assert.Equal(t, state.NewCheckpoint(), 1)
	state.Revert()
	assert.Equal(t, state.NewCheckpoint(), 1)

}
