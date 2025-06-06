// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testnode

import (
	"errors"
	"net/http/httptest"
	"sync/atomic"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/tx"
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
	txPool          transactions.Pool
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

	apiHandler, apiCloser := api.New(
		n.Chain().Repo(),
		n.Chain().Stater(),
		n.txPool,
		n.Chain().LogDB(),
		bft.NewMockedEngine(n.Chain().Repo().GenesisBlock().Header().ID()),
		&solo.Communicator{},
		n.Chain().GetForkConfig(),
		api.Config{
			AllowedOrigins: "*",
			BacktraceLimit: 100,
			CallGasLimit:   40_000_000,
			SkipLogs:       false,
			Fees: fees.Config{
				APIBacktraceLimit:          100,
				FixedCacheSize:             1024,
				PriorityIncreasePercentage: 5,
			},
			EnableReqLogger:  &atomic.Bool{},
			LogsLimit:        100,
			AllowedTracers:   []string{"all"},
			EnableDeprecated: true,
			SoloMode:         true,
			EnableTxpool:     true,
		},
	)

	n.apiServer = httptest.NewServer(apiHandler)
	n.apiServerCloser = apiCloser
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
