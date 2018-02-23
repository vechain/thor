package builtin

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
	assert.True(t, len(Energy.RuntimeBytecodes()) > 0)

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))
	contractAddr := thor.BytesToAddress([]byte("c1"))

	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{Energy.GetBalance(st, 0, acc), &big.Int{}},
		{func() bool { Energy.SetBalance(st, 0, acc, big.NewInt(10)); return true }(), true},
		{Energy.GetBalance(st, 0, acc), big.NewInt(10)},
		{func() bool { Energy.SetContractMaster(st, contractAddr, acc); return true }(), true},
		{Energy.GetContractMaster(st, contractAddr), acc},
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

	Energy.SetBalance(st, 0, acc, &big.Int{})
	Energy.AdjustGrowthRate(st, 0, thor.InitialEnergyGrowthRate)

	bal1 := Energy.GetBalance(st, blockTime1, acc)
	x := new(big.Int).Mul(thor.InitialEnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(blockTime1))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)

	blockTime2 := uint64(2000)
	rate2 := new(big.Int).Mul(thor.InitialEnergyGrowthRate, big.NewInt(2))
	Energy.AdjustGrowthRate(st, blockTime2, rate2)

	blockTime3 := uint64(3000)
	bal2 := Energy.GetBalance(st, blockTime3, acc)

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
	callee := thor.BytesToAddress([]byte("callee"))
	blockTime1 := uint64(1000)
	bal := big.NewInt(1e18)
	credit := big.NewInt(1e18)
	recRate := big.NewInt(100)
	exp := uint64(2000)

	Energy.SetBalance(st, blockTime1, callee, bal)
	Energy.SetSharing(st, blockTime1, callee, caller, credit, recRate, exp)

	remained := Energy.GetSharingRemained(st, blockTime1, callee, caller)
	assert.Equal(t, credit, remained)

	consumed := big.NewInt(1e9)
	payer, ok := Energy.Consume(st, blockTime1, caller, callee, consumed)
	assert.Equal(t, callee, payer)
	assert.True(t, ok)

	remained = Energy.GetSharingRemained(st, blockTime1, callee, caller)
	assert.Equal(t, new(big.Int).Sub(credit, consumed), remained)

	blockTime2 := uint64(1500)
	remained = Energy.GetSharingRemained(st, blockTime2, callee, caller)
	x := new(big.Int).SetUint64(blockTime2 - blockTime1)
	x.Mul(x, recRate)
	x.Add(x, credit)
	x.Sub(x, consumed)
	assert.Equal(t, x, remained)

	remained = Energy.GetSharingRemained(st, math.MaxUint64, callee, caller)
	assert.Equal(t, &big.Int{}, remained)
}
