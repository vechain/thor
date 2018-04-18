package builtin_test

import (
	"math"
	"math/big"
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
		expectedOutputs []interface{}
	}{
		{"native_getExecutor",
			nil,
			[]interface{}{common.Address(builtin.Executor.Address)}},
		{"native_set",
			[]interface{}{common.BytesToHash([]byte("key")), big.NewInt(1)},
			nil},
		{"native_get",
			[]interface{}{common.BytesToHash([]byte("key"))},
			[]interface{}{big.NewInt(1)}},
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

		out, err := method.EncodeOutput(tt.expectedOutputs...)
		assert.Nil(t, err, "should encode output of method "+tt.name)
		assert.Equal(t, out, vmout.Value, "shoudl match output of method: "+tt.name)
	}
}

func TestAuthorityNative(t *testing.T) {
	var (
		signer1   = common.BytesToAddress([]byte("signer1"))
		endorsor1 = common.BytesToAddress([]byte("endorsor1"))
		identity1 = common.BytesToHash([]byte("identity1"))

		signer2   = common.BytesToAddress([]byte("signer2"))
		endorsor2 = common.BytesToAddress([]byte("endorsor2"))
		identity2 = common.BytesToHash([]byte("identity2"))

		signer3   = common.BytesToAddress([]byte("signer3"))
		endorsor3 = common.BytesToAddress([]byte("endorsor3"))
		identity3 = common.BytesToHash([]byte("identity3"))
	)

	tests := []struct {
		name            string
		args            []interface{}
		expectedOutputs []interface{}
	}{
		{"native_getExecutor", nil, []interface{}{common.Address(builtin.Executor.Address)}},
		{"native_add", []interface{}{signer1, endorsor1, identity1}, []interface{}{true}},
		{"native_add", []interface{}{signer1, endorsor1, identity1}, []interface{}{false}},
		{"native_remove", []interface{}{signer1}, []interface{}{true}},
		{"native_add", []interface{}{signer1, endorsor1, identity1}, []interface{}{true}},
		{"native_add", []interface{}{signer2, endorsor2, identity2}, []interface{}{true}},
		{"native_add", []interface{}{signer3, endorsor3, identity3}, []interface{}{true}},
		{"native_get", []interface{}{signer1}, []interface{}{true, endorsor1, identity1, false}},
		{"native_first", nil, []interface{}{signer1}},
		{"native_next", []interface{}{signer1}, []interface{}{signer2}},
		{"native_next", []interface{}{signer2}, []interface{}{signer3}},
		{"native_next", []interface{}{signer3}, []interface{}{common.Address{}}},

		{"native_isEndorsed", []interface{}{signer1}, []interface{}{true}},
		{"native_isEndorsed", []interface{}{signer2}, []interface{}{false}},
	}

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	st.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
	st.SetBalance(thor.Address(endorsor1), thor.InitialProposerEndorsement)
	builtin.Params.Native(st).Set(thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)

	rt := runtime.New(st, thor.Address{}, 0, 0, 0, func(uint32) thor.Bytes32 { return thor.Bytes32{} })
	for _, tt := range tests {
		nabi := builtin.Authority.NativeABI()
		method, ok := nabi.MethodByName(tt.name)
		assert.True(t, ok, "should have method "+tt.name)
		data, err := method.EncodeInput(tt.args...)
		assert.Nil(t, err, "should encode input of method "+tt.name)

		vmout := rt.Call(tx.NewClause(&builtin.Authority.Address).WithData(data),
			0, math.MaxUint64, builtin.Authority.Address, &big.Int{}, thor.Bytes32{})
		assert.Nil(t, vmout.VMErr, "should execute method "+tt.name)

		out, err := method.EncodeOutput(tt.expectedOutputs...)
		assert.Nil(t, err, "should encode output of method "+tt.name)
		assert.Equal(t, out, vmout.Value, "should match output of method: "+tt.name)

	}
}
