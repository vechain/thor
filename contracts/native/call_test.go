package native

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/contracts/abi"
	"github.com/vechain/thor/contracts/gen"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

func TestCall(t *testing.T) {

	data := gen.MustAsset("compiled/Params.abi")
	abi, _ := abi.New(bytes.NewReader(data))

	mp, _ := abi.ForMethod("get")
	requiredCaller := thor.BytesToAddress([]byte("addr"))

	key := thor.BytesToHash([]byte("key"))
	value := big.NewInt(1)
	gas := uint64(1)

	callable := Callable{
		MethodPacker:   mp,
		Gas:            gas,
		RequiredCaller: &requiredCaller,
		AllocArg: func() interface{} {
			return &common.Hash{}
		},
		Proc: func(env *Env) ([]interface{}, error) {
			k, ok := env.Arg.(*common.Hash)
			assert.True(t, ok)
			assert.Equal(t, key, thor.Hash(*k))
			return []interface{}{value}, nil
		},
	}

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	input, _ := mp.PackInput(key)

	// good case
	out, err := callable.Call(st, &vm.Context{}, requiredCaller, func(_gas uint64) bool {
		assert.Equal(t, gas, _gas)
		return true
	}, input)

	assert.Nil(t, err)

	var v *big.Int
	assert.Nil(t, mp.UnpackOutput(out, &v))
	assert.Equal(t, value, v)

	// bad cases
	_, err = callable.Call(st, &vm.Context{}, thor.Address{}, func(_gas uint64) bool {
		return true
	}, input)

	assert.Equal(t, errNativeNotPermitted, err)

	_, err = callable.Call(st, &vm.Context{}, requiredCaller, func(_gas uint64) bool {
		return false
	}, input)

	assert.Equal(t, evm.ErrOutOfGas, err)

}
