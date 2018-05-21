// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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
	st, _ := state.New(thor.Bytes32{}, kv)

	p1 := thor.BytesToAddress([]byte("p1"))
	p2 := thor.BytesToAddress([]byte("p2"))
	p3 := thor.BytesToAddress([]byte("p3"))

	aut := New(thor.BytesToAddress([]byte("aut")), st)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{aut.Add(p1, p1, thor.Bytes32{}), true},
		{aut.Add(p2, p2, thor.Bytes32{}), true},
		{aut.Add(p3, p3, thor.Bytes32{}), true},
		{M(aut.Candidates()), []interface{}{
			[]*Candidate{{p1, p1, thor.Bytes32{}, true}, {p2, p2, thor.Bytes32{}, true}, {p3, p3, thor.Bytes32{}, true}},
		}},
		{aut.Get(p1), &Entry{p1, thor.Bytes32{}, true, nil, &p2}},
		{aut.Update(p1, false), true},
		{aut.Get(p1), &Entry{p1, thor.Bytes32{}, false, nil, &p2}},
		{aut.Remove(p1), true},
		{M(aut.Candidates()), []interface{}{
			[]*Candidate{{p2, p2, thor.Bytes32{}, true}, {p3, p3, thor.Bytes32{}, true}},
		}},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}

}
