package state

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestStage(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := New(thor.Hash{}, kv)

	addr := thor.BytesToAddress([]byte("acc1"))

	balance := big.NewInt(10)
	code := []byte{1, 2, 3}

	storage := map[thor.Hash]thor.Hash{
		thor.BytesToHash([]byte("s1")): thor.BytesToHash([]byte("v1")),
		thor.BytesToHash([]byte("s2")): thor.BytesToHash([]byte("v2")),
		thor.BytesToHash([]byte("s3")): thor.BytesToHash([]byte("v3"))}

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
