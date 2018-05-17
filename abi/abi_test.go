// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package abi_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin/gen"
	"github.com/vechain/thor/thor"
)

func TestABI(t *testing.T) {
	data := gen.MustAsset("compiled/Params.abi")
	abi, err := abi.New(data)
	assert.Nil(t, err)

	// pack/unpack input
	{
		name := "set"
		method, found := abi.MethodByName(name)
		assert.True(t, found)
		assert.NotNil(t, method)
		assert.Equal(t, name, method.Name())

		key := thor.BytesToBytes32([]byte("k"))
		value := big.NewInt(1)

		input, err := method.EncodeInput(key, value)
		assert.Nil(t, err)

		method, err = abi.MethodByInput(input)
		assert.Nil(t, err)
		assert.Equal(t, name, method.Name())

		var v struct {
			Key   common.Hash
			Value *big.Int
		}
		assert.Nil(t, method.DecodeInput(input, &v))
		assert.Equal(t, key, thor.Bytes32(v.Key))
		assert.Equal(t, value, v.Value)
	}

	// pack/unpack output
	{
		name := "get"
		method, found := abi.MethodByName(name)
		assert.True(t, found)
		assert.NotNil(t, method)

		value := big.NewInt(1)
		output, err := method.EncodeOutput(value)
		assert.Nil(t, err)

		var v *big.Int
		assert.Nil(t, method.DecodeOutput(output, &v))
		assert.Equal(t, value, v)
	}

	// pack/unpack event
	{
		name := "Set"
		event, found := abi.EventByName(name)
		assert.True(t, found)

		value := big.NewInt(999)

		data, err := event.Encode(value)
		assert.Nil(t, err)

		var d *big.Int

		err = event.Decode(data, &d)
		assert.Nil(t, err)

		assert.Equal(t, value, d)

	}
}
