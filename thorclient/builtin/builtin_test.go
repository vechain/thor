// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"context"
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func txContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func txOpts() *bind.TxOptions {
	gas := uint64(10_000_000)
	return &bind.TxOptions{
		Gas: &gas,
	}
}

func newRange(receipt *transactions.Receipt) *events.Range {
	block64 := uint64(receipt.Meta.BlockNumber)
	return &events.Range{
		From: &block64,
		To:   &block64,
	}
}

// mockPool instantly mines a block when adding a transaction to the pool.
type mockPool struct {
	txs       tx.Transactions
	validator genesis.DevAccount
	chain     *testchain.Chain

	mutex  sync.Mutex
	scope  event.SubscriptionScope
	txFeed event.Feed
}

var _ transactions.Pool = (*mockPool)(nil)

func (m *mockPool) Get(txID thor.Bytes32) *tx.Transaction {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, tx := range m.txs {
		if tx.ID() == txID {
			return tx
		}
	}
	return nil
}

func (m *mockPool) AddLocal(trx *tx.Transaction) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.txs == nil {
		m.txs = make(tx.Transactions, 0)
	}
	m.txs = append(m.txs, trx)
	executable := true
	m.txFeed.Send(&txpool.TxEvent{
		Tx:         trx,
		Executable: &executable,
	})
	return m.chain.MintBlock(m.validator, trx)
}

func (m *mockPool) Dump() tx.Transactions {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.txs
}

func (m *mockPool) Len() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return len(m.txs)
}

func (m *mockPool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	return m.scope.Track(m.txFeed.Subscribe(ch))
}

// newChain creates a node with the API enabled to test the smart contract wrappers
func newChain(t *testing.T, useExecutor bool) (*testchain.Chain, *thorclient.Client) {
	accounts := genesis.DevAccounts()
	authAccs := make([]genesis.Authority, 0, len(accounts))
	stateAccs := make([]genesis.Account, 0, len(accounts))

	for i, acc := range accounts {
		if i == 0 {
			authAccs = append(authAccs, genesis.Authority{
				MasterAddress:   acc.Address,
				EndorsorAddress: acc.Address,
				Identity:        thor.BytesToBytes32([]byte("master")),
			})
		}
		bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
		stateAccs = append(stateAccs, genesis.Account{
			Address: acc.Address,
			Balance: (*genesis.HexOrDecimal256)(bal),
			Energy:  (*genesis.HexOrDecimal256)(bal),
			Code:    "",
			Storage: nil,
		})
	}

	mbp := uint64(1_000)
	genConfig := genesis.CustomGenesis{
		LaunchTime: uint64(time.Now().Unix()),
		GasLimit:   thor.InitialGasLimit,
		ExtraData:  "",
		ForkConfig: &thor.SoloFork,
		Authority:  authAccs,
		Accounts:   stateAccs,
		Params: genesis.Params{
			MaxBlockProposers: &mbp,
		},
	}

	if useExecutor {
		approvers := make([]genesis.Approver, 3)
		for i, acc := range genesis.DevAccounts()[0:3] {
			approvers[i] = genesis.Approver{
				Address:  acc.Address,
				Identity: datagen.RandomHash(),
			}
		}
		genConfig.Executor = genesis.Executor{
			Approvers: approvers,
		}
	} else {
		genConfig.Params.ExecutorAddress = &genesis.DevAccounts()[0].Address
	}

	gene, err := genesis.NewCustomNet(&genConfig)
	require.NoError(t, err, "failed to create genesis builder")

	chain, err := testchain.NewIntegrationTestChainWithGenesis(gene, &thor.SoloFork)
	if err != nil {
		t.Fatalf("failed to create test chain: %v", err)
	}
	// this is a bug with logsdb, can't have multiple instances of in-mem databases
	path := filepath.Join(t.TempDir(), "logs.db")
	logs, err := logdb.New(path)
	if err != nil {
		t.Fatalf("failed to create logdb: %v", err)
	}
	t.Cleanup(func() {
		if err := logs.Close(); err != nil {
			t.Fatalf("failed to close logdb: %v", err)
		}
	})

	chain = testchain.New(
		chain.Database(),
		chain.Genesis(),
		chain.Engine(),
		chain.Repo(),
		chain.Stater(),
		chain.GenesisBlock(),
		logs,
		chain.GetForkConfig(),
	)
	if err := chain.MintBlock(genesis.DevAccounts()[0]); err != nil {
		t.Fatalf("failed to mint genesis block: %v", err)
	}

	pool := &mockPool{
		txs:       make(tx.Transactions, 0),
		validator: genesis.DevAccounts()[0],
		chain:     chain,
	}

	apiConfig := api.Config{
		AllowedOrigins:    "*",
		BacktraceLimit:    1000,
		CallGasLimit:      40_000_000,
		AllowCustomTracer: true,
		EnableReqLogger:   &atomic.Bool{},
		LogsLimit:         1000,
		AllowedTracers:    []string{"all"},
		SoloMode:          true,
		EnableDeprecated:  true,
		EnableTxpool:      true,
		Fees: fees.Config{
			FixedCacheSize:             1000,
			PriorityIncreasePercentage: 10,
			APIBacktraceLimit:          1000,
		},
	}

	handler, cancelSubs := api.New(
		chain.Repo(),
		chain.Stater(),
		pool,
		chain.LogDB(),
		chain.Engine(),
		nil,
		chain.GetForkConfig(),
		apiConfig,
	)

	t.Cleanup(cancelSubs)

	ts := httptest.NewServer(handler)

	return chain, thorclient.New(ts.URL)
}
