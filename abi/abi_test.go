// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package abi_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin/gen"
	"github.com/vechain/thor/v2/thor"
)

func TestABI(t *testing.T) {
	data := gen.MustABI("compiled/Params.abi")
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

func TestStakerABI(t *testing.T) {
	data := gen.MustABI("compiled/Staker.abi")
	abi, err := abi.New(data)
	assert.Nil(t, err)

	type testCase struct {
		name     string
		constant bool
	}

	testCases := []testCase{
		{"totalStake", true},
		{"queuedStake", true},
		{"addValidation", false},
		{"withdrawStake", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			method, found := abi.MethodByName(tc.name)
			assert.True(t, found)
			assert.NotNil(t, method)
			assert.Equal(t, tc.constant, method.Const())
		})
	}
}

func TestConstructorWithParameters(t *testing.T) {
	abiJSON := []byte(`[
		{
			"inputs": [
				{"name": "_value", "type": "uint256"},
				{"name": "_owner", "type": "address"},
				{"name": "_name", "type": "string"}
			],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "constructor"
		}
	]`)

	abi, err := abi.New(abiJSON)
	assert.Nil(t, err)

	constructor := abi.Constructor()
	assert.NotNil(t, constructor)
	assert.Equal(t, "", constructor.Name())
	assert.Equal(t, false, constructor.Const())

	// Test encoding constructor with parameters
	value := big.NewInt(12345)
	owner := common.HexToAddress("0x1234567890123456789012345678901234567890")
	name := "TestContract"

	input, err := constructor.EncodeInput(value, owner, name)
	assert.Nil(t, err)

	// Constructor input should NOT include MethodID (4 bytes)
	// It should only contain the encoded parameters
	assert.NotNil(t, input)

	// The encoded data should start directly with the parameters,
	// not with 0x00000000 (empty MethodID)
	// For a constructor with parameters, the data should be > 0 bytes
	assert.Greater(t, len(input), 0)

	// Log the actual output to see what we're getting
	t.Logf("Constructor input length: %d bytes", len(input))
	t.Logf("Constructor input hex: %x", input)

	// Verify the MethodID is empty (all zeros)
	methodID := constructor.ID()
	assert.Equal(t, [4]byte{0x00, 0x00, 0x00, 0x00}, [4]byte(methodID))

	// Verify that constructor data does NOT start with MethodID
	// The constructor data should be pure ABI encoding without 4-byte selector
	// Expected structure for constructor(uint256 _value, address _owner, string _name):
	// - bytes 0-31:   uint256 value (12345 right-padded in 32 bytes)
	// - bytes 32-63:  address owner (20 bytes, left-padded to 32 bytes)
	// - bytes 64-95:  offset to string data (96 in decimal = 0x60)
	// - bytes 96-127: string length
	// - bytes 128+:   string data

	// The total length should be 160 bytes (5 * 32)
	assert.Equal(t, 160, len(input), "Constructor data should be 160 bytes")

	// Verify first 32 bytes contain the uint256 value (12345)
	decodedValue := new(big.Int).SetBytes(input[:32])
	assert.Equal(t, value, decodedValue, "First 32 bytes should contain value 12345")
}

func TestConstructorWithoutParameters(t *testing.T) {
	abiJSON := []byte(`[
		{
			"inputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "constructor"
		}
	]`)

	abi, err := abi.New(abiJSON)
	assert.Nil(t, err)

	constructor := abi.Constructor()
	assert.NotNil(t, constructor)
	assert.Equal(t, "", constructor.Name())

	// Test encoding constructor without parameters
	input, err := constructor.EncodeInput()
	assert.Nil(t, err)

	// Log the actual output
	t.Logf("Constructor input length: %d bytes", len(input))
	t.Logf("Constructor input hex: %x", input)

	// Constructor without parameters should return empty data (0 bytes)
	assert.Len(t, input, 0, "Constructor without parameters should return 0 bytes")
}
