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

const expectedGasPriceUsedRatio = 0.0021
const expectedBaseFee = thor.InitialBaseFee

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

func initFeesServer(t *testing.T, backtraceLimit uint32, fixedCacheSize uint32, numberOfBlocks int) (*httptest.Server, *chain.Chain) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 1
	thorChain, err := testchain.NewWithFork(forkConfig)
	require.NoError(t, err)

	router := mux.NewRouter()
	fees := fees.New(thorChain.Repo(), thorChain.Engine(), fees.Config{
		APIBacktraceLimit:        backtraceLimit,
		PriorityBacktraceLimit:   20,
		PrioritySampleTxPerBlock: 3,
		PriorityPercentile:       60,
		FixedCacheSize:           fixedCacheSize,
	})
	fees.Mount(router, "/fees")

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	var dynFeeTx *tx.Transaction

	for i := 0; i < numberOfBlocks-1; i++ {
		dynFeeTx = tx.NewTxBuilder(tx.TypeDynamicFee).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(250_000_000_000_000)).
			MaxPriorityFeePerGas(big.NewInt(100)).
			Expiration(10).
			Gas(21000).
			Nonce(uint64(i)).
			Clause(cla).
			BlockRef(tx.NewBlockRef(uint32(i))).
			MustBuild()
		dynFeeTx = tx.MustSign(dynFeeTx, genesis.DevAccounts()[0].PrivateKey)
		require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], dynFeeTx))
	}

	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	require.Len(t, allBlocks, numberOfBlocks)

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
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatios: []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
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
		BaseFees: []*hexutil.Big{
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
		},
		GasUsedRatios: []float64{
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
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee)), (*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatios: []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
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
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatios: []float64{expectedGasPriceUsedRatio},
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
		BaseFees: []*hexutil.Big{
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
		GasUsedRatios: []float64{
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
	assert.Equal(t, expectedFeesHistory.BaseFees[0].String(), feesHistory.BaseFees[0].String())
	assert.Equal(t, expectedFeesHistory.BaseFees[1:], feesHistory.BaseFees[1:])
	assert.Equal(t, expectedFeesHistory.GasUsedRatios, feesHistory.GasUsedRatios)
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
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(0))},
		GasUsedRatios: []float64{0},
	}

	assert.Equal(t, expectedFeesHistory.OldestBlock, feesHistory.OldestBlock)
	assert.Equal(t, expectedFeesHistory.BaseFees[0].String(), feesHistory.BaseFees[0].String())
	assert.Equal(t, expectedFeesHistory.GasUsedRatios, feesHistory.GasUsedRatios)
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
		BaseFees: []*hexutil.Big{
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee)),
			(*hexutil.Big)(big.NewInt(expectedBaseFee))},
		GasUsedRatios: []float64{
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
		MaxPriorityFeePerGas: (*hexutil.Big)(big.NewInt(100)),
	}

	assert.Equal(t, expectedFeesPriority, feesPriority)
}
