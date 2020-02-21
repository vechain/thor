// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// func M(a ...interface{}) []interface{} {
// 	return a
// }

func TestAuthorityV2(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, thor.Bytes32{})

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
			M(aut.Add2(p1, p1, thor.Bytes32{}, pk1)),
			M(true, nil),
		},
		{
			M(aut.Get2(p1)),
			M(true, p1, thor.Bytes32{}, true, pk1, nil),
		},
		{
			M(aut.Add2(p2, p2, thor.Bytes32{}, pk2)),
			M(true, nil),
		},
		{
			M(aut.Add2(p3, p3, thor.Bytes32{}, pk3)),
			M(true, nil),
		},
		{
			M(aut.Candidates2(big.NewInt(10), thor.MaxBlockProposers)),
			M([]*Candidate{
				{p1, p1, thor.Bytes32{}, true, pk1},
				{p2, p2, thor.Bytes32{}, true, pk2},
				{p3, p3, thor.Bytes32{}, true, pk3}}, nil),
		},
		{
			M(aut.Candidates2(big.NewInt(20), thor.MaxBlockProposers)),
			M([]*Candidate{
				{p2, p2, thor.Bytes32{}, true, pk2},
				{p3, p3, thor.Bytes32{}, true, pk3}}, nil),
		},
		{
			M(aut.Candidates2(big.NewInt(30), thor.MaxBlockProposers)),
			M([]*Candidate{{p3, p3, thor.Bytes32{}, true, pk3}}, nil),
		},
		{
			M(aut.Candidates2(big.NewInt(10), 2)),
			M([]*Candidate{
				{p1, p1, thor.Bytes32{}, true, pk1},
				{p2, p2, thor.Bytes32{}, true, pk2}}, nil),
		},
		{
			M(aut.Get2(p1)),
			M(true, p1, thor.Bytes32{}, true, pk1, nil),
		},
		{
			M(aut.Update2(p1, false)),
			M(true, nil),
		},
		{
			M(aut.Get2(p1)),
			M(true, p1, thor.Bytes32{}, false, pk1, nil),
		},
		{
			M(aut.Update2(p1, true)),
			M(true, nil),
		},
		{
			M(aut.Get2(p1)),
			M(true, p1, thor.Bytes32{}, true, pk1, nil),
		},
		{
			M(aut.Revoke2(p1)),
			M(true, nil),
		},
		{
			M(aut.Get2(p1)),
			M(false, p1, thor.Bytes32{}, false, pk1, nil),
		},
		{
			M(aut.Candidates2(&big.Int{}, thor.MaxBlockProposers)),
			M([]*Candidate{
				{p2, p2, thor.Bytes32{}, true, pk2},
				{p3, p3, thor.Bytes32{}, true, pk3}},
				nil),
		},
	}

	for i, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret, "#%v", i)
	}

}
