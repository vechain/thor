// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package thor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalUnmarshall(t *testing.T) {
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
	directMarshallJson, err := unmarshaledValue.MarshalJSON()
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, originalHex, string(directMarshallJson))

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
}
