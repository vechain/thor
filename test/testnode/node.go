package testnode

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/fees"
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
	frozenTxPool    *txpool.TxPool
	mempoolCloser   func()
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

	// Create a frozen tx pool - no housekeeping
	n.frozenTxPool = txpool.NewFrozenPool(
		n.Chain().Repo(),
		n.Chain().Stater(),
		txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute},
		n.Chain().GetForkConfig(),
	)

	apiHandler, apiCloser := api.New(
		n.Chain().Repo(),
		n.Chain().Stater(),
		n.frozenTxPool,
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
	n.mempoolCloser = n.txsLoop()
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

	if n.mempoolCloser != nil {
		n.mempoolCloser()
		n.mempoolCloser = nil
	}

	n.apiServer = nil
	n.frozenTxPool = nil
	return nil
}

// MintPoolTransactions processes all transactions in the frozen transaction pool,
// removing them from the pool and adding them to the chain. Returns an error if
// the minting process fails.
func (n *node) MintPoolTransactions() error {
	transactions := tx.Transactions{}
	// we miss a bunch of checks here because the wash cleans up bad txs
	// would require some refactoring of the tx pool to get those validations in
	for _, transaction := range n.frozenTxPool.Dump() {
		n.frozenTxPool.Remove(transaction.Hash(), transaction.ID())
		transactions = append(transactions, transaction)
	}
	return n.Chain().MintTransactions(genesis.DevAccounts()[0], transactions...)
}

func (n *node) Chain() *testchain.Chain {
	return n.chain
}

// APIServer returns the node api server
func (n *node) APIServer() *httptest.Server {
	return n.apiServer
}

func (n *node) txsLoop() func() {
	stopChan := make(chan any)
	go func() {
		txEvCh := make(chan *txpool.TxEvent, 10)
		sub := n.frozenTxPool.SubscribeTxEvent(txEvCh)
		defer sub.Unsubscribe()

		for {
			select {
			case <-stopChan:
				return
			case <-txEvCh:
				err := n.MintPoolTransactions()
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	}()
	return func() {
		stopChan <- true
	}
}
