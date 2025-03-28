// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees_test

import (
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

const expectedGasPriceUsedRatio = 0.0042
const expectedBaseFee = thor.InitialBaseFee
const priorityFeesPercentage = 5

func TestFeesBacktraceGreaterThanFixedSize(t *testing.T) {
	ts, bestchain := initFeesServer(t, 8, 10, 10)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client, *chain.Chain){
		"getFeeHistoryBestBlock":               getFeeHistoryBestBlock,
		"getFeeHistoryWrongBlockCount":         getFeeHistoryWrongBlockCount,
		"getFeeHistoryWrongNewestBlock":        getFeeHistoryWrongNewestBlock,
		"getFeeHistoryNewestBlockNotIncluded":  getFeeHistoryNewestBlockNotIncluded,
		"getFeeHistoryCacheLimit":              getFeeHistoryCacheLimit,
		"getFeeHistoryBlockCountBiggerThanMax": getFeeHistoryBlockCountBiggerThanMax,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient, bestchain)
		})
	}
}

func TestFeesFixedSizeGreaterThanBacktrace(t *testing.T) {
	ts, bestchain := initFeesServer(t, 8, 6, 10)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client, *chain.Chain){
		"getFeeHistoryWithSummaries":          getFeeHistoryWithSummaries,
		"getFeeHistoryOnlySummaries":          getFeeHistoryOnlySummaries,
		"getFeeHistoryMoreThanBacktraceLimit": getFeeHistoryMoreThanBacktraceLimit,
		"getFeeHistoryNextBlock":              getFeeHistoryNextBlock,
		"getFeeHistoryOnlyNextBlock":          getFeeHistoryOnlyNextBlock,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient, bestchain)
		})
	}
}

func TestFeesFixedSizeSameAsBacktrace(t *testing.T) {
	// Less blocks than the backtrace limit
	ts, bestchain := initFeesServer(t, 11, 11, 10)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client, *chain.Chain){
		"getFeeHistoryBestBlock":                        getFeeHistoryBestBlock,
		"getFeeHistoryMoreBlocksRequestedThanAvailable": getFeeHistoryMoreBlocksRequestedThanAvailable,
		"getFeeHistoryBlock0":                           getFeeHistoryBlock0,
		"getFeeHistoryBlockCount0":                      getFeeHistoryBlockCount0,
		"getFeePriority":                                getFeePriority,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient, bestchain)
		})
	}
}

func TestRewardPercentiles(t *testing.T) {
	ts, bestchain := initFeesServer(t, 8, 10, 10)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client, *chain.Chain){
		"getRewardsValidPercentiles":      getRewardsValidPercentiles,
		"getRewardsInvalidPercentiles":    getRewardsInvalidPercentiles,
		"getRewardsOutOfRangePercentiles": getRewardsOutOfRangePercentiles,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient, bestchain)
		})
	}
}

func initFeesServer(t *testing.T, backtraceLimit int, fixedCacheSize int, numberOfBlocks int) (*httptest.Server, *chain.Chain) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 1
	thorChain, err := testchain.NewWithFork(forkConfig)
	require.NoError(t, err)

	router := mux.NewRouter()
	fees := fees.New(thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), fees.Config{
		APIBacktraceLimit:          backtraceLimit,
		FixedCacheSize:             fixedCacheSize,
		PriorityIncreasePercentage: priorityFeesPercentage,
	})
	fees.Mount(router, "/fees")

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	// Create blocks with transactions
	for i := range numberOfBlocks - 1 {
		// Create one transaction per block with different priority fees
		priorityFee1 := big.NewInt(10)
		trx1 := tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(250_000_000_000_000)).
			MaxPriorityFeePerGas(priorityFee1).
			Expiration(720).
			Gas(21000).
			Nonce(uint64(i)).
			Clause(cla).
			BlockRef(tx.NewBlockRef(uint32(i))).
			Build()
		priorityFee2 := big.NewInt(12)
		trx2 := tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(250_000_000_000_000)).
			MaxPriorityFeePerGas(priorityFee2).
			Expiration(720).
			Gas(21000).
			Nonce(uint64(i)).
			Clause(cla).
			BlockRef(tx.NewBlockRef(uint32(i))).
			Build()
		require.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0], tx.MustSign(trx1, genesis.DevAccounts()[0].PrivateKey), tx.MustSign(trx2, genesis.DevAccounts()[0].PrivateKey)))
	}

	return httptest.NewServer(router), thorChain.Repo().NewBestChain()
}

func getFeeHistoryWithSummaries(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=4")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(2)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock:   expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio:  []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
	}
	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryOnlySummaries(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=4&newestBlock=3")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(2)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock: expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
		},
		GasUsedRatio: []float64{
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
		},
	}

	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryBestBlock(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=4&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(6)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock:   expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio:  []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
	}

	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryWrongBlockCount(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=wrong&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "invalid blockCount, it should represent an integer: strconv.ParseUint: parsing \"wrong\": invalid syntax\n", string(res))
}

func getFeeHistoryWrongNewestBlock(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=wrong")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "newestBlock: strconv.ParseUint: parsing \"wrong\": invalid syntax\n", string(res))
}

func getFeeHistoryNewestBlockNotIncluded(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=20")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "newestBlock: not found\n", string(res))
}

func getFeeHistoryCacheLimit(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=4&newestBlock=2")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}

	// We expect this since:
	// - The cache and backtrace limit match (8)
	// - There are 10 blocks, from 0 to 9
	// So the oldest block is 2 since we cannot keep going backwards,
	// meaning that we cannot give the 4 requested blocks.
	expectedOldestBlock, err := bestchain.GetBlockID(2)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock:   expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio:  []float64{expectedGasPriceUsedRatio},
	}

	require.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryBlockCountBiggerThanMax(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=1025&newestBlock=1")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "invalid newestBlock, it is below the minimum allowed block\n", string(res))
}

func getFeeHistoryMoreBlocksRequestedThanAvailable(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=11&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(0)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock: expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{
			(*hexutil.Big)(big.NewInt(0)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio: []float64{
			0,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
		},
	}

	assert.Equal(t, expectedFeesHistory.OldestBlock, feesHistory.OldestBlock)
	assert.Equal(t, expectedFeesHistory.BaseFeePerGas[0].String(), feesHistory.BaseFeePerGas[0].String())
	assert.Equal(t, expectedFeesHistory.BaseFeePerGas[1:], feesHistory.BaseFeePerGas[1:])
	assert.Equal(t, expectedFeesHistory.GasUsedRatio, feesHistory.GasUsedRatio)
}

func getFeeHistoryBlock0(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=1&newestBlock=0")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(0)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock:   expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(0))},
		GasUsedRatio:  []float64{0},
	}

	assert.Equal(t, expectedFeesHistory.OldestBlock, feesHistory.OldestBlock)
	assert.Equal(t, expectedFeesHistory.BaseFeePerGas[0].String(), feesHistory.BaseFeePerGas[0].String())
	assert.Equal(t, expectedFeesHistory.GasUsedRatio, feesHistory.GasUsedRatio)
}

func getFeeHistoryMoreThanBacktraceLimit(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=5&newestBlock=4")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(2)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock: expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio: []float64{
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio,
			expectedGasPriceUsedRatio},
	}

	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryBlockCount0(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=0&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "invalid blockCount, it should not be 0\n", string(res))
}

func getFeeHistoryNextBlock(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=next")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock, err := bestchain.GetBlockID(8)
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock:   expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio:  []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
	}

	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryOnlyNextBlock(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=1&newestBlock=next")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.FeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock := thor.Bytes32{0x0, 0x0, 0x0, 0xa}
	require.NoError(t, err)
	expectedFeesHistory := fees.FeesHistory{
		OldestBlock:   expectedOldestBlock,
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatio:  []float64{expectedGasPriceUsedRatio},
	}

	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeePriority(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/priority")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesPriority fees.FeesPriority
	if err := json.Unmarshal(res, &feesPriority); err != nil {
		t.Fatal(err)
	}

	expectedFeesPriority := fees.FeesPriority{
		MaxPriorityFeePerGas: (*hexutil.Big)(new(big.Int).Div(new(big.Int).Mul(big.NewInt(thor.InitialBaseFee), big.NewInt(priorityFeesPercentage)), big.NewInt(100))),
	}

	assert.Equal(t, expectedFeesPriority, feesPriority)
}

func getRewardsValidPercentiles(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=4&rewardPercentiles=25,50,75")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)

	var feesHistory fees.FeesHistory
	require.NoError(t, json.Unmarshal(res, &feesHistory))

	require.NotNil(t, feesHistory.Reward, "reward array should not be nil")
	require.Len(t, feesHistory.Reward, 3, "should have rewards for 3 blocks")

	// Expected reward values based on MaxPriorityFeePerGas values
	expectedRewards := [][]*hexutil.Big{
		{
			(*hexutil.Big)(big.NewInt(10)), // 25th percentile
			(*hexutil.Big)(big.NewInt(10)), // 50th percentile
			(*hexutil.Big)(big.NewInt(12)), // 75th percentile
		},
		{
			(*hexutil.Big)(big.NewInt(10)), // 25th percentile
			(*hexutil.Big)(big.NewInt(10)), // 50th percentile
			(*hexutil.Big)(big.NewInt(12)), // 75th percentile
		},
		{
			(*hexutil.Big)(big.NewInt(10)), // 25th percentile
			(*hexutil.Big)(big.NewInt(10)), // 50th percentile
			(*hexutil.Big)(big.NewInt(12)), // 75th percentile
		},
	}

	for i, blockRewards := range feesHistory.Reward {
		require.NotNil(t, blockRewards, "block %d rewards should not be nil", i)
		require.Len(t, blockRewards, 3,
			"block %d should have 3 rewards, has %d",
			i, len(blockRewards))

		// Verify specific reward values
		for j, reward := range blockRewards {
			require.NotNil(t, reward, "block %d, reward %d should not be nil", i, j)
			require.NotNil(t, expectedRewards[i][j], "expected reward %d for block %d should not be nil", j, i)

			rewardValue := reward.ToInt()
			expectedValue := expectedRewards[i][j].ToInt()

			assert.Equal(t, expectedValue.String(), rewardValue.String(),
				"block %d, percentile %d should have reward %s, got %s",
				i, j, expectedValue.String(), rewardValue.String())
		}
	}
}

func getRewardsInvalidPercentiles(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	testCases := []string{
		"abc",
		"25,abc,75",
		"25,,75",
		",50,",
	}

	for _, percentiles := range testCases {
		_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=4&rewardPercentiles=" + percentiles)
		require.NoError(t, err)
		require.Equal(t, 400, statusCode, "should fail with invalid percentiles: %s", percentiles)
	}
}

func getRewardsOutOfRangePercentiles(t *testing.T, tclient *thorclient.Client, bestchain *chain.Chain) {
	testCases := []string{
		"-10,50,75",
		"25,101,75",
		"0,50,100.1",
	}

	for _, percentiles := range testCases {
		_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=4&rewardPercentiles=" + percentiles)
		require.NoError(t, err)
		require.Equal(t, 400, statusCode, "should fail with percentiles out of range: %s", percentiles)
	}
}
