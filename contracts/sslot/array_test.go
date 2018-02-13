package sslot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/contracts/sslot"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestArray(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))

	array := sslot.NewArray(addr, 0)

	assert.Zero(t, array.Len(st))

	assert.Equal(t, uint64(1), array.Append(st, "s"))

	var v string
	array.ForIndex(0).Load(st, &v)
	assert.Equal(t, "s", v)

	array.SetLen(st, 10)
	assert.Equal(t, uint64(10), array.Len(st))

	array.SetLen(st, 0)
	assert.Zero(t, array.Len(st))

	array.ForIndex(0).Load(st, &v)
	assert.Equal(t, "", v)
}
