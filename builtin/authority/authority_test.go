// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"math/big"
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

	pk1 := thor.BytesToBytes32([]byte("pk1"))
	pk2 := thor.BytesToBytes32([]byte("pk2"))
	pk3 := thor.BytesToBytes32([]byte("pk3"))

	st.SetBalance(p1, big.NewInt(10))
	st.SetBalance(p2, big.NewInt(20))
	st.SetBalance(p3, big.NewInt(30))

	aut := New(thor.BytesToAddress([]byte("aut")), st)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{
			aut.Add(p1, p1, thor.Bytes32{}, pk1),
			true,
		},
		{
			M(aut.Get(p1)),
			[]interface{}{true, p1, thor.Bytes32{}, true, pk1},
		},
		{
			aut.Add(p2, p2, thor.Bytes32{}, pk2),
			true,
		},
		{
			aut.Add(p3, p3, thor.Bytes32{}, pk3),
			true,
		},
		{
			M(aut.Candidates(big.NewInt(10), thor.MaxBlockProposers)),
			[]interface{}{[]*Candidate{
				{p1, p1, thor.Bytes32{}, true, pk1},
				{p2, p2, thor.Bytes32{}, true, pk2},
				{p3, p3, thor.Bytes32{}, true, pk3}}},
		},
		{
			M(aut.Candidates(big.NewInt(20), thor.MaxBlockProposers)),
			[]interface{}{[]*Candidate{
				{p2, p2, thor.Bytes32{}, true, pk2},
				{p3, p3, thor.Bytes32{}, true, pk3}}},
		},
		{
			M(aut.Candidates(big.NewInt(30), thor.MaxBlockProposers)),
			[]interface{}{[]*Candidate{{p3, p3, thor.Bytes32{}, true, pk3}}},
		},
		{
			M(aut.Candidates(big.NewInt(10), 2)),
			[]interface{}{[]*Candidate{
				{p1, p1, thor.Bytes32{}, true, pk1},
				{p2, p2, thor.Bytes32{}, true, pk2}}},
		},
		{
			M(aut.Get(p1)),
			[]interface{}{true, p1, thor.Bytes32{}, true, pk1},
		},
		{
			aut.Update(p1, false),
			true,
		},
		{
			M(aut.Get(p1)),
			[]interface{}{true, p1, thor.Bytes32{}, false, pk1},
		},
		{
			aut.Update(p1, true),
			true,
		},
		{
			M(aut.Get(p1)),
			[]interface{}{true, p1, thor.Bytes32{}, true, pk1},
		},
		{
			aut.Revoke(p1),
			true,
		},
		{
			M(aut.Get(p1)),
			[]interface{}{false, p1, thor.Bytes32{}, false, pk1},
		},
		{
			M(aut.Candidates(&big.Int{}, thor.MaxBlockProposers)),
			[]interface{}{[]*Candidate{
				{p2, p2, thor.Bytes32{}, true, pk2},
				{p3, p3, thor.Bytes32{}, true, pk3}},
			},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}

	assert.Nil(t, st.Err())

}
