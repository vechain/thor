// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

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

func TestEnergy(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	acc := thor.BytesToAddress([]byte("a1"))

	eng := New(thor.BytesToAddress([]byte("eng")), st, 0)
	tests := []struct {
		ret      any
		expected any
	}{
		{M(eng.Get(acc)), M(&big.Int{}, nil)},
		{eng.Add(acc, big.NewInt(10)), nil},
		{M(eng.Get(acc)), M(big.NewInt(10), nil)},
		{M(eng.Sub(acc, big.NewInt(5))), M(true, nil)},
		{M(eng.Sub(acc, big.NewInt(6))), M(false, nil)},
		{eng.Add(acc, big.NewInt(0)), nil},
		{M(eng.Sub(acc, big.NewInt(0))), M(true, nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestInitialSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	eng := New(thor.BytesToAddress([]byte("eng")), st, 0)

	// get initial supply before set should return 0
	supply, err := eng.getInitialSupply()
	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(0), Energy: big.NewInt(0), BlockTime: 0x0})

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	supply, err = eng.getInitialSupply()
	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(123), Energy: big.NewInt(456), BlockTime: 0x0})
}

func TestInitialSupplyError(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	eng := New(thor.BytesToAddress([]byte("a1")), st, 0)

	eng.SetInitialSupply(big.NewInt(0), big.NewInt(0))

	supply, err := eng.getInitialSupply()

	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(0), Energy: big.NewInt(0), BlockTime: 0x0})
}

func TestTotalSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	eng := New(thor.BytesToAddress([]byte("eng")), st, 0)

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	totalSupply, err := eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, totalSupply, big.NewInt(456))
}

func TestTokenTotalSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	eng := New(thor.BytesToAddress([]byte("eng")), st, 0)

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	totalTokenSupply, err := eng.TokenTotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, totalTokenSupply, big.NewInt(123))
}

func TestTotalBurned(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	eng := New(thor.BytesToAddress([]byte("eng")), st, 0)

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	totalBurned, err := eng.TotalBurned()

	assert.Nil(t, err)
	assert.Equal(t, totalBurned, big.NewInt(0))
}

func TestEnergyGrowth(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	acc := thor.BytesToAddress([]byte("a1"))

	st.SetEnergy(acc, &big.Int{}, 10)

	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)

	bal1, err := New(thor.Address{}, st, 1000).
		Get(acc)

	assert.Nil(t, err)

	x := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(1000-10))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)
}
