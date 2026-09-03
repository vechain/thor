// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkToGas(t *testing.T) {
	// Test cases
	testCases := []struct {
		name     string
		work     *big.Int
		blockNum uint32
		expected uint64
	}{
		{
			name:     "Basic conversion",
			work:     big.NewInt(10000),
			blockNum: 100,
			expected: 10,
		},
		{
			name:     "Zero work",
			work:     big.NewInt(0),
			blockNum: 100,
			expected: 0,
		},
		{
			name:     "Large work value",
			work:     big.NewInt(math.MaxInt64),
			blockNum: 100,
			expected: 0x20c49ba5e353f7,
		},
		{
			name:     "Large work value and blockNum",
			work:     big.NewInt(math.MaxInt64),
			blockNum: 12345679,
			expected: 0x52fc533083329,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := workToGas(tc.work, tc.blockNum)
			assert.Equal(t, tc.expected, result, "Expected and actual gas should match")
		})
	}
}
