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

func TestWorkToGas_MonotonicDecay(t *testing.T) {
	work := new(big.Int).Mul(big.NewInt(1e12), big.NewInt(1e12)) // 1e24
	g1 := workToGas(work, 1)
	g2 := workToGas(work, 10_000_000)
	if g2 > g1 {
		t.Fatalf("expected gas to be non-increasing with larger blockNum: g2=%d g1=%d", g2, g1)
	}
}

func TestWorkToGas_SaturatesToMaxUint64(t *testing.T) {
	huge := new(big.Int).Lsh(big.NewInt(1), 400) // 2^400
	g := workToGas(huge, 0)
	if g != math.MaxUint64 {
		t.Fatalf("expected MaxUint64, got %d", g)
	}
}
