package state

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
)

func TestStage(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := New(cry.Hash{}, kv)

	addr := acc.BytesToAddress([]byte("acc1"))

	balance := big.NewInt(10)
	code := []byte{1, 2, 3}

	storage := map[cry.Hash]cry.Hash{
		cry.BytesToHash([]byte("s1")): cry.BytesToHash([]byte("v1")),
		cry.BytesToHash([]byte("s2")): cry.BytesToHash([]byte("v2")),
		cry.BytesToHash([]byte("s3")): cry.BytesToHash([]byte("v3"))}

	state.SetBalance(addr, balance)
	state.SetCode(addr, code)
	for k, v := range storage {
		state.SetStorage(addr, k, v)
	}

	stage := state.Stage()

	hash, err := stage.Hash()
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	assert.Equal(t, hash, root)

	state, _ = New(root, kv)

	assert.Equal(t, balance, state.GetBalance(addr))
	assert.Equal(t, code, state.GetCode(addr))
	assert.Equal(t, cry.HashSum(code), state.GetCodeHash(addr))
	for k, v := range storage {
		assert.Equal(t, v, state.GetStorage(addr, k))
	}
}
