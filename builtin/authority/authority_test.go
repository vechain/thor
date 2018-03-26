package authority

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestAuthority(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	p1 := thor.BytesToAddress([]byte("p1"))
	p2 := thor.BytesToAddress([]byte("p2"))
	p3 := thor.BytesToAddress([]byte("p3"))

	aut := New(thor.BytesToAddress([]byte("aut")), st)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{aut.Add(p1, p1, thor.Hash{}), true},
		{aut.Add(p2, p2, thor.Hash{}), true},
		{aut.Add(p3, p3, thor.Hash{}), true},
		{M(aut.Pick()), []interface{}{
			[]*Proposer{{p1, thor.Hash{}, false, nil, &p2}, {p2, thor.Hash{}, false, &p1, &p3}, {p3, thor.Hash{}, false, &p2, nil}},
			[]thor.Address{p1, p2, p3},
		}},
		{aut.Get(p1), &Proposer{p1, thor.Hash{}, false, nil, &p2}},
		{aut.Update(p1, true), true},
		{aut.Get(p1), &Proposer{p1, thor.Hash{}, true, nil, &p2}},
		{aut.Remove(p1), true},
		{M(aut.Pick()), []interface{}{
			[]*Proposer{{p2, thor.Hash{}, false, nil, &p3}, {p3, thor.Hash{}, false, &p2, nil}},
			[]thor.Address{p2, p3},
		}},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}

}
