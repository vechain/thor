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

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/debug"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/node"
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
	gasLimit   = 30_000_000
	logDBLimit = 1_000
)

func initAPIServer(t *testing.T) (*testchain.Chain, *httptest.Server) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	// mint some transactions to be used in the endpoints
	mintTransactions(t, thorChain)

	router := mux.NewRouter()

	accounts.New(thorChain.Repo(), thorChain.Stater(), uint64(gasLimit), thor.NoFork, thorChain.Engine()).
		Mount(router, "/accounts")

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
		}),
	)
	node.New(communicator).Mount(router, "/node")

	return thorChain, httptest.NewServer(router)
}

func mintTransactions(t *testing.T, thorChain *testchain.Chain) {
	toAddr := datagen.RandAddress()

	noClausesTx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(21000).
		Build()
	sig, err := crypto.Sign(noClausesTx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	noClausesTx = noClausesTx.WithSignature(sig)

	cla := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
	transaction := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err = crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	transaction = transaction.WithSignature(sig)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], transaction, noClausesTx))
}

func TestAPIs(t *testing.T) {
	thorChain, ts := initAPIServer(t)
	defer ts.Close()

	for name, tt := range map[string]func(*testing.T, *testchain.Chain, *httptest.Server){
		"testAccountEndpoint": testAccountEndpoint,
		"testBlocksEndpoint":  testBlocksEndpoint,
		"testDebugEndpoint":   testDebugEndpoint,
		"testEventsEndpoint":  testEventsEndpoint,
		"testNodeEndpoint":    testNodeEndpoint,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, thorChain, ts)
		})
	}
}

func testAccountEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	// Example storage key
	storageKey := "0x0000000000000000000000000000000000000000000000000000000000000000"

	// Example addresses
	address1 := "0x0123456789abcdef0123456789abcdef01234567"
	address2 := "0xabcdef0123456789abcdef0123456789abcdef01"

	// 1. Test GET /accounts/{address}
	t.Run("GetAccount", func(t *testing.T) {
		resp, err := ts.Client().Get(ts.URL + "/accounts/" + address1)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 2. Test GET /accounts/{address}/code
	t.Run("GetCode", func(t *testing.T) {
		resp, err := ts.Client().Get(ts.URL + "/accounts/" + address1 + "/code")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 3. Test GET /accounts/{address}/storage/{key}
	t.Run("GetStorage", func(t *testing.T) {
		resp, err := ts.Client().Get(ts.URL + "/accounts/" + address1 + "/storage/" + storageKey)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 4. Test POST /accounts/*
	t.Run("InspectClauses", func(t *testing.T) {
		// Define the payload for the batch call
		payload := `{
		"clauses": [
			{
				"to": "` + address1 + `",
				"value": "0x0",
				"data": "0x"
			},
			{
				"to": "` + address2 + `",
				"value": "0x1",
				"data": "0x"
			}
		],
		"gas": 1000000,
		"gasPrice": "0x0",
		"caller": "` + address1 + `"
	}`
		req, err := http.NewRequest("POST", ts.URL+"/accounts/*", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Simulate sending request with revision query parameter
		query := req.URL.Query()
		query.Add("revision", "best") // Add any revision parameter as expected
		req.URL.RawQuery = query.Encode()

		// Perform the request
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
	})
}

func testBlocksEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	// Example revision (this could be a block number or block ID)
	revision := "best" // You can adjust this to a real block number or ID for integration testing

	// 1. Test GET /blocks/{revision}
	t.Run("GetBlock", func(t *testing.T) {
		// Send request to get block information by revision
		resp, err := ts.Client().Get(ts.URL + "/blocks/" + revision)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 2. Test GET /blocks/{revision}?expanded=true
	t.Run("GetBlockExpanded", func(t *testing.T) {
		// Send request to get expanded block information (includes transactions and receipts)
		resp, err := ts.Client().Get(ts.URL + "/blocks/" + revision + "?expanded=true")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
	})

	// 3. Test GET /blocks/{revision}?expanded=invalid (should return bad request)
	t.Run("GetBlockInvalidExpanded", func(t *testing.T) {
		// Send request with an invalid 'expanded' parameter
		resp, err := ts.Client().Get(ts.URL + "/blocks/" + revision + "?expanded=invalid")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 400 Bad Request
		require.Equal(t, 400, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
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
	// Example address and topic for filtering events
	address := "0x0123456789abcdef0123456789abcdef01234567"
	topic := thor.BytesToBytes32([]byte("topic")).String()

	// 1. Test POST /events (Filter events)
	t.Run("FilterEvents", func(t *testing.T) {
		// Define the payload for filtering events
		payload := `{
			"criteriaSet": [
				{
					"address": "` + address + `",
					"topic0": "` + topic + `"
				}
			],
			"options": {
				"limit": 10,
				"offset": 0
			}
		}`

		req, err := http.NewRequest("POST", ts.URL+"/logs/event", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Perform the request
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
		// body, err := ioutil.ReadAll(resp.Body)
		// require.NoError(t, err)
		// fmt.Println(string(body))
	})
}

func testNodeEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	// 1. Test GET /node/network/peers
	t.Run("GetPeersStats", func(t *testing.T) {
		// Send request to get peers statistics
		resp, err := ts.Client().Get(ts.URL + "/node/network/peers")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Ensure the response code is 200 OK
		require.Equal(t, 200, resp.StatusCode)
		// Optionally, you can unmarshal and validate the response body here
		// body, err := ioutil.ReadAll(resp.Body)
		// require.NoError(t, err)
		// fmt.Println(string(body))
	})
}
