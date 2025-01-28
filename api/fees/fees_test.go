package fees_test

import (
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"strconv"
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

var (
	tclient *thorclient.Client
)

func TestFees(t *testing.T) {
	ts := initFeesServer(t)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T){
		"getFeeHistory": getFeeHistory,
		"getFeeHistoryWrongBlockCount": getFeeHistoryWrongBlockCount,
	} {
		t.Run(name, tt)
	}
}

func initFeesServer(t *testing.T) *httptest.Server {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 1
	thorChain, err := testchain.NewIntegrationTestChainWithFork(forkConfig)
	require.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	var dynFeeTx *tx.Transaction

	for i := 0; i < 9; i++ {
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
	require.Len(t, allBlocks, 10)

	router := mux.NewRouter()
	fees.New(thorChain.Repo(), thorChain.Engine()).
		Mount(router, "/fees")

	return httptest.NewServer(router)
}

func getFeeHistory(t *testing.T) {
	const blockCount = 3
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=" + strconv.Itoa(blockCount) + "&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 200, statusCode)
	require.NotNil(t, res)
	var feesHistory fees.GetFeesHistory
	if err := json.Unmarshal(res, &feesHistory); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, blockCount, len(feesHistory.BaseFees))
	expectedOldestBlock := uint32(7)
	expectedFeesHistory := fees.GetFeesHistory{
		OldestBlock:   &expectedOldestBlock,
		BaseFees:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(450413409)), (*hexutil.Big)(big.NewInt(345261708)), (*hexutil.Big)(big.NewInt(394348200))},
		GasUsedRatios: []float64{0.0021, 0.0021, 0.0021},
	}
	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func getFeeHistoryWrongBlockCount(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/fees/history?blockCount=wrong&newestBlock=best")
	require.NoError(t, err)
	require.Equal(t, 400, statusCode)
	require.NotNil(t, res)
}
