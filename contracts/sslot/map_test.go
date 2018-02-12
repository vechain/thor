package sslot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/contracts/sslot"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"

	"github.com/vechain/thor/lvldb"
)

func TestMap(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	addr := thor.BytesToAddress([]byte("acc"))
	m := sslot.NewMap(addr, 0)

	ss := m.ForKey("key")

	v1 := uint32(1)
	ss.SaveStructed(st, v1)

	var v2 uint32
	ss.LoadStructed(st, &v2)
	assert.Equal(t, v1, v2)
}
