package builtin

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestParams(t *testing.T) {
	assert.True(t, len(Params.RuntimeBytecodes()) > 0)
}

func TestParamsGetSet(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	setv := big.NewInt(10)
	key := thor.BytesToHash([]byte("key"))
	Params.Set(st, key, setv)

	getv := Params.Get(st, key)
	assert.Equal(t, setv, getv)
}
