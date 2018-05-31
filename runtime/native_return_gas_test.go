package runtime

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

func TestNativeCallReturnGas(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := state.New(thor.Bytes32{}, kv)
	state.SetCode(builtin.Measure.Address, builtin.Measure.RuntimeBytecodes())

	inner, _ := builtin.Measure.ABI.MethodByName("inner")
	innerData, _ := inner.EncodeInput()
	outer, _ := builtin.Measure.ABI.MethodByName("outer")
	outerData, _ := outer.EncodeInput()

	ctx := vm.Context{}
	cfg := vm.Config{}

	innerOutput := vm.New(ctx, state, cfg).Call(
		thor.Address{},
		builtin.Measure.Address,
		innerData,
		math.MaxUint64,
		&big.Int{})
	assert.Nil(t, innerOutput.VMErr)

	outerOutput := vm.New(ctx, state, vm.Config{}).Call(
		thor.Address{},
		builtin.Measure.Address,
		outerData,
		math.MaxUint64,
		&big.Int{})
	assert.Nil(t, outerOutput.VMErr)

	innerGasUsed := math.MaxUint64 - innerOutput.LeftOverGas
	outerGasUsed := math.MaxUint64 - outerOutput.LeftOverGas

	// gas = enter1 + prepare2 + enter2 + leave2 + leave1
	// here returns prepare2
	assert.Equal(t, uint64(1562), outerGasUsed-innerGasUsed*2)
}
