package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestAuthority(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	assert.True(t, len(Authority.RuntimeBytecodes()) > 0)

	p1 := thor.BytesToAddress([]byte("p1"))
	p2 := thor.BytesToAddress([]byte("p2"))
	p3 := thor.BytesToAddress([]byte("p3"))

	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{Authority.Add(st, p1, thor.Hash{}), true},
		{Authority.Add(st, p2, thor.Hash{}), true},
		{Authority.Add(st, p3, thor.Hash{}), true},
		{Authority.All(st), []poa.Proposer{{p1, 0}, {p2, 0}, {p3, 0}}},
		{M(Authority.Status(st, p1)), []interface{}{true, thor.Hash{}, uint32(0)}},
		{Authority.Update(st, p1, 1), true},
		{M(Authority.Status(st, p1)), []interface{}{true, thor.Hash{}, uint32(1)}},
		{Authority.Remove(st, p1), true},
		{Authority.All(st), []poa.Proposer{{p3, 0}, {p2, 0}}},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}

}
