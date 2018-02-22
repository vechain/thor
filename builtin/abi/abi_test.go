package abi_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin/abi"
	"github.com/vechain/thor/builtin/gen"
	"github.com/vechain/thor/thor"
)

func TestABI(t *testing.T) {
	data := gen.MustAsset("compiled/Params.abi")
	abi, err := abi.New(bytes.NewReader(data))
	assert.Nil(t, err)

	// pack/unpack input
	{
		method := "set"
		codec, err := abi.ForMethod(method)
		assert.Nil(t, err)
		assert.Equal(t, method, codec.Name())

		key := thor.BytesToHash([]byte("k"))
		value := big.NewInt(1)

		input, err := codec.EncodeInput(key, value)
		assert.Nil(t, err)

		name, err := abi.MethodName(input)
		assert.Nil(t, err)
		assert.Equal(t, method, name)

		var v struct {
			Key   common.Hash
			Value *big.Int
		}
		assert.Nil(t, codec.DecodeInput(input, &v))
		assert.Equal(t, key, thor.Hash(v.Key))
		assert.Equal(t, value, v.Value)
	}

	// pack/unpack output
	{
		method := "get"
		codec, err := abi.ForMethod(method)
		assert.Nil(t, err)

		value := big.NewInt(1)
		output, err := codec.EncodeOutput(value)
		assert.Nil(t, err)

		var v *big.Int
		assert.Nil(t, codec.DecodeOutput(output, &v))
		assert.Equal(t, value, v)
	}
}
