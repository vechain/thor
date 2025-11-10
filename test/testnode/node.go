// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testnode

import (
	"errors"
	"net/http/httptest"

	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/debug"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/fees"
	node2 "github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// Node represents a complete test node with chain, API server, and transaction pool capabilities
type Node interface {
	// Chain returns the underlying chain interface
	Chain() *testchain.Chain

	// Start starts the node
	Start() error

	// Stop stops the node
	Stop() error

	// APIServer returns the node api server
	APIServer() *httptest.Server
}

// node implements the Node interface
type node struct {
	chain           *testchain.Chain
	apiServer       *httptest.Server
	apiServerCloser func()
	txPool          txpool.Pool
}

// Start starts the node and initializes all necessary components including the API server
// and transaction pool. Returns an error if the node is already running or if required
// components are not properly initialized.
func (n *node) Start() error {
	if n.chain == nil {
		return errors.New("chain is not initialized")
	}
	if n.apiServer != nil {
		return errors.New("node is already running")
	}

	// Create a mock transaction pool
	n.txPool = &instantMintPool{
		txs:       tx.Transactions{},
		validator: genesis.DevAccounts()[0],
		chain:     n.chain,
	}

	router := mux.NewRouter()
	repo := n.chain.Repo()
	stater := n.chain.Stater()
	logDB := n.chain.LogDB()
	forkConfig := n.chain.GetForkConfig()
	engine := bft.NewMockedEngine(repo.GenesisBlock().Header().ID())

	accounts.New(repo, stater, 40_000_000, forkConfig, engine, true).Mount(router, "/accounts")
	events.New(repo, logDB, 1000).Mount(router, "/logs/event")
	transfers.New(repo, logDB, 1000).Mount(router, "/logs/transfer")
	blocks.New(repo, engine).Mount(router, "/blocks")
	transactions.New(repo, n.txPool).Mount(router, "/transactions")
	debug.New(repo, stater, forkConfig, engine,
		40_000_000,
		true,
		[]string{"all"},
		true,
	).Mount(router, "/debug")
	node2.New(&solo.Communicator{}, n.txPool, true).Mount(router, "/node")
	fees.New(repo, engine, forkConfig, stater, fees.Config{
		APIBacktraceLimit:          1000,
		PriorityIncreasePercentage: 5,
		FixedCacheSize:             1000,
	}).Mount(router, "/fees")
	subs := subscriptions.New(repo, []string{"*"}, 1000, n.txPool, true)
	subs.Mount(router, "/subscriptions")

	n.apiServer = httptest.NewServer(router)
	n.apiServerCloser = func() {
		subs.Close()
		n.apiServer.Close()
	}
	return nil
}

// Stop gracefully shuts down the node, closing all resources including the API server
// and transaction pool. Returns an error if the node is not running.
func (n *node) Stop() error {
	if n.apiServer == nil {
		return errors.New("node is not running")
	}

	if n.apiServerCloser != nil {
		n.apiServerCloser()
		n.apiServerCloser = nil
	}

	n.apiServer = nil
	return nil
}

func (n *node) Chain() *testchain.Chain {
	return n.chain
}

// APIServer returns the node api server
func (n *node) APIServer() *httptest.Server {
	return n.apiServer
}
