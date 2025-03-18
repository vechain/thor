// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
	return a
}

func TestAuthority(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p1 := thor.BytesToAddress([]byte("p1"))
	p2 := thor.BytesToAddress([]byte("p2"))
	p3 := thor.BytesToAddress([]byte("p3"))

	st.SetBalance(p1, big.NewInt(10))
	st.SetBalance(p2, big.NewInt(20))
	st.SetBalance(p3, big.NewInt(30))

	aut := New(thor.BytesToAddress([]byte("aut")), st)
	tests := []struct {
		ret      any
		expected any
	}{
		{M(aut.Add(p1, p1, thor.Bytes32{})), M(true, nil)},
		{M(aut.Get(p1)), M(true, p1, thor.Bytes32{}, true, nil)},
		{M(aut.Add(p2, p2, thor.Bytes32{})), M(true, nil)},
		{M(aut.Add(p3, p3, thor.Bytes32{})), M(true, nil)},
		{M(aut.Candidates(big.NewInt(10), thor.InitialMaxBlockProposers)), M(
			[]*Candidate{{p1, p1, thor.Bytes32{}, true}, {p2, p2, thor.Bytes32{}, true}, {p3, p3, thor.Bytes32{}, true}}, nil,
		)},
		{M(aut.Candidates(big.NewInt(20), thor.InitialMaxBlockProposers)), M(
			[]*Candidate{{p2, p2, thor.Bytes32{}, true}, {p3, p3, thor.Bytes32{}, true}}, nil,
		)},
		{M(aut.Candidates(big.NewInt(30), thor.InitialMaxBlockProposers)), M(
			[]*Candidate{{p3, p3, thor.Bytes32{}, true}}, nil,
		)},
		{M(aut.Candidates(big.NewInt(10), 2)), M(
			[]*Candidate{{p1, p1, thor.Bytes32{}, true}, {p2, p2, thor.Bytes32{}, true}}, nil,
		)},
		{M(aut.Get(p1)), M(true, p1, thor.Bytes32{}, true, nil)},
		{M(aut.Update(p1, false)), M(true, nil)},
		{M(aut.Get(p1)), M(true, p1, thor.Bytes32{}, false, nil)},
		{M(aut.Update(p1, true)), M(true, nil)},
		{M(aut.Get(p1)), M(true, p1, thor.Bytes32{}, true, nil)},
		{M(aut.Revoke(p1)), M(true, nil)},
		{M(aut.Get(p1)), M(false, p1, thor.Bytes32{}, false, nil)},
		{M(aut.Candidates(&big.Int{}, thor.InitialMaxBlockProposers)), M(
			[]*Candidate{{p2, p2, thor.Bytes32{}, true}, {p3, p3, thor.Bytes32{}, true}}, nil,
		)},
		{M(aut.AllCandidates()), M([]*Candidate{
			{p2, p2, thor.Bytes32{}, true},
			{p3, p3, thor.Bytes32{}, true}}, nil),
		},
		{M(aut.First()), M(&p2, nil)},
		{M(aut.Next(p2)), M(&p3, nil)},
		{M(aut.Revoke(p1)), M(false, nil)},
		{M(aut.Revoke(p3)), M(true, nil)},
	}

	for i, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret, "#%v", i)
	}
}
