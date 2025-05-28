package builtin

import (
	"context"
	"log/slog"
	"math/big"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/txpool"
)

var (
	client *thorclient.Client
	chain  *testchain.Chain
)

func TestMain(m *testing.M) {
	var err error

	chain, err = testchain.NewWithFork(&thor.SoloFork)
	if err != nil {
		panic(err)
	}
	if err := chain.MintBlock(genesis.DevAccounts()[0]); err != nil {
		panic(err)
	}

	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{
		Limit:           1000,
		LimitPerAccount: 128,
		MaxLifetime:     20 * time.Minute,
	}, chain.GetForkConfig())

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
	defer cancelSubs()

	txpoolChan := make(chan *txpool.TxEvent, 1000)

	go func() {
		for ev := range txpoolChan {
			if err := chain.MintBlock(genesis.DevAccounts()[0], ev.Tx); err != nil {
				// Log the error if minting the block fails
				slog.Error("failed to mint block from txpool event", "error", err, "txID", ev.Tx.ID())
			}
		}
	}()

	sub := pool.SubscribeTxEvent(txpoolChan)
	defer sub.Unsubscribe()

	ts := httptest.NewServer(handler)
	client = thorclient.New(ts.URL)
	m.Run()
}

func TestEnergy(t *testing.T) {
	energy, err := NewEnergy(client)
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		name, err := energy.Name()
		require.NoError(t, err)
		require.Equal(t, "VeThor", name)
	})

	t.Run("Symbol", func(t *testing.T) {
		t.Parallel()
		symbol, err := energy.Symbol()
		require.NoError(t, err)
		require.Equal(t, "VTHO", symbol)
	})

	t.Run("Decimals", func(t *testing.T) {
		t.Parallel()
		decimals, err := energy.Decimals()
		require.NoError(t, err)
		require.Equal(t, uint8(18), decimals)
	})

	t.Run("TotalSupply", func(t *testing.T) {
		t.Parallel()
		totalSupply, err := energy.TotalSupply()
		require.NoError(t, err)
		require.Equal(t, 1, totalSupply.Sign())
	})

	t.Run("TotalBurned", func(t *testing.T) {
		t.Parallel()
		totalBurned, err := energy.TotalBurned()
		require.NoError(t, err)
		require.Equal(t, 0, totalBurned.Sign())
	})

	t.Run("BalanceOf", func(t *testing.T) {
		t.Parallel()
		balance, err := energy.BalanceOf(genesis.DevAccounts()[0].Address)
		require.NoError(t, err)
		require.Equal(t, 1, balance.Sign())
	})

	t.Run("Approve-Approval-TransferFrom", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		acc1 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[1].PrivateKey)
		acc2 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[2].PrivateKey)
		acc3 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[3].PrivateKey)

		allowanceAmount := big.NewInt(1000)

		slog.Info("sending approve", "t", time.Now())
		receipt, _, err := energy.Approve(acc1, acc2.Address(), allowanceAmount).Receipt(ctx, &bind.Options{})
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "Transaction should not be reverted")

		approvals, err := energy.FilterApproval(nil, nil, logdb.ASC)
		require.NoError(t, err)
		found := false
		for _, approval := range approvals {
			if approval.Owner == acc1.Address() && approval.Spender == acc2.Address() && approval.Value.Cmp(allowanceAmount) == 0 {
				found = true
				break
			}
		}
		require.True(t, found, "Approval event should be found in the logs")

		allowance, err := energy.Allowance(acc1.Address(), acc2.Address())
		require.NoError(t, err)
		require.Equal(t, allowanceAmount, allowance, "Allowance should match the approved amount")

		transferAmount := big.NewInt(500)
		receipt, _, err = energy.TransferFrom(acc2, acc1.Address(), acc3.Address(), transferAmount).Receipt(ctx, &bind.Options{})
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "TransferFrom should not be reverted")
	})

	t.Run("Transfer", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		acc1 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[1].PrivateKey)
		random, err := crypto.GenerateKey()
		require.NoError(t, err)
		acc2 := (*bind.PrivateKeySigner)(random)

		transferAmount := big.NewInt(999)

		receipt, _, err := energy.Transfer(acc1, acc2.Address(), transferAmount).Receipt(ctx, &bind.Options{})
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "Transfer should not be reverted")

		balance, err := energy.BalanceOf(acc2.Address())
		require.NoError(t, err)
		require.Equal(t, transferAmount, balance, "Balance should match the transferred amount")

		transfers, err := energy.FilterTransfer(nil, nil, logdb.ASC)
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
