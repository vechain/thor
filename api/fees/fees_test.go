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
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

const expectedGasPriceUsedRatio = 0.0021

func TestFeesBacktraceGreaterThanFixedSize(t *testing.T) {
	ts, closeFunc := initFeesServer(t, 8, 10, 10)
	t.Cleanup(func() {
		closeFunc()
		ts.Close()
	})

	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client){
		"getFeeHistoryBestBlock":               getFeeHistoryBestBlock,
		"getFeeHistoryWrongBlockCount":         getFeeHistoryWrongBlockCount,
		"getFeeHistoryWrongNewestBlock":        getFeeHistoryWrongNewestBlock,
		"getFeeHistoryNewestBlockNotIncluded":  getFeeHistoryNewestBlockNotIncluded,
		"getFeeHistoryCacheLimit":              getFeeHistoryCacheLimit,
		"getFeeHistoryBlockCountBiggerThanMax": getFeeHistoryBlockCountBiggerThanMax,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient)
		})
	}
}

func TestFeesFixedSizeGreaterThanBacktrace(t *testing.T) {
	ts, closeFunc := initFeesServer(t, 8, 6, 10)
	defer func() {
		closeFunc()
		ts.Close()
	}()
	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client){
		"getFeeHistoryWithSummaries": getFeeHistoryWithSummaries,
		"getFeeHistoryOnlySummaries": getFeeHistoryOnlySummaries,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient)
		})
	}
}

func TestFeesFixedSizeSameAsBacktrace(t *testing.T) {
	// Less blocks than the backtrace limit
	ts, closeFunc := initFeesServer(t, 11, 11, 10)
	defer func() {
		closeFunc()
		ts.Close()
	}()
	tclient := thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T, *thorclient.Client){
		"getFeeHistoryBestBlock":                        getFeeHistoryBestBlock,
		"getFeeHistoryMoreBlocksRequestedThanAvailable": getFeeHistoryMoreBlocksRequestedThanAvailable,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, tclient)
		})
	}
}

func initFeesServer(t *testing.T, backtraceLimit uint32, fixedCacheSize uint32, numberOfBlocks int) (*httptest.Server, func()) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 1
	thorChain, err := testchain.NewIntegrationTestChainWithFork(forkConfig)
	require.NoError(t, err)

	router := mux.NewRouter()
	fees := fees.New(thorChain.Repo(), thorChain.Engine(), backtraceLimit, fixedCacheSize)
	fees.Mount(router, "/fees")

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	var dynFeeTx *tx.Transaction

	for i := 0; i < numberOfBlocks-1; i++ {
		dynFeeTx = tx.NewTxBuilder(tx.DynamicFeeTxType).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(100000)).
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

	return httptest.NewServer(router), fees.Close
}

func getFeeHistoryWithSummaries(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=4")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.GetFeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock := uint32(2)
	expectedFeesHistory := fees.GetFeesHistory{
		OldestBlock:   &expectedOldestBlock,
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(875525000)), (*hexutil.Big)(big.NewInt(766544026)), (*hexutil.Big)(big.NewInt(671128459))},
		GasUsedRatios: []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
	}
	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryOnlySummaries(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=4&newestBlock=3")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.Equal(t, "newestBlock must be between 4 and 9\n", string(res))
}

func getFeeHistoryBestBlock(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=4&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.GetFeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock := uint32(6)
	expectedFeesHistory := fees.GetFeesHistory{
		OldestBlock:   &expectedOldestBlock,
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(514449512)), (*hexutil.Big)(big.NewInt(450413409)), (*hexutil.Big)(big.NewInt(394348200)), (*hexutil.Big)(big.NewInt(345261708))},
		GasUsedRatios: []float64{expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio, expectedGasPriceUsedRatio},
	}

	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryWrongBlockCount(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=wrong&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "invalid blockCount, it should represent an integer: strconv.ParseUint: parsing \"wrong\": invalid syntax\n", string(res))
}

func getFeeHistoryWrongNewestBlock(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=wrong")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "newestBlock: strconv.ParseUint: parsing \"wrong\": invalid syntax\n", string(res))
}

func getFeeHistoryNewestBlockNotIncluded(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=3&newestBlock=20")
	require.NoError(t, err)
	require.Equal(t, 404, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "newestBlock: not found\n", string(res))
}

func getFeeHistoryCacheLimit(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=4&newestBlock=2")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.GetFeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}

	// We expect this since:
	// - The cache and backtrace limit match (8)
	// - There are 10 blocks, from 0 to 9
	// So the oldest block is 2 since we cannot keep going backwards,
	// meaning that we cannot give the 4 requested blocks.
	expectedOldestBlock := uint32(2)
	expectedFeesHistory := fees.GetFeesHistory{
		OldestBlock:   &expectedOldestBlock,
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(875525000))},
		GasUsedRatios: []float64{expectedGasPriceUsedRatio},
	}

	require.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryBlockCountBiggerThanMax(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=1025&newestBlock=1")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
	assert.Equal(t, "blockCount must be between 1 and 8\n", string(res))
}

func getFeeHistoryMoreBlocksRequestedThanAvailable(t *testing.T, tclient *thorclient.Client) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=11&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.GetFeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	expectedOldestBlock := uint32(1)
	expectedFeesHistory := fees.GetFeesHistory{
		OldestBlock: &expectedOldestBlock,
		BaseFees: []*hexutil.Big{
			(*hexutil.Big)(big.NewInt(1000000000)),
			(*hexutil.Big)(big.NewInt(875525000)),
			(*hexutil.Big)(big.NewInt(766544026)),
			(*hexutil.Big)(big.NewInt(671128459)),
			(*hexutil.Big)(big.NewInt(587589745)),
			(*hexutil.Big)(big.NewInt(514449512)),
			(*hexutil.Big)(big.NewInt(450413409)),
			(*hexutil.Big)(big.NewInt(394348200)),
			(*hexutil.Big)(big.NewInt(345261708))},
		GasUsedRatios: []float64{
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

	assert.Equal(t, expectedFeesHistory, feesHistory)
}
