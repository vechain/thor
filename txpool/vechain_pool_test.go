// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
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

func newVeChainPoolForUnitTest(t *testing.T) (*VeChainPool, *testchain.Chain) {
	t.Helper()
	metrics.InitializePrometheusMetrics()
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := newVeChainPool(tchain.Repo(), tchain.Stater(), Options{
		Limit:           1000,
		LimitPerAccount: 128,
		MaxLifetime:     time.Hour,
	}, tchain.GetForkConfig())
	return pool, tchain
}

func TestVeChainPoolRejectsEthereumTx(t *testing.T) {
	pool, tchain := newVeChainPoolForUnitTest(t)
	defer pool.Close()

	ethTx := buildEthTxForChain(t, tchain, genesis.DevAccounts()[0], 0, int64(thor.InitialBaseFee), 2*int64(thor.InitialBaseFee))
	require.ErrorContains(t, pool.Add(ethTx), "ethereum tx not accepted by VeChainPool")
	assert.Equal(t, 0, pool.Len())
}

func TestVeChainPoolGetByHashAndPoolNonce(t *testing.T) {
	pool, tchain := newVeChainPoolForUnitTest(t)
	defer pool.Close()

	legacyTx := newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	require.NoError(t, pool.Add(legacyTx))

	assert.Equal(t, legacyTx.ID(), pool.Get(legacyTx.ID()).ID())
	assert.Equal(t, legacyTx.ID(), pool.GetByHash(legacyTx.Hash()).ID())
	assert.Equal(t, uint64(0), pool.PoolNonce(genesis.DevAccounts()[0].Address))
	assert.True(t, pool.Remove(legacyTx.Hash(), legacyTx.ID()))
	assert.Nil(t, pool.GetByHash(legacyTx.Hash()))
}

func TestVeChainPoolFillSkipsEthereumTx(t *testing.T) {
	pool, tchain := newVeChainPoolForUnitTest(t)
	defer pool.Close()

	legacyTx := newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	ethTx := buildEthTxForChain(t, tchain, genesis.DevAccounts()[1], 0, int64(thor.InitialBaseFee), 2*int64(thor.InitialBaseFee))

	pool.Fill(tx.Transactions{legacyTx, ethTx})
	assert.Equal(t, 1, pool.Len())
	assert.Equal(t, legacyTx.ID(), pool.GetByHash(legacyTx.Hash()).ID())
	assert.Nil(t, pool.GetByHash(ethTx.Hash()))
}

func TestVeChainPoolExecutablesSortedReturnsObjects(t *testing.T) {
	pool, tchain := newVeChainPoolForUnitTest(t)
	defer pool.Close()

	legacyTxs := tx.Transactions{
		newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0]),
		newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1]),
	}
	for _, trx := range legacyTxs {
		require.NoError(t, pool.Add(trx))
	}

	executables, _, _, err := pool.wash(pool.repo.BestBlockSummary(), false)
	require.NoError(t, err)
	pool.executables.Store(executables)

	sorted := pool.executablesSorted()
	require.Len(t, sorted, len(executables))
	for i, txObj := range sorted {
		assert.Equal(t, executables[i].ID(), txObj.ID())
		assert.Same(t, executables[i], txObj.Transaction)
	}
}

func TestVeChainPoolStrictlyAddRejectsNonExecutable(t *testing.T) {
	pool, tchain := newVeChainPoolForUnitTest(t)
	defer pool.Close()

	futureRef := tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number() + 10)
	futureTx := newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, futureRef, 100, nil, tx.Features(0), genesis.DevAccounts()[0])

	require.ErrorContains(t, pool.StrictlyAdd(futureTx), "tx is not executable")
	assert.Equal(t, 0, pool.Len())
}
