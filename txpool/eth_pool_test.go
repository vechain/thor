// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestEthPoolAddGetDumpAndRemove(t *testing.T) {
	pool, tchain := newEthPoolTestPool(t)
	defer pool.Close()

	sender := genesis.DevAccounts()[0]
	base := int64(thor.InitialBaseFee)
	eth0 := buildEthTxForChain(t, tchain, sender, 0, base, 2*base)
	eth1 := buildEthTxForChain(t, tchain, sender, 1, base, 2*base)

	require.NoError(t, pool.Add(eth0))
	require.NoError(t, pool.AddLocal(eth1))

	assert.Equal(t, 2, pool.Len())
	assert.Equal(t, eth0.ID(), pool.Get(eth0.ID()).ID())
	assert.Equal(t, eth1.ID(), pool.GetByHash(eth1.Hash()).ID())
	assert.Len(t, pool.Dump(), 2)
	assert.Equal(t, uint64(2), pool.PoolNonce(sender.Address))
	assert.Len(t, pool.Executables(), 2)

	assert.True(t, pool.Remove(eth0.Hash(), eth0.ID()))
	assert.Nil(t, pool.GetByHash(eth0.Hash()))
	assert.Equal(t, 1, pool.Len())
	assert.False(t, pool.Remove(eth0.Hash(), eth0.ID()))
}

func TestEthPoolRejectsNonEthAndInvalidEthTx(t *testing.T) {
	pool, tchain := newEthPoolTestPool(t)
	defer pool.Close()

	legacyTx := newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	require.ErrorContains(t, pool.Add(legacyTx), "non-ethereum tx")

	otherChainTx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainID(pool.ethChainID + 1).
		Nonce(0).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		GasLimit(21000).
		Build(genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, err)
	require.ErrorContains(t, pool.Add(otherChainTx), "does not match network chain ID")
}

func TestEthPoolRejectsBeforeInterstellar(t *testing.T) {
	metrics.InitializePrometheusMetrics()
	tchain, err := testchain.NewWithFork(&thor.NoFork, 180)
	require.NoError(t, err)
	pool := NewEth(tchain.Repo(), tchain.Stater(), Options{
		Limit:           1000,
		LimitPerAccount: 128,
	}, tchain.GetForkConfig())
	defer pool.Close()

	ethTx := buildEthTxForChain(t, tchain, genesis.DevAccounts()[0], 0, int64(thor.InitialBaseFee), 2*int64(thor.InitialBaseFee))
	require.ErrorContains(t, pool.Add(ethTx), "not supported before the INTERSTELLAR fork")
}

func TestEthPoolFillFiltersAndRoutesEthTxs(t *testing.T) {
	pool, tchain := newEthPoolTestPool(t)
	defer pool.Close()

	ethTx := buildEthTxForChain(t, tchain, genesis.DevAccounts()[0], 0, int64(thor.InitialBaseFee), 2*int64(thor.InitialBaseFee))
	legacyTx := newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])

	pool.Fill(tx.Transactions{legacyTx, ethTx})
	assert.Equal(t, 1, pool.Len())
	assert.Equal(t, ethTx.ID(), pool.GetByHash(ethTx.Hash()).ID())
	assert.Nil(t, pool.GetByHash(legacyTx.Hash()))
}

func TestEthPoolProcessHeadChangeEvictsIncludedTx(t *testing.T) {
	pool, tchain := newEthPoolTestPool(t)
	defer pool.Close()

	sender := genesis.DevAccounts()[0]
	base := int64(thor.InitialBaseFee)
	prevHead := tchain.Repo().BestBlockSummary()
	eth0 := buildEthTxForChain(t, tchain, sender, 0, base, 2*base)
	eth1 := buildEthTxForChain(t, tchain, sender, 1, base, 2*base)
	require.NoError(t, pool.Add(eth0))
	require.NoError(t, pool.Add(eth1))

	require.NoError(t, tchain.MintBlock(eth0))
	newHead := tchain.Repo().BestBlockSummary()
	pool.processHeadChange(prevHead, newHead)

	assert.Nil(t, pool.GetByHash(eth0.Hash()))
	assert.Equal(t, eth1.ID(), pool.GetByHash(eth1.Hash()).ID())
	assert.Equal(t, uint64(2), pool.PoolNonce(sender.Address))
	execs := pool.Executables()
	require.Len(t, execs, 1)
	assert.Equal(t, eth1.ID(), execs[0].ID())
}

func TestEthPoolEmitsEventsForAddAndReplacement(t *testing.T) {
	pool, tchain := newEthPoolTestPool(t)
	defer pool.Close()

	events := make(chan *TxEvent, 4)
	sub := pool.SubscribeTxEvent(events)
	defer sub.Unsubscribe()

	sender := genesis.DevAccounts()[1]
	base := int64(thor.InitialBaseFee)
	original := buildEthTxForChain(t, tchain, sender, 0, base, 2*base)
	replacement := buildEthTxForChain(t, tchain, sender, 0, 2*base, 3*base)

	require.NoError(t, pool.Add(original))
	require.NoError(t, pool.Add(replacement))

	seen := map[thor.Bytes32]bool{}
	timeout := time.After(3 * time.Second)
	for len(seen) < 2 {
		select {
		case ev := <-events:
			seen[ev.Tx.ID()] = true
		case <-timeout:
			t.Fatalf("timed out waiting for eth pool events; seen=%v", seen)
		}
	}
	assert.True(t, seen[original.ID()])
	assert.True(t, seen[replacement.ID()])
}
