// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
)

func TestCalculateRewards(t *testing.T) {
	tests := []struct {
		name              string
		cachedRewards     *rewards
		rewardPercentiles []float64
		expected          []*hexutil.Big
	}{
		{
			name:              "nil cached rewards",
			cachedRewards:     nil,
			rewardPercentiles: []float64{25, 50, 75},
			expected: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(0)),
				(*hexutil.Big)(big.NewInt(0)),
				(*hexutil.Big)(big.NewInt(0)),
			},
		},
		{
			name: "empty cached rewards",
			cachedRewards: &rewards{
				items:        []rewardItem{},
				totalGasUsed: 0,
			},
			rewardPercentiles: []float64{25, 50, 75},
			expected: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(0)),
				(*hexutil.Big)(big.NewInt(0)),
				(*hexutil.Big)(big.NewInt(0)),
			},
		},
		{
			name: "single transaction",
			cachedRewards: &rewards{
				items: []rewardItem{
					{reward: big.NewInt(100), gasUsed: 1000},
				},
				totalGasUsed: 1000,
			},
			rewardPercentiles: []float64{25, 50, 75},
			expected: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(100)),
				(*hexutil.Big)(big.NewInt(100)),
				(*hexutil.Big)(big.NewInt(100)),
			},
		},
		{
			name: "multiple transactions with different gas usage",
			cachedRewards: &rewards{
				items: []rewardItem{
					{reward: big.NewInt(100), gasUsed: 1000},
					{reward: big.NewInt(200), gasUsed: 2000},
					{reward: big.NewInt(300), gasUsed: 3000},
				},
				totalGasUsed: 6000,
			},
			rewardPercentiles: []float64{25, 50, 75},
			expected: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(200)), // 25% threshold at 1500 gas
				(*hexutil.Big)(big.NewInt(200)), // 50% threshold at 3000 gas
				(*hexutil.Big)(big.NewInt(300)), // 75% threshold at 4500 gas
			},
		},
		{
			name: "multiple transactions with equal gas usage",
			cachedRewards: &rewards{
				items: []rewardItem{
					{reward: big.NewInt(100), gasUsed: 1000},
					{reward: big.NewInt(200), gasUsed: 1000},
					{reward: big.NewInt(300), gasUsed: 1000},
				},
				totalGasUsed: 3000,
			},
			rewardPercentiles: []float64{25, 50, 75},
			expected: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(100)), // 25% threshold at 750 gas
				(*hexutil.Big)(big.NewInt(200)), // 50% threshold at 1500 gas
				(*hexutil.Big)(big.NewInt(300)), // 75% threshold at 2250 gas
			},
		},
		{
			name: "empty percentiles",
			cachedRewards: &rewards{
				items: []rewardItem{
					{reward: big.NewInt(100), gasUsed: 1000},
				},
				totalGasUsed: 1000,
			},
			rewardPercentiles: []float64{},
			expected:          []*hexutil.Big{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := &FeesData{}
			result := fd.calculateRewards(tt.cachedRewards, tt.rewardPercentiles)
			assert.Equal(t, tt.expected, result)
		})
	}
}
