// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thorclient

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/debug"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"

	// Force-load the tracer native engines to trigger registration
	_ "github.com/vechain/thor/v2/tracers/js"
	_ "github.com/vechain/thor/v2/tracers/logger"
)

const (
	gasLimit               = 30_000_000
	logDBLimit             = 1_000
	priorityFeesPercentile = 5
)

var (
	preMintedTx01 *tx.Transaction
)

func initAPIServer(t *testing.T) (*testchain.Chain, *httptest.Server) {
	forks := testchain.DefaultForkConfig
	forks.GALACTICA = 1
	thorChain, err := testchain.NewWithFork(forks)
	require.NoError(t, err)

	// mint some transactions to be used in the endpoints
	mintTransactions(t, thorChain)

	router := mux.NewRouter()

	accounts.New(thorChain.Repo(), thorChain.Stater(), uint64(gasLimit), thor.NoFork, thorChain.Engine(), true).
		Mount(router, "/accounts")

	mempool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute}, &forks)
	transactions.New(thorChain.Repo(), mempool).Mount(router, "/transactions")

	blocks.New(thorChain.Repo(), thorChain.Engine()).Mount(router, "/blocks")

	debug.New(thorChain.Repo(), thorChain.Stater(), thorChain.GetForkConfig(), gasLimit, true, thorChain.Engine(), []string{"all"}, false).
		Mount(router, "/debug")

	logDb, err := logdb.NewMem()
	require.NoError(t, err)
	events.New(thorChain.Repo(), logDb, logDBLimit).Mount(router, "/logs/event")

	communicator := comm.New(
		thorChain.Repo(),
		txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
			Limit:           10000,
			LimitPerAccount: 16,
			MaxLifetime:     10 * time.Minute,
		}, &thor.NoFork),
	)
	node.New(communicator).Mount(router, "/node")

	fees.New(thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), fees.Config{
		APIBacktraceLimit:      6,
		FixedCacheSize:         6,
		PriorityFeesPercentile: priorityFeesPercentile,
	}).Mount(router, "/fees")

	return thorChain, httptest.NewServer(router)
}

func mintTransactions(t *testing.T, thorChain *testchain.Chain) {
	toAddr := datagen.RandAddress()

	noClausesTx := tx.NewTxBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(21000).
		MustBuild()
	sig, err := crypto.Sign(noClausesTx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	noClausesTx = noClausesTx.WithSignature(sig)

	cla := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
	transaction := tx.NewTxBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(tx.NewBlockRef(0)).
		MustBuild()

	sig, err = crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	transaction = transaction.WithSignature(sig)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], transaction, noClausesTx))
	preMintedTx01 = transaction
}

func TestAPIs(t *testing.T) {
	thorChain, ts := initAPIServer(t)
	defer ts.Close()

	for name, tt := range map[string]func(*testing.T, *testchain.Chain, *httptest.Server){
		"testAccountEndpoint":      testAccountEndpoint,
		"testTransactionsEndpoint": testTransactionsEndpoint,
		"testBlocksEndpoint":       testBlocksEndpoint,
		"testDebugEndpoint":        testDebugEndpoint,
		"testEventsEndpoint":       testEventsEndpoint,
		"testNodeEndpoint":         testNodeEndpoint,
		"testFeesEndpoint":         testFeesEndpoint,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, thorChain, ts)
		})
	}
}

func testAccountEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	// Example storage key
	storageKey := thor.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000000")

	// Example addresses
	address1 := thor.MustParseAddress("0x0123456789abcdef0123456789abcdef01234567")
	address2 := thor.MustParseAddress("0xabcdef0123456789abcdef0123456789abcdef01")

	// 1. Test GET /accounts/{address}
	t.Run("GetAccount", func(t *testing.T) {
		c := New(ts.URL)
		_, err := c.Account(&address1)
		require.NoError(t, err)
		// TODO validate the response body here
	})

	// 2. Test GET /accounts/{address}/code
	t.Run("GetCode", func(t *testing.T) {
		c := New(ts.URL)
		_, err := c.AccountCode(&address1)
		require.NoError(t, err)
		// TODO validate the response body here
	})

	// 3. Test GET /accounts/{address}/storage/{key}
	t.Run("GetStorage", func(t *testing.T) {
		c := New(ts.URL)
		_, err := c.AccountStorage(&address1, &storageKey)
		require.NoError(t, err)
		// TODO validate the response body here
	})

	// 4. Test POST /accounts/*
	t.Run("InspectClauses", func(t *testing.T) {
		c := New(ts.URL)
		// Define the payload for the batch call
		value := math.HexOrDecimal256(*big.NewInt(1))
		payload := &accounts.BatchCallData{
			Clauses: accounts.Clauses{
				accounts.Clause{
					To:    &address1,
					Value: nil,
					Data:  "0x",
				},
				accounts.Clause{
					To:    &address2,
					Value: &value,
					Data:  "0x",
				},
			},
			Gas:      1000000,
			GasPrice: &value,
			Caller:   &address1,
		}
		_, err := c.InspectClauses(payload)
		require.NoError(t, err)

		// Simulate sending request with revision query parameter
		_, err = c.InspectClauses(payload, Revision("best"))
		require.NoError(t, err)
	})
}

func testTransactionsEndpoint(t *testing.T, thorChain *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)

	// 1. Test retrieving a pre-mined transaction by ID
	t.Run("GetTransaction", func(t *testing.T) {
		id := preMintedTx01.ID()
		trx, err := c.Transaction(&id)
		require.NoError(t, err)
		require.NotNil(t, trx)
		require.Equal(t, id.String(), trx.ID.String())
	})

	// 2. Test sending a new transaction
	t.Run("SendTransaction", func(t *testing.T) {
		toAddr := thor.MustParseAddress("0x0123456789abcdef0123456789abcdef01234567")
		clause := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
		trx := tx.NewTxBuilder(tx.TypeLegacy).
			ChainTag(thorChain.Repo().ChainTag()).
			Expiration(10).
			Gas(21000).
			Clause(clause).
			MustBuild()

		trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
		sendResult, err := c.SendTransaction(trx)
		require.NoError(t, err)
		require.NotNil(t, sendResult)
		require.Equal(t, trx.ID().String(), sendResult.ID.String()) // Ensure transaction was successful

		trx = tx.NewTxBuilder(tx.TypeLegacy).
			ChainTag(thorChain.Repo().ChainTag()).
			Expiration(10).
			Gas(21000).
			Clause(clause).
			MustBuild()

		trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
		sendResult, err = c.SendTransaction(trx)
		require.NoError(t, err)
		require.NotNil(t, sendResult)
		require.Equal(t, trx.ID().String(), sendResult.ID.String()) // Ensure transaction was successful
	})

	// 3. Test retrieving the transaction receipt
	t.Run("GetTransactionReceipt", func(t *testing.T) {
		txID := preMintedTx01.ID()
		receipt, err := c.TransactionReceipt(&txID)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.Equal(t, txID.String(), receipt.Meta.TxID.String())
	})

	// 4. Test inspecting clauses of a transaction
	t.Run("InspectClauses", func(t *testing.T) {
		clause := tx.NewClause(nil).WithValue(big.NewInt(10000)).WithData([]byte("0x"))
		batchCallData := convertToBatchCallData(preMintedTx01, nil)
		batchCallData.Clauses = append(batchCallData.Clauses, convertClauseAccounts(clause))

		callResults, err := c.InspectClauses(batchCallData)
		require.NoError(t, err)
		require.NotNil(t, callResults)
		require.Greater(t, len(callResults), 0)
	})
}

func testBlocksEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)
	// Example revision (this could be a block number or block ID)
	revision := "best"

	// 1. Test GET /blocks/{revision}
	t.Run("GetBlock", func(t *testing.T) {
		_, err := c.Block(revision)
		require.NoError(t, err)
		// TODO validate the response body here
	})

	// 2. Test GET /blocks/{revision}?expanded=true
	t.Run("GetBlockExpanded", func(t *testing.T) {
		_, err := c.ExpandedBlock(revision)
		require.NoError(t, err)
		// TODO validate the response body here
	})
}

func testDebugEndpoint(t *testing.T, thorChain *testchain.Chain, ts *httptest.Server) {
	// Example block ID, transaction index, and clause index
	bestBlock, _ := thorChain.BestBlock()
	blockID := bestBlock.Header().ID().String()
	txIndex := uint64(0)
	clauseIndex := uint32(0)

	// Example contract address
	contractAddress := "0xabcdef0123456789abcdef0123456789abcdef01"

	// 1. Test POST /debug/tracers (Trace an existing clause)
	t.Run("TraceClause", func(t *testing.T) {
		payload := `{
			"name": "structLoggerTracer",
			"target": "` + blockID + `/` + strconv.FormatUint(txIndex, 10) + `/` + strconv.FormatUint(uint64(clauseIndex), 10) + `"
		}`

		req, err := http.NewRequest("POST", ts.URL+"/debug/tracers", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Perform the request
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 2. Test POST /debug/tracers/call (Trace a contract call)
	t.Run("TraceCall", func(t *testing.T) {
		payload := `{
			"name": "structLoggerTracer",
			"to": "` + contractAddress + `",
			"value": "0x0",
			"data": "0x",
			"gas": 1000000,
			"gasPrice": "0x0",
			"caller": "` + contractAddress + `"
		}`

		req, err := http.NewRequest("POST", ts.URL+"/debug/tracers/call", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Perform the request
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 3. Test POST /debug/storage-range (Debug storage for a contract)
	t.Run("DebugStorage", func(t *testing.T) {
		payload := `{
			"address": "` + contractAddress + `",
			"target": "` + blockID + `/` + strconv.FormatUint(txIndex, 10) + `/` + strconv.FormatUint(uint64(clauseIndex), 10) + `",
			"keyStart": "",
			"maxResult": 100
		}`

		req, err := http.NewRequest("POST", ts.URL+"/debug/storage-range", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Perform the request
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})
}

func testEventsEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)

	// Example address and topic for filtering events
	address := thor.MustParseAddress("0x0123456789abcdef0123456789abcdef01234567")
	topic := thor.BytesToBytes32([]byte("topic"))

	// 1. Test POST /events (Filter events)
	t.Run("FilterEvents", func(t *testing.T) {
		// Define the payload for filtering events
		payload := &events.EventFilter{
			CriteriaSet: []*events.EventCriteria{
				{
					Address: &address,
					TopicSet: events.TopicSet{
						Topic0: &topic,
					},
				},
			},
			Range: nil,
			Options: &events.Options{
				Offset: 0,
				Limit:  10,
			},
			Order: "",
		}

		_, err := c.FilterEvents(payload)
		require.NoError(t, err)

		//TODO validate the response body here
	})
}

func testNodeEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)
	// 1. Test GET /node/network/peers
	t.Run("GetPeersStats", func(t *testing.T) {
		_, err := c.Peers()
		require.NoError(t, err)
	})
}

func testFeesEndpoint(t *testing.T, testchain *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)
	// 1. Test GET /fees/history
	t.Run("GetFeesHistory", func(t *testing.T) {
		blockCount := uint32(1)
		newestBlock := "best"
		feesHistory, err := c.FeesHistory(blockCount, newestBlock)
		require.NoError(t, err)
		require.NotNil(t, feesHistory)

		expectedOldestBlock, err := testchain.Repo().NewBestChain().GetBlockID(1)
		require.NoError(t, err)
		expectedFeesHistory := &fees.FeesHistory{
			OldestBlock: expectedOldestBlock,
			BaseFees: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(thor.InitialBaseFee)),
			},
			GasUsedRatios: []float64{
				0.0058,
			},
		}

		require.Equal(t, expectedFeesHistory, feesHistory)
	})

	// 2. Test GET /fees/priority
	t.Run("GetFeesPriority", func(t *testing.T) {
		feesPriority, err := c.FeesPriority()
		require.NoError(t, err)
		require.NotNil(t, feesPriority)

		expectedFeesPriority := &fees.FeesPriority{
			MaxPriorityFeePerGas: (*hexutil.Big)(new(big.Int).Div(new(big.Int).Mul(big.NewInt(thor.InitialBaseFee), big.NewInt(priorityFeesPercentile)), big.NewInt(100))),
		}

		require.Equal(t, expectedFeesPriority, feesPriority)
	})
}
