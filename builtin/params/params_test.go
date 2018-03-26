package params

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestParamsGetSet(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	setv := big.NewInt(10)
	key := thor.BytesToHash([]byte("key"))
	p := New(thor.BytesToAddress([]byte("par")), st)
	p.Set(key, setv)

	getv := p.Get(key)
	assert.Equal(t, setv, getv)
}
