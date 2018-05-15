package energy

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestEnergy(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))

	eng := New(thor.BytesToAddress([]byte("eng")), st)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{eng.GetBalance(acc, 0), &big.Int{}},
		{func() bool { eng.AddBalance(acc, big.NewInt(10), 0); return true }(), true},
		{eng.GetBalance(acc, 0), big.NewInt(10)},
		{eng.SubBalance(acc, big.NewInt(5), 0), true},
		{eng.SubBalance(acc, big.NewInt(6), 0), false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestEnergyGrowth(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))

	time1 := uint64(1000)

	eng := New(thor.BytesToAddress([]byte("eng")), st)

	eng.AddBalance(acc, &big.Int{}, 10)

	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)

	bal1 := eng.GetBalance(acc, time1)
	x := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(time1-10))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)

}
