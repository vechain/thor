// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
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

func TestRewardsBeforeAndAfterGalactica(t *testing.T) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 2

	thorChain, err := testchain.NewWithFork(&forkConfig)
	assert.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	trx1 := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(10).
		Expiration(720).
		Gas(21000).
		Nonce(0).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	trx2 := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(12).
		Expiration(720).
		Gas(21000).
		Nonce(0).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0], tx.MustSign(trx1, genesis.DevAccounts()[0].PrivateKey), tx.MustSign(trx2, genesis.DevAccounts()[0].PrivateKey)))
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	feesData := newFeesData(thorChain.Repo(), thorChain.Stater(), 10)

	bestBlockSummary := thorChain.Repo().BestBlockSummary()

	blockCount := 2
	oldestBlockID, baseFees, gasUsedRatios, rewards, err := feesData.resolveRange(bestBlockSummary, uint32(blockCount), []float64{25, 50, 75})
	assert.NoError(t, err)
	assert.NotNil(t, oldestBlockID)
	assert.Len(t, baseFees, blockCount)
	assert.Len(t, gasUsedRatios, blockCount)
	assert.Len(t, rewards, blockCount)

	// Before GALACTICA
	assert.NotNil(t, baseFees[0])
	assert.Equal(t, baseFees[0], (*hexutil.Big)(big.NewInt(0)))

	assert.NotNil(t, rewards[0])
	assert.Len(t, rewards[0], 3)
	for _, reward := range rewards[0] {
		assert.NotNil(t, reward)
		rewardValue := reward.ToInt()
		assert.Greater(t, rewardValue.Int64(), int64(0))
	}

	// After GALACTICA
	assert.NotNil(t, baseFees[1])
	assert.Equal(t, baseFees[1], (*hexutil.Big)(big.NewInt(thor.InitialBaseFee)))

	assert.NotNil(t, rewards[1])
	assert.Len(t, rewards[1], 3)
	for _, reward := range rewards[1] {
		assert.NotNil(t, reward)
		rewardValue := reward.ToInt()
		assert.Equal(t, int64(0), rewardValue.Int64())
	}
}
