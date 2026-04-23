// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package tests hosts end-to-end integration tests that exercise multiple
// subsystems (txpool + packer + consensus + chain index) via a testchain.
package tests

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// makeEthTx constructs and signs an 0x02 tx using the first dev account.
func makeEthTx(t *testing.T, chainID *big.Int, nonce uint64, maxFee, maxPrio int64) *tx.Transaction {
	t.Helper()
	to := thor.BytesToAddress([]byte("recipient"))
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(&to).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(maxFee)).
		MaxPriorityFeePerGas(big.NewInt(maxPrio)).
		Gas(21000).
		Nonce(nonce).
		Build()
	return tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
}

// allForksActive returns a fork config with every fork active at block 0.
func allForksActive() *thor.ForkConfig {
	return &thor.ForkConfig{}
}

// beforeInterstellar returns a fork config with GALACTICA active but
// INTERSTELLAR deferred far in the future. Used to verify 0x02 is rejected
// outside its activation window.
func beforeInterstellar() *thor.ForkConfig {
	return &thor.ForkConfig{
		GALACTICA:    0,
		INTERSTELLAR: 1_000_000,
	}
}

// TestEthType2_PoolAcceptsAfterInterstellar exercises the full pipeline:
// pool admission → packer adoption → consensus validation → chain index
// lookup by the ETH keccak256 hash. It would fail if CanonicalTxID isn't
// wired consistently across all layers.
func TestEthType2_PoolAcceptsAfterInterstellar(t *testing.T) {
	forks := allForksActive()
	tchain, err := testchain.NewWithFork(forks, 180)
	require.NoError(t, err)

	pool := txpool.New(tchain.Repo(), tchain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 10,
		MaxLifetime:     time.Hour,
	}, forks)

	chainID := new(big.Int).SetUint64(thor.ChainID(tchain.GenesisBlock().Header().ID()))
	trx := makeEthTx(t, chainID, 0, 10_000_000_000_000, 1_000_000_000)

	require.NoError(t, pool.AddLocal(trx), "pool should accept 0x02 after INTERSTELLAR")

	// Pack + validate via consensus (MintBlock runs both).
	require.NoError(t, tchain.MintBlock(trx))

	// Chain index: the tx must be retrievable by its CanonicalTxID (keccak256 hash).
	want := trx.CanonicalTxID()
	got, meta, err := tchain.Repo().NewBestChain().GetTransaction(want)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want, got.CanonicalTxID())
	assert.Equal(t, uint32(1), meta.BlockNum)

	// It must NOT be retrievable by its legacy ID (Blake2b-based).
	_, _, err = tchain.Repo().NewBestChain().GetTransaction(trx.ID())
	assert.Error(t, err, "legacy ID() must not be indexed for 0x02 txs")
}

// TestEthType2_PoolRejectsBeforeInterstellar confirms pool-level gating.
func TestEthType2_PoolRejectsBeforeInterstellar(t *testing.T) {
	forks := beforeInterstellar()
	tchain, err := testchain.NewWithFork(forks, 180)
	require.NoError(t, err)

	pool := txpool.New(tchain.Repo(), tchain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 10,
		MaxLifetime:     time.Hour,
	}, forks)

	chainID := new(big.Int).SetUint64(thor.ChainID(tchain.GenesisBlock().Header().ID()))
	trx := makeEthTx(t, chainID, 0, 10_000_000_000_000, 1_000_000_000)

	err = pool.AddLocal(trx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INTERSTELLAR")
}

// TestEthType2_PoolRejectsBadChainID confirms a 0x02 tx with wrong chainID is
// rejected regardless of whether it would otherwise be adoptable.
func TestEthType2_PoolRejectsBadChainID(t *testing.T) {
	forks := allForksActive()
	tchain, err := testchain.NewWithFork(forks, 180)
	require.NoError(t, err)

	pool := txpool.New(tchain.Repo(), tchain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 10,
		MaxLifetime:     time.Hour,
	}, forks)

	wrongChainID := big.NewInt(999)
	trx := makeEthTx(t, wrongChainID, 0, 10_000_000_000_000, 1_000_000_000)

	err = pool.AddLocal(trx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chain id")
}

// TestEthType2_PoolRejectsNonEmptyAccessList confirms that, even after
// INTERSTELLAR, a 0x02 tx carrying a non-empty access list is rejected at
// the resolve stage (runtime.ResolveTransaction, invoked by pool admission).
func TestEthType2_PoolRejectsNonEmptyAccessList(t *testing.T) {
	forks := allForksActive()
	tchain, err := testchain.NewWithFork(forks, 180)
	require.NoError(t, err)

	pool := txpool.New(tchain.Repo(), tchain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 10,
		MaxLifetime:     time.Hour,
	}, forks)

	chainID := new(big.Int).SetUint64(thor.ChainID(tchain.GenesisBlock().Header().ID()))
	to := thor.BytesToAddress([]byte("to"))
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(&to).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).
		Nonce(0).
		AccessList(tx.AccessList{
			{Address: thor.Address{0x01}, StorageKeys: []thor.Bytes32{{0x02}}},
		}).
		Build()
	trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)

	err = pool.AddLocal(trx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access list")
}
