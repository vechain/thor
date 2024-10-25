// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package thor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytes32_UnmarshalJSON(t *testing.T) {
	// Example hex string representing the value 100
	originalHex := `"0x00000000000000000000000000000000000000000000000000006d6173746572"` // Note the enclosing double quotes for valid JSON string

	// Unmarshal JSON into HexOrDecimal256
	var unmarshaledValue Bytes32

	// using direct function
	err := unmarshaledValue.UnmarshalJSON([]byte(originalHex))
	assert.NoError(t, err)

	// using json overloading ( satisfies the json.Unmarshal interface )
	err = json.Unmarshal([]byte(originalHex), &unmarshaledValue)
	assert.NoError(t, err)

	// Marshal the value back to JSON
	// using direct function
	directMarshallJSON, err := unmarshaledValue.MarshalJSON()
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, originalHex, string(directMarshallJSON))

	// using json overloading ( satisfies the json.Unmarshal interface )
	// using value
	marshalVal, err := json.Marshal(unmarshaledValue)
	assert.NoError(t, err)
	assert.Equal(t, originalHex, string(marshalVal))

	// using json overloading ( satisfies the json.Unmarshal interface )
	// using pointer
	marshalPtr, err := json.Marshal(&unmarshaledValue)
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, originalHex, string(marshalPtr))

	// Marshal a zero value
	var b Bytes32
	j, err := b.MarshalJSON()
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, `"0x0000000000000000000000000000000000000000000000000000000000000000"`, string(j))
}

func TestParseBytes32(t *testing.T) {
	// Example hex string representing the value 100
	expected := MustParseBytes32("0x0000000000000000000000006d95e6dca01d109882fe1726a2fb9865fa41e7aa")
	trimmed := "0x6d95e6dca01d109882fe1726a2fb9865fa41e7aa"
	parsed, err := ParseBytes32(trimmed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)
}
