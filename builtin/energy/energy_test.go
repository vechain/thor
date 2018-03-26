package energy

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestEnergy(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))
	contractAddr := thor.BytesToAddress([]byte("c1"))

	eng := New(thor.BytesToAddress([]byte("eng")), st)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{eng.GetBalance(0, acc), &big.Int{}},
		{func() bool { eng.AddBalance(0, acc, big.NewInt(10)); return true }(), true},
		{eng.GetBalance(0, acc), big.NewInt(10)},
		{eng.SubBalance(0, acc, big.NewInt(5)), true},
		{eng.SubBalance(0, acc, big.NewInt(6)), false},
		{func() bool { eng.SetContractMaster(contractAddr, acc); return true }(), true},
		{eng.GetContractMaster(contractAddr), acc},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestEnergyGrowth(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))

	blockTime1 := uint64(1000)

	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)

	eng := New(thor.BytesToAddress([]byte("eng")), st)

	eng.AddBalance(0, acc, &big.Int{})
	eng.AdjustGrowthRate(0, thor.InitialEnergyGrowthRate)

	bal1 := eng.GetBalance(blockTime1, acc)
	x := new(big.Int).Mul(thor.InitialEnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(blockTime1))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)

	blockTime2 := uint64(2000)
	rate2 := new(big.Int).Mul(thor.InitialEnergyGrowthRate, big.NewInt(2))
	eng.AdjustGrowthRate(blockTime2, rate2)

	blockTime3 := uint64(3000)
	bal2 := eng.GetBalance(blockTime3, acc)

	x.Mul(thor.InitialEnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(blockTime2))
	x.Div(x, big.NewInt(1e18))

	y := new(big.Int).Mul(rate2, vetBal)
	y.Mul(y, new(big.Int).SetUint64(blockTime3-blockTime2))
	y.Div(y, big.NewInt(1e18))

	x.Add(x, y)
	assert.Equal(t, x, bal2)
}

func TestEnergyShare(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	caller := thor.BytesToAddress([]byte("caller"))
	contract := thor.BytesToAddress([]byte("contract"))
	blockTime1 := uint64(1000)
	bal := big.NewInt(1e18)
	credit := big.NewInt(1e18)
	recRate := big.NewInt(100)
	exp := uint64(2000)

	eng := New(thor.BytesToAddress([]byte("eng")), st)
	eng.AddBalance(blockTime1, contract, bal)
	eng.ApproveConsumption(blockTime1, contract, caller, credit, recRate, exp)

	remained := eng.GetConsumptionAllowance(blockTime1, contract, caller)
	assert.Equal(t, credit, remained)

	consumed := big.NewInt(1e9)
	payer, ok := eng.Consume(blockTime1, contract, caller, consumed)
	assert.Equal(t, contract, payer)
	assert.True(t, ok)

	remained = eng.GetConsumptionAllowance(blockTime1, contract, caller)
	assert.Equal(t, new(big.Int).Sub(credit, consumed), remained)

	blockTime2 := uint64(1500)
	remained = eng.GetConsumptionAllowance(blockTime2, contract, caller)
	x := new(big.Int).SetUint64(blockTime2 - blockTime1)
	x.Mul(x, recRate)
	x.Add(x, credit)
	x.Sub(x, consumed)
	assert.Equal(t, x, remained)

	remained = eng.GetConsumptionAllowance(math.MaxUint64, contract, caller)
	assert.Equal(t, &big.Int{}, remained)
}
