// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestClauseTo(t *testing.T) {
	var toAddress thor.Address
	copy(toAddress[:], []byte{0xde, 0xad, 0xbe, 0xef})

	clause := &Clause{
		body: clauseBody{
			To: &toAddress,
		},
	}

	result := clause.To()

	// The result should not be nil and should match the mock address
	assert.NotNil(t, result)
	assert.Equal(t, toAddress, *result)

	// Test the case where 'To' is nil
	clause.body.To = nil
	result = clause.To()

	// The result should be nil
	assert.Nil(t, result)
}

func TestClauseValue(t *testing.T) {
	expectedValue := big.NewInt(100) // Mock value

	clause := &Clause{
		body: clauseBody{
			Value: expectedValue,
		},
	}

	result := clause.Value()

	// The result should not be nil and should match the mock value
	assert.NotNil(t, result)
	assert.Equal(t, 0, expectedValue.Cmp(result))
}

func TestClauseData(t *testing.T) {
	expectedData := []byte{0x01, 0x02, 0x03} // Mock data

	clause := &Clause{
		body: clauseBody{
			Data: expectedData,
		},
	}

	result := clause.Data()

	// The result should not be nil and should match the mock data
	assert.NotNil(t, result)
	assert.True(t, reflect.DeepEqual(expectedData, result))
}
