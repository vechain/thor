// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package node_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/txpool"
)

var ts *httptest.Server

func TestNode(t *testing.T) {
	initCommServer(t)
	tclient := thorclient.New(ts.URL)

	peersStats, err := tclient.Peers()
	require.NoError(t, err)
	assert.Equal(t, 0, len(peersStats), "count should be zero")
}

func initCommServer(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	communicator := comm.New(
		thorChain.Repo(),
		txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
			Limit:           10000,
			LimitPerAccount: 16,
			MaxLifetime:     10 * time.Minute,
		}))

	router := mux.NewRouter()
	node.New(communicator).Mount(router, "/node")

	ts = httptest.NewServer(router)
}
