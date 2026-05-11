// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newCoordinatorTestPool(t *testing.T) (*TxPool, *testchain.Chain) {
	t.Helper()
	metrics.InitializePrometheusMetrics()
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)

	pool := New(tchain.Repo(), tchain.Stater(), Options{
		Limit:           1000,
		LimitPerAccount: 128,
		MaxLifetime:     0,
	}, tchain.GetForkConfig())
	return pool, tchain
}

func newEthPoolTestPool(t *testing.T) (*EthPool, *testchain.Chain) {
	t.Helper()
	metrics.InitializePrometheusMetrics()
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	return NewEth(tchain.Repo(), tchain.Stater(), tchain.GetForkConfig()), tchain
}

func buildEthTxForChain(
	t *testing.T,
	tchain *testchain.Chain,
	sender genesis.DevAccount,
	nonce uint64,
	maxPriorityFeePerGas int64,
	maxFeePerGas int64,
) *tx.Transaction {
	t.Helper()
	to := genesis.DevAccounts()[1].Address
	trx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainID(thor.GetEthChainID(tchain.GenesisBlock().Header().ID())).
		Nonce(nonce).
		MaxPriorityFeePerGas(big.NewInt(maxPriorityFeePerGas)).
		MaxFeePerGas(big.NewInt(maxFeePerGas)).
		GasLimit(21000).
		To(&to).
		Value(big.NewInt(1)).
		Build(sender.PrivateKey)
	require.NoError(t, err)
	return trx
}

func TestZZZTxPoolCoordinatorRoutesFamilies(t *testing.T) {
	pool, tchain := newCoordinatorTestPool(t)
	defer pool.Close()

	legacyTx := newTx(tx.TypeLegacy, tchain.Repo().ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	ethTx := buildEthTxForChain(t, tchain, genesis.DevAccounts()[1], 0, int64(thor.InitialBaseFee), 2*int64(thor.InitialBaseFee))

	require.NoError(t, pool.Add(legacyTx))
	require.NoError(t, pool.Add(ethTx))

	assert.Equal(t, 1, pool.VeChain().Len())
	assert.Equal(t, 1, pool.Eth().Len())
	assert.Equal(t, 2, pool.Len())
	assert.Equal(t, legacyTx.ID(), pool.Get(legacyTx.ID()).ID())
	assert.Equal(t, ethTx.ID(), pool.Get(ethTx.ID()).ID())
	assert.Equal(t, legacyTx.ID(), pool.GetByHash(legacyTx.Hash()).ID())
	assert.Equal(t, ethTx.ID(), pool.GetByHash(ethTx.Hash()).ID())
}

func TestZZZTxPoolCoordinatorPreservesEthNonceOrder(t *testing.T) {
	pool, tchain := newCoordinatorTestPool(t)
	defer pool.Close()

	sender := genesis.DevAccounts()[2]
	base := int64(thor.InitialBaseFee)
	eth0 := buildEthTxForChain(t, tchain, sender, 0, base, 2*base)
	eth1 := buildEthTxForChain(t, tchain, sender, 1, 3*base, 4*base)
	eth2 := buildEthTxForChain(t, tchain, sender, 2, 2*base, 3*base)

	require.NoError(t, pool.Add(eth0))
	require.NoError(t, pool.Add(eth1))
	require.NoError(t, pool.Add(eth2))

	execs := pool.Executables()
	require.Len(t, execs, 3)
	assert.Equal(t, eth0.ID(), execs[0].ID())
	assert.Equal(t, eth1.ID(), execs[1].ID())
	assert.Equal(t, eth2.ID(), execs[2].ID())
	assert.Equal(t, uint64(3), pool.PoolNonce(sender.Address))
}

func TestZZZEthTxsHaveNonZeroPriorityInMerge(t *testing.T) {
	pool, tchain := newCoordinatorTestPool(t)
	defer pool.Close()

	senderA := genesis.DevAccounts()[4]
	senderB := genesis.DevAccounts()[5]
	base := int64(thor.InitialBaseFee)

	ethHigh := buildEthTxForChain(t, tchain, senderA, 0, 5*base, 10*base)
	ethLow := buildEthTxForChain(t, tchain, senderB, 0, base, 2*base)
	require.NoError(t, pool.Add(ethHigh))
	require.NoError(t, pool.Add(ethLow))

	groups := pool.eth.executablePendingGroups()
	require.NotEmpty(t, groups)
	for _, g := range groups {
		for _, txObj := range g {
			require.NotNil(t, txObj.priorityGasPrice, "Eth tx must have non-nil priorityGasPrice")
			assert.True(t, txObj.priorityGasPrice.Sign() > 0, "Eth tx priorityGasPrice must be > 0")
		}
	}

	execs := pool.Executables()
	require.Len(t, execs, 2)
	assert.Equal(t, ethHigh.ID(), execs[0].ID(), "higher-priority Eth tx must come first in merge")
	assert.Equal(t, ethLow.ID(), execs[1].ID())
}

func TestZZZEthPoolReplacementAndQueue(t *testing.T) {
	pool, tchain := newEthPoolTestPool(t)
	defer pool.Close()

	sender := genesis.DevAccounts()[3]
	base := int64(thor.InitialBaseFee)
	future := buildEthTxForChain(t, tchain, sender, 2, base, 2*base)
	require.NoError(t, pool.Add(future))
	assert.Empty(t, pool.Executables())
	assert.Equal(t, uint64(0), pool.PoolNonce(sender.Address))

	nonce0 := buildEthTxForChain(t, tchain, sender, 0, base, 2*base)
	require.NoError(t, pool.Add(nonce0))
	assert.Len(t, pool.Executables(), 1)

	replacement := buildEthTxForChain(t, tchain, sender, 0, 2*base, 3*base)
	require.NoError(t, pool.Add(replacement))
	assert.Nil(t, pool.GetByHash(nonce0.Hash()))
	assert.Equal(t, replacement.ID(), pool.GetByHash(replacement.Hash()).ID())
}
