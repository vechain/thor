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
	st, _ := state.New(thor.Bytes32{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))
	contractAddr := thor.BytesToAddress([]byte("c1"))

	eng := New(thor.BytesToAddress([]byte("eng")), st)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{eng.GetBalance(acc, 0), &big.Int{}},
		{func() bool { eng.AddBalance(acc, 0, big.NewInt(10)); return true }(), true},
		{eng.GetBalance(acc, 0), big.NewInt(10)},
		{eng.SubBalance(acc, 0, big.NewInt(5)), true},
		{eng.SubBalance(acc, 0, big.NewInt(6)), false},
		{func() bool { eng.SetContractMaster(contractAddr, acc); return true }(), true},
		{eng.GetContractMaster(contractAddr), acc},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestEnergyGrowth(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))

	blockNum1 := uint32(1000)

	eng := New(thor.BytesToAddress([]byte("eng")), st)

	eng.AddBalance(acc, 10, &big.Int{})

	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)

	bal1 := eng.GetBalance(acc, blockNum1)
	x := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(uint64(blockNum1-10)))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)

}

func TestEnergyShare(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)

	caller := thor.BytesToAddress([]byte("caller"))
	contract := thor.BytesToAddress([]byte("contract"))
	blockNum1 := uint32(1000)
	bal := big.NewInt(1e18)
	credit := big.NewInt(1e18)
	recRate := big.NewInt(100)
	exp := uint32(2000)

	eng := New(thor.BytesToAddress([]byte("eng")), st)
	eng.AddBalance(contract, blockNum1, bal)
	eng.ApproveConsumption(blockNum1, contract, caller, credit, recRate, exp)

	remained := eng.GetConsumptionAllowance(blockNum1, contract, caller)
	assert.Equal(t, credit, remained)

	consumed := big.NewInt(1e9)
	payer, ok := eng.Consume(blockNum1, &contract, caller, consumed)
	assert.Equal(t, contract, payer)
	assert.True(t, ok)

	remained = eng.GetConsumptionAllowance(blockNum1, contract, caller)
	assert.Equal(t, new(big.Int).Sub(credit, consumed), remained)

	blockNum2 := uint32(1500)
	remained = eng.GetConsumptionAllowance(blockNum2, contract, caller)
	x := new(big.Int).SetUint64(uint64(blockNum2 - blockNum1))
	x.Mul(x, recRate)
	x.Add(x, credit)
	x.Sub(x, consumed)
	assert.Equal(t, x, remained)

	remained = eng.GetConsumptionAllowance(math.MaxUint32, contract, caller)
	assert.Equal(t, &big.Int{}, remained)
}
