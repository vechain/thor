package builtin_test

import (
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestParamsNative(t *testing.T) {
	tests := []struct {
		name            string
		args            []interface{}
		expectedOutputs interface{}
	}{
		{"native_getExecutor", nil, common.Address(builtin.Executor.Address)},
		{"native_set", []interface{}{common.BytesToHash([]byte("key")), big.NewInt(1)}, nil},
		{"native_get", []interface{}{common.BytesToHash([]byte("key"))}, big.NewInt(1)},
	}

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	st.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

	rt := runtime.New(st, thor.Address{}, 0, 0, 0, func(uint32) thor.Bytes32 { return thor.Bytes32{} })
	for _, tt := range tests {
		nabi := builtin.Params.NativeABI()
		method, ok := nabi.MethodByName(tt.name)
		assert.True(t, ok, "should have method "+tt.name)
		data, err := method.EncodeInput(tt.args...)
		assert.Nil(t, err, "should encode input of method "+tt.name)

		vmout := rt.Call(tx.NewClause(&builtin.Params.Address).WithData(data),
			0, math.MaxUint64, builtin.Params.Address, &big.Int{}, thor.Bytes32{})
		assert.Nil(t, vmout.VMErr, "should execute method "+tt.name)
		if tt.expectedOutputs == nil {
			assert.True(t, len(vmout.Value) == 0, "should no output of method "+tt.name)
		} else {
			cpy := reflect.New(reflect.TypeOf(tt.expectedOutputs))
			err = method.DecodeOutput(vmout.Value, cpy.Interface())
			assert.Nil(t, err, "should decode output of method "+tt.name)
			assert.Equal(t, tt.expectedOutputs, cpy.Elem().Interface(), "should have expected output of method "+tt.name)
		}
	}
}
