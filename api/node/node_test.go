// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package node_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/tx"

	//"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/txpool"
)

var (
	ts      *httptest.Server
	tclient *thorclient.Client
	pool    *txpool.TxPool
)

func TestNode(t *testing.T) {
	initCommServer(t)
	tclient = thorclient.New(ts.URL)

	peersStats, err := tclient.Peers()
	require.NoError(t, err)
	assert.Equal(t, 0, len(peersStats), "count should be zero")

	t.Run("getTransactions", testGetTransactions)
	t.Run("getTransactionsExpanded", testGetTransactionsExpanded)
	t.Run("getTransactionsWithFrom", testGetTransactionsWithFrom)
	t.Run("getTransactionsWithTo", testGetTransactionsWithTo)
	t.Run("getTransactionsWithBadExpanded", testGetTransactionsWithBadExpanded)
	t.Run("getTransactionsWithBadFrom", testGetTransactionsWithBadFrom)
	t.Run("getTransactionsWithBadTo", testGetTransactionsWithBadTo)
}

func initCommServer(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	chainTag := thorChain.Repo().ChainTag()

	pool = txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	})

	for i := 0; i < 3; i++ {
		transaction := new(tx.Builder).
			Clause(tx.NewClause(&genesis.DevAccounts()[1].Address)).
			ChainTag(chainTag).
			Expiration(10).
			Gas(21000).
			Nonce(uint64(i)).
			Build()
		transaction = tx.MustSign(transaction, genesis.DevAccounts()[0].PrivateKey)
		err := pool.Add(transaction)
		require.NoError(t, err)
	}

	communicator := comm.New(
		thorChain.Repo(),
		txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
			Limit:           10000,
			LimitPerAccount: 16,
			MaxLifetime:     10 * time.Minute,
		}),
	)

	router := mux.NewRouter()
	node.New(communicator, pool).Mount(router, "/node", true)

	ts = httptest.NewServer(router)
}

func httpGetAndCheckResponseStatus(t *testing.T, url string, responseStatusCode int) []byte {
	body, statusCode, err := tclient.RawHTTPClient().RawHTTPGet(url)
	require.NoError(t, err)
	assert.Equal(t, responseStatusCode, statusCode)
	return body
}

func testGetTransactions(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/node/mempool", 200)
	var txResponse []string
	err := json.Unmarshal(res, &txResponse)
	require.NoError(t, err)
	assert.NotNil(t, txResponse)
	assert.True(t, len(txResponse) > 0)
}

func testGetTransactionsExpanded(t *testing.T) {
	res := httpGetAndCheckResponseStatus(t, "/node/mempool?expanded=true", 200)
	var txResponse []transactions.Transaction
	err := json.Unmarshal(res, &txResponse)
	require.NoError(t, err)
	assert.NotNil(t, txResponse)
	assert.True(t, len(txResponse) > 0)
}

func testGetTransactionsWithFrom(t *testing.T) {
	acc := genesis.DevAccounts()[0]
	res := httpGetAndCheckResponseStatus(t, "/node/mempool?from="+acc.Address.String(), 200)
	var txResponse []string
	err := json.Unmarshal(res, &txResponse)
	require.NoError(t, err)
	assert.NotNil(t, txResponse)
	assert.True(t, len(txResponse) > 0)
}

func testGetTransactionsWithTo(t *testing.T) {
	acc := genesis.DevAccounts()[1]
	res := httpGetAndCheckResponseStatus(t, "/node/mempool?to="+acc.Address.String(), 200)
	var txResponse []string
	err := json.Unmarshal(res, &txResponse)
	require.NoError(t, err)
	assert.NotNil(t, txResponse)
	assert.True(t, len(txResponse) > 0)
}

func testGetTransactionsWithBadExpanded(t *testing.T) {
	httpGetAndCheckResponseStatus(t, "/node/mempool?expanded=notboolean", 400)
}

func testGetTransactionsWithBadFrom(t *testing.T) {
	httpGetAndCheckResponseStatus(t, "/node/mempool?from=0xinvalid", 400)
}

func testGetTransactionsWithBadTo(t *testing.T) {
	httpGetAndCheckResponseStatus(t, "/node/mempool?to=0xinvalid", 400)
}
