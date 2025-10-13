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
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/test/testnode"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"

	// Force-load the tracer native engines to trigger registration
	_ "github.com/vechain/thor/v2/tracers/js"
	_ "github.com/vechain/thor/v2/tracers/logger"
)

var preMintedTx01 *tx.Transaction

func initTestNode(t *testing.T) testnode.Node {
	forks := testchain.DefaultForkConfig
	forks.GALACTICA = 1
	thorChain, err := testchain.NewWithFork(&forks, 180)
	require.NoError(t, err)

	testNode, err := testnode.NewNodeBuilder().WithChain(thorChain).Build()
	require.NoError(t, err)
	require.NoError(t, testNode.Start())

	// mint some transactions to be used in the endpoints
	mintTransactions(t, thorChain)
	// add the transactions to the mempool
	addTransactionToPool(t, testNode)

	return testNode
}

func mintTransactions(t *testing.T, thorChain *testchain.Chain) {
	toAddr := datagen.RandAddress()

	noClausesTx := tx.NewBuilder(tx.TypeLegacy).
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
	legacyTx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	legacyTx = tx.MustSign(legacyTx, genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], legacyTx, noClausesTx))
	preMintedTx01 = legacyTx

	dynFeeTx := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(thorChain.Repo().ChainTag()).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(new(big.Int).Add(big.NewInt(thor.InitialBaseFee), big.NewInt(thor.InitialBaseFee))).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	dynFeeTx = tx.MustSign(dynFeeTx, genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], dynFeeTx))
}

func addTransactionToPool(t *testing.T, testNode testnode.Node) {
	toAddr := datagen.RandAddress()
	chainTag := testNode.Chain().ChainTag()

	cla := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
	testTx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(chainTag).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	testTx = tx.MustSign(testTx, genesis.DevAccounts()[0].PrivateKey)

	// Add transaction to the pool
	c := New(testNode.APIServer().URL)
	_, err := c.SendTransaction(testTx)
	require.NoError(t, err)
}

func TestAPIs(t *testing.T) {
	testNode := initTestNode(t)
	defer func() {
		require.NoError(t, testNode.Stop())
	}()

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
			tt(t, testNode.Chain(), testNode.APIServer())
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
		payload := &api.BatchCallData{
			Clauses: api.Clauses{
				&api.Clause{
					To:    &address1,
					Value: nil,
					Data:  "0x",
				},
				&api.Clause{
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
		trx := tx.NewBuilder(tx.TypeLegacy).
			ChainTag(thorChain.Repo().ChainTag()).
			Expiration(10).
			Gas(21000).
			Clause(clause).
			Build()

		trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
		sendResult, err := c.SendTransaction(trx)
		require.NoError(t, err)
		require.NotNil(t, sendResult)
		require.Equal(t, trx.ID().String(), sendResult.ID.String()) // Ensure transaction was successful

		trx = tx.NewBuilder(tx.TypeLegacy).
			ChainTag(thorChain.Repo().ChainTag()).
			Expiration(10).
			Gas(21000).
			Clause(clause).
			Nonce(datagen.RandUint64()).
			Build()

		trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
		sendResult, err = c.SendTransaction(trx)
		require.NoError(t, err)
		require.NotNil(t, sendResult)
		require.Equal(t, trx.ID().String(), sendResult.ID.String()) // Ensure transaction was successful

		txID := trx.ID()

		require.NoError(t, test.Retry(func() error {
			receipt, err := c.TransactionReceipt(&txID)
			if err != nil {
				return err
			}
			require.NoError(t, err)
			require.NotNil(t, receipt)
			require.Equal(t, txID.String(), receipt.Meta.TxID.String())
			return nil
		}, time.Second, 10*time.Second))
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
		limit := uint64(10)
		payload := &api.EventFilter{
			CriteriaSet: []*api.EventCriteria{
				{
					Address: &address,
					TopicSet: api.TopicSet{
						Topic0: &topic,
					},
				},
			},
			Range: nil,
			Options: &api.Options{
				Offset: 0,
				Limit:  &limit,
			},
			Order: "",
		}

		_, err := c.FilterEvents(payload)
		require.NoError(t, err)

		// TODO validate the response body here
	})
}

func testNodeEndpoint(t *testing.T, _ *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)
	// 1. Test GET /node/network/peers
	t.Run("GetPeersStats", func(t *testing.T) {
		_, err := c.Peers()
		require.NoError(t, err)
	})

	// 2. Test GET /node/txpool
	t.Run("GetTxPool", func(t *testing.T) {
		// Test with transaction IDs only
		result, err := c.PoolTransactionIDs(nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.GreaterOrEqual(t, len(result), 1, "Expected at least one transaction in pool")

		// Test with origin filter
		origin := genesis.DevAccounts()[0].Address
		result, err = c.PoolTransactionIDs(&origin)
		require.NoError(t, err)
		require.NotNil(t, result)

		require.GreaterOrEqual(t, len(result), 1, "Expected non-negative length")

		// Origin does not exist
		origin = thor.MustParseAddress("0x0123456789abcdef0123456789abcdef01234567")
		result, err = c.PoolTransactionIDs(&origin)
		require.NoError(t, err)
		require.NotNil(t, result)

		require.Equal(t, len(result), 0, "No tx expected")
	})

	// 3. Test GET /node/txpool?expanded=true
	t.Run("GetExpandedTxPool", func(t *testing.T) {
		result, err := c.PoolTransactions(nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.GreaterOrEqual(t, len(result), 1, "Expected at least one transaction in pool")
	})

	// 3. Test GET /node/txpool/status
	t.Run("GetTxPoolStatus", func(t *testing.T) {
		status, err := c.TxPoolStatus()
		require.NoError(t, err)
		require.NotNil(t, status)
		require.True(t, status.Amount >= 1 && status.Amount <= 3, "Expected 1 or 3 transactions, got %d", status.Amount)
	})
}

func testFeesEndpoint(t *testing.T, testchain *testchain.Chain, ts *httptest.Server) {
	c := New(ts.URL)
	// 1. Test GET /fees/history
	t.Run("GetFeesHistory", func(t *testing.T) {
		blockCount := uint32(1)

		feesHistory, err := c.FeesHistory(blockCount, "2", nil)
		require.NoError(t, err)
		require.NotNil(t, feesHistory)

		expectedOldestBlock, err := testchain.Repo().NewBestChain().GetBlockID(2)
		require.NoError(t, err)
		expectedFeesHistory := &api.FeesHistory{
			OldestBlock: expectedOldestBlock,
			BaseFeePerGas: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(thor.InitialBaseFee)),
			},
			GasUsedRatio: []float64{
				0.0037,
			},
		}

		require.Equal(t, expectedFeesHistory, feesHistory)

		rewardPercentiles := []float64{10, 90}
		feesHistory, err = c.FeesHistory(blockCount, "2", rewardPercentiles)
		require.NoError(t, err)
		require.NotNil(t, feesHistory)

		expectedOldestBlock, err = testchain.Repo().NewBestChain().GetBlockID(2)
		require.NoError(t, err)
		expectedFeesHistory = &api.FeesHistory{
			OldestBlock: expectedOldestBlock,
			BaseFeePerGas: []*hexutil.Big{
				(*hexutil.Big)(big.NewInt(thor.InitialBaseFee)),
			},
			GasUsedRatio: []float64{
				0.0037,
			},
			Reward: [][]*hexutil.Big{
				{
					(*hexutil.Big)(big.NewInt(0)),
					(*hexutil.Big)(big.NewInt(0)),
				},
			},
		}

		require.Equal(t, expectedFeesHistory.OldestBlock, feesHistory.OldestBlock)
		require.Equal(t, expectedFeesHistory.BaseFeePerGas, feesHistory.BaseFeePerGas)
		require.Equal(t, expectedFeesHistory.GasUsedRatio, feesHistory.GasUsedRatio)

		// Compare rewards as strings
		require.Len(t, feesHistory.Reward, len(expectedFeesHistory.Reward), "should have same number of reward blocks")
		for i, blockRewards := range feesHistory.Reward {
			require.Len(t, blockRewards, len(expectedFeesHistory.Reward[i]), "block %d should have same number of rewards", i)
			for j, reward := range blockRewards {
				require.NotNil(t, reward, "reward %d in block %d should not be nil", j, i)
				require.NotNil(t, expectedFeesHistory.Reward[i][j], "expected reward %d in block %d should not be nil", j, i)
				require.Equal(t, expectedFeesHistory.Reward[i][j].String(), reward.String(), "reward %d in block %d should match", j, i)
			}
		}
	})

	// 2. Test GET /fees/priority
	t.Run("GetFeesPriority", func(t *testing.T) {
		feesPriority, err := c.FeesPriority()
		require.NoError(t, err)
		require.NotNil(t, feesPriority)

		expectedFeesPriority := &api.FeesPriority{
			MaxPriorityFeePerGas: (*hexutil.Big)(
				new(big.Int).Div(new(big.Int).Mul(big.NewInt(thor.InitialBaseFee), big.NewInt(5)), big.NewInt(100)),
			),
		}

		require.Equal(t, expectedFeesPriority, feesPriority)
	})
}
