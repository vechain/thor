// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func TestEnergy(t *testing.T) {
	_, client, cancel := newChain(t)
	t.Cleanup(cancel)

	energy, err := NewEnergy(client)
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		name, err := energy.Name()
		require.NoError(t, err)
		require.Equal(t, "VeThor", name)
	})

	t.Run("Symbol", func(t *testing.T) {
		symbol, err := energy.Symbol()
		require.NoError(t, err)
		require.Equal(t, "VTHO", symbol)
	})

	t.Run("Decimals", func(t *testing.T) {
		decimals, err := energy.Decimals()
		require.NoError(t, err)
		require.Equal(t, uint8(18), decimals)
	})

	t.Run("TotalSupply", func(t *testing.T) {
		totalSupply, err := energy.TotalSupply()
		require.NoError(t, err)
		require.Equal(t, 1, totalSupply.Sign())
	})

	t.Run("TotalBurned", func(t *testing.T) {
		totalBurned, err := energy.TotalBurned()
		require.NoError(t, err)
		require.NotNil(t, totalBurned)
	})

	t.Run("BalanceOf", func(t *testing.T) {
		balance, err := energy.BalanceOf(genesis.DevAccounts()[0].Address)
		require.NoError(t, err)
		require.Equal(t, 1, balance.Sign())
	})

	t.Run("Approve-Approval-TransferFrom", func(t *testing.T) {
		acc1 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[1].PrivateKey)
		acc2 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[2].PrivateKey)
		acc3 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[3].PrivateKey)

		allowanceAmount := big.NewInt(1000)

		receipt, _, err := energy.Approve(acc1, acc2.Address(), allowanceAmount).Receipt(txContext(t), txOpts())
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "Transaction should not be reverted")

		approvals, err := energy.FilterApproval(newRange(receipt), nil, logdb.ASC)
		require.NoError(t, err)
		require.Len(t, approvals, 1, "There should be one approval event")

		allowance, err := energy.Allowance(acc1.Address(), acc2.Address())
		require.NoError(t, err)
		require.Equal(t, allowanceAmount, allowance, "Allowance should match the approved amount")

		transferAmount := big.NewInt(500)
		receipt, _, err = energy.TransferFrom(acc2, acc1.Address(), acc3.Address(), transferAmount).Receipt(txContext(t), txOpts())
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "TransferFrom should not be reverted")
	})

	t.Run("Transfer", func(t *testing.T) {
		acc1 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[1].PrivateKey)
		random, err := crypto.GenerateKey()
		require.NoError(t, err)
		acc2 := (*bind.PrivateKeySigner)(random)

		transferAmount := big.NewInt(999)

		receipt, _, err := energy.Transfer(acc1, acc2.Address(), transferAmount).Receipt(txContext(t), txOpts())
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "Transfer should not be reverted")

		balance, err := energy.BalanceOf(acc2.Address())
		require.NoError(t, err)
		require.Equal(t, transferAmount, balance, "Balance should match the transferred amount")

		transfers, err := energy.FilterTransfer(newRange(receipt), nil, logdb.ASC)
		require.NoError(t, err)

		found := false
		for _, transfer := range transfers {
			if transfer.To == acc2.Address() && transfer.From == acc1.Address() && transfer.Value.Cmp(transferAmount) == 0 {
				found = true
				break
			}
		}
		require.True(t, found, "Transfer event should be found in the logs")
	})
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
func newChain(t *testing.T) (*testchain.Chain, *thorclient.Client, func()) {
	chain, err := testchain.NewWithFork(&thor.SoloFork)
	if err != nil {
		t.Fatalf("failed to create test chain: %v", err)
	}
	if err := chain.MintBlock(genesis.DevAccounts()[0]); err != nil {
		t.Fatalf("failed to mint genesis block: %v", err)
	}

	// this is a bug with logsdb, can't have multiple instances on in-mem databases
	path := filepath.Join(t.TempDir(), "logs.db")
	logs, err := logdb.New(path)
	if err != nil {
		t.Fatalf("failed to create logdb: %v", err)
	}
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
		SkipLogs:          false,
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

	ts := httptest.NewServer(handler)
	client := thorclient.New(ts.URL)

	return chain, client, cancelSubs
}
