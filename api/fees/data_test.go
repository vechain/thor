// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
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

	thorChain, err := testchain.NewWithFork(&forkConfig, 180)
	assert.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	trx1 := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(100).
		Expiration(720).
		Gas(21000).
		Nonce(datagen.RandUint64()).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	trx2 := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(0).
		Expiration(720).
		Gas(21000).
		Nonce(datagen.RandUint64()).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	assert.NoError(t,
		thorChain.MintBlock(
			tx.MustSign(trx1, genesis.DevAccounts()[0].PrivateKey),
			tx.MustSign(trx2, genesis.DevAccounts()[0].PrivateKey),
		),
	)

	trx3 := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(10).
		Expiration(720).
		Gas(21000).
		Nonce(datagen.RandUint64()).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	assert.NoError(t, thorChain.MintBlock(tx.MustSign(trx3, genesis.DevAccounts()[0].PrivateKey)))

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
		assert.True(t, big.NewInt(0).Cmp((*big.Int)(reward)) == 0)
	}

	// After GALACTICA
	assert.NotNil(t, baseFees[1])
	assert.Equal(t, baseFees[1], (*hexutil.Big)(big.NewInt(thor.InitialBaseFee)))

	assert.NotNil(t, rewards[1])
	assert.Len(t, rewards[1], 3)

	expectedReward, ok := new(big.Int).SetString("1029215686274509", 10)
	require.True(t, ok, "failed to parse expected reward")

	for _, reward := range rewards[1] {
		assert.NotNil(t, reward)
		assert.True(t, expectedReward.Cmp((*big.Int)(reward)) == 0)
	}
}

func TestRewardsComputedAfterWarmupWithoutPercentiles(t *testing.T) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 1

	thorChain, err := testchain.NewWithFork(&forkConfig, 180)
	require.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	// Create several post-Galactica blocks with dynamic fee transactions
	const numberOfBlocks = 5
	for i := range numberOfBlocks {
		trx1 := tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(250_000_000_000_000)).
			MaxPriorityFeePerGas(big.NewInt(10)).
			Expiration(720).
			Gas(21000).
			Nonce(uint64(i * 2)).
			Clause(cla).
			BlockRef(tx.NewBlockRef(uint32(i))).
			Build()
		trx2 := tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(250_000_000_000_000)).
			MaxPriorityFeePerGas(big.NewInt(12)).
			Expiration(720).
			Gas(21000).
			Nonce(uint64(i*2 + 1)).
			Clause(cla).
			BlockRef(tx.NewBlockRef(uint32(i))).
			Build()
		require.NoError(
			t,
			thorChain.MintBlock(
				tx.MustSign(trx1, genesis.DevAccounts()[0].PrivateKey),
				tx.MustSign(trx2, genesis.DevAccounts()[0].PrivateKey),
			),
		)
	}

	feesData := newFeesData(thorChain.Repo(), thorChain.Stater(), 10)
	newestBlockSummary := thorChain.Repo().BestBlockSummary()

	// Warm the cache without reward percentiles: entries are cached, rewards were computed at miss
	_, _, _, _, err = feesData.resolveRange(newestBlockSummary, 3, nil)
	require.NoError(t, err)

	// Now request with percentiles
	_, _, _, rewards, err := feesData.resolveRange(newestBlockSummary, 3, []float64{25, 50, 75})
	require.NoError(t, err)

	require.NotNil(t, rewards)
	require.Len(t, rewards, 3)

	expected := []*hexutil.Big{
		(*hexutil.Big)(big.NewInt(10)), // 25th percentile
		(*hexutil.Big)(big.NewInt(10)), // 50th percentile
		(*hexutil.Big)(big.NewInt(12)), // 75th percentile
	}

	for i := range rewards {
		require.NotNil(t, rewards[i])
		require.Len(t, rewards[i], 3)
		for j := range rewards[i] {
			assert.Equal(t, expected[j].String(), rewards[i][j].String())
		}
	}
}
