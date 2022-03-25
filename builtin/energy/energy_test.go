// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestEnergy(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, thor.Bytes32{}, 0, 0, 0)

	acc := thor.BytesToAddress([]byte("a1"))

	eng := New(thor.BytesToAddress([]byte("eng")), st, 0)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{M(eng.Get(acc)), M(&big.Int{}, nil)},
		{eng.Add(acc, big.NewInt(10)), nil},
		{M(eng.Get(acc)), M(big.NewInt(10), nil)},
		{M(eng.Sub(acc, big.NewInt(5))), M(true, nil)},
		{M(eng.Sub(acc, big.NewInt(6))), M(false, nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestEnergyGrowth(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, thor.Bytes32{}, 0, 0, 0)

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
