// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func buildEthPoolTx(t *testing.T, chainID, nonce uint64, feeCap, tipCap *big.Int, signer genesis.DevAccount) *tx.Transaction {
	t.Helper()
	to := devAccounts[1].Address
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		Nonce(nonce).
		Gas(21_000).
		MaxFeePerGas(feeCap).
		MaxPriorityFeePerGas(tipCap).
		To(&to).
		Value(new(big.Int)).
		Build()
	return tx.MustSign(trx, signer.PrivateKey)
}

func newEthPoolTest(t *testing.T, options Options) (*EthPool, *testchain.Chain) {
	t.Helper()
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := NewEth(tchain.Repo(), tchain.Stater(), options, &thor.SoloFork)
	t.Cleanup(pool.Close)
	return pool, tchain
}

func nextTxEvent(t *testing.T, events <-chan *TxEvent) *TxEvent {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for transaction event")
		return nil
	}
}

func TestEthPoolAddRemoteNoncePlacementAndReplacement(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[5]
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))
	events := make(chan *TxEvent, 4)
	sub := pool.SubscribeTxEvent(events)
	defer sub.Unsubscribe()

	nonce1 := buildEthPoolTx(t, tchain.Repo().ChainID(), 1, fee, big.NewInt(100), signer)
	require.NoError(t, pool.AddRemote(nonce1))
	assert.Equal(t, uint64(0), pool.PoolNonce(signer.Address))
	queuedEvent := nextTxEvent(t, events)
	require.NotNil(t, queuedEvent.Executable)
	assert.False(t, *queuedEvent.Executable)

	nonce0 := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(100), signer)
	require.NoError(t, pool.AddRemote(nonce0))
	assert.Equal(t, uint64(2), pool.PoolNonce(signer.Address), "gap fill must promote the queued suffix")
	admittedEvent := nextTxEvent(t, events)
	promotedEvent := nextTxEvent(t, events)
	assert.Equal(t, nonce0.Hash(), admittedEvent.Tx.Hash())
	assert.Equal(t, nonce1.Hash(), promotedEvent.Tx.Hash())
	assert.True(t, *admittedEvent.Executable)
	assert.True(t, *promotedEvent.Executable)

	replacement := buildEthPoolTx(
		t,
		tchain.Repo().ChainID(),
		1,
		new(big.Int).Div(new(big.Int).Mul(fee, big.NewInt(110)), big.NewInt(100)),
		big.NewInt(110),
		signer,
	)
	require.NoError(t, pool.AddRemote(replacement))
	assert.Nil(t, pool.GetByHash(nonce1.Hash()))
	assert.Equal(t, replacement, pool.GetByHash(replacement.Hash()))
	assert.Equal(t, 2, pool.Len())

	err := pool.AddRemote(replacement)
	require.Error(t, err)
	assert.True(t, IsTxRejected(err))
	assert.Contains(t, err.Error(), "already known")
}

func TestEthPoolRemoveAndDump(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[6]
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))
	nonce1 := buildEthPoolTx(t, tchain.Repo().ChainID(), 1, fee, big.NewInt(100), signer)
	nonce0 := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(100), signer)
	require.NoError(t, pool.AddRemote(nonce1))
	require.NoError(t, pool.AddRemote(nonce0))

	assert.ElementsMatch(t, tx.Transactions{nonce0, nonce1}, pool.Dump())
	assert.Equal(t, pool.Len(), len(pool.Dump()))
	assert.False(t, pool.Remove(nonce0.Hash(), nonce1.ID()), "hash and ID must identify the same transaction")
	assert.NotNil(t, pool.GetByHash(nonce0.Hash()))

	assert.True(t, pool.Remove(nonce0.Hash(), nonce0.ID()))
	assert.False(t, pool.Remove(nonce0.Hash(), nonce0.ID()))
	assert.Nil(t, pool.GetByHash(nonce0.Hash()))
	assert.NotNil(t, pool.GetByHash(nonce1.Hash()), "demoted suffix remains queued")
	assert.Equal(t, uint64(0), pool.PoolNonce(signer.Address))
	assert.Equal(t, tx.Transactions{nonce1}, pool.Dump())

	assert.True(t, pool.Remove(nonce1.Hash(), nonce1.ID()))
	assert.Empty(t, pool.Dump())
	assert.Zero(t, pool.Len())
}

func TestEthPoolAddRemoteRejectsInvalidAndUnderpriced(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[6]
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))

	native := tx.MustSign(tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(tchain.Repo().ChainTag()).
		Gas(21_000).
		MaxFeePerGas(fee).
		MaxPriorityFeePerGas(big.NewInt(1)).
		Clause(tx.NewClause(nil)).
		Build(), signer.PrivateKey)
	err := pool.AddRemote(native)
	require.Error(t, err)
	assert.True(t, IsBadTx(err))

	unsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(tchain.Repo().ChainID()).
		Nonce(0).
		Gas(21_000).
		MaxFeePerGas(fee).
		MaxPriorityFeePerGas(big.NewInt(1)).
		Build()
	err = pool.AddRemote(unsigned)
	require.Error(t, err)
	assert.True(t, IsBadTx(err))

	invalidFees := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, big.NewInt(1), big.NewInt(2), signer)
	err = pool.AddRemote(invalidFees)
	require.Error(t, err)
	assert.True(t, IsBadTx(err))

	original := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(100), signer)
	require.NoError(t, pool.AddRemote(original))
	underpriced := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, new(big.Int).Add(fee, big.NewInt(1)), big.NewInt(109), signer)
	err = pool.AddRemote(underpriced)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replacement transaction underpriced")
	assert.Equal(t, original, pool.GetByHash(original.Hash()))

	wrongChain := buildEthPoolTx(t, tchain.Repo().ChainID()+1, 1, fee, big.NewInt(100), signer)
	err = pool.AddRemote(wrongChain)
	require.Error(t, err)
	assert.True(t, IsBadTx(err))
}

func TestEthPoolReplacementCostRollback(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[4]
	head := tchain.Repo().BestBlockSummary()
	baseFee := head.Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))

	original := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(100), signer)
	require.NoError(t, pool.AddRemote(original))
	originalObj := pool.all.GetByHash(original.Hash())
	require.NotNil(t, originalObj)
	require.NotNil(t, originalObj.Cost())

	st := tchain.Stater().NewState(head.Root())
	balance, err := builtin.Energy.Native(st, head.Header.Timestamp()+thor.BlockInterval()).Get(signer.Address)
	require.NoError(t, err)
	remaining := new(big.Int).Sub(balance, originalObj.Cost())
	externalOwner := vechainReservationOwner(thor.Bytes32{0xaa})
	require.NoError(t, pool.costs.reserve(externalOwner, signer.Address, remaining, balance))
	t.Cleanup(func() { _ = pool.costs.release(externalOwner) })

	replacement := buildEthPoolTx(
		t,
		tchain.Repo().ChainID(),
		0,
		new(big.Int).Div(new(big.Int).Mul(fee, big.NewInt(120)), big.NewInt(100)),
		big.NewInt(120),
		signer,
	)
	err = pool.AddRemote(replacement)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient energy")
	assert.Equal(t, original, pool.GetByHash(original.Hash()), "failed replacement must retain incumbent")
	assert.Nil(t, pool.GetByHash(replacement.Hash()))
	assert.Equal(t, balance, pool.costs.pendingCost(signer.Address), "failed replacement must restore its old reservation")
}

func TestEthPoolFeeBelowBaseAndQueueLimit(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 1, EthPriceBump: 10,
	})
	signer := devAccounts[7]
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()

	lowFee := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, new(big.Int).Sub(baseFee, big.NewInt(1)), big.NewInt(0), signer)
	require.NoError(t, pool.AddRemote(lowFee))
	assert.Equal(t, uint64(0), pool.PoolNonce(signer.Address))

	overflow := buildEthPoolTx(t, tchain.Repo().ChainID(), 2, baseFee, big.NewInt(0), signer)
	err := pool.AddRemote(overflow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "account queue limit exceeded")
}

func TestEthPoolPromotionStopsAtAffordablePrefix(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[3]
	head := tchain.Repo().BestBlockSummary()
	baseFee := head.Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))
	tip := big.NewInt(100)

	nonce1 := buildEthPoolTx(t, tchain.Repo().ChainID(), 1, fee, tip, signer)
	require.NoError(t, pool.AddRemote(nonce1))

	st := tchain.Stater().NewState(head.Root())
	balance, err := builtin.Energy.Native(st, head.Header.Timestamp()+thor.BlockInterval()).Get(signer.Address)
	require.NoError(t, err)
	effectivePrice := new(big.Int).Add(baseFee, tip)
	oneTxCost := new(big.Int).Mul(new(big.Int).SetUint64(21_000), effectivePrice)
	externalCost := new(big.Int).Sub(balance, oneTxCost)
	externalOwner := vechainReservationOwner(thor.Bytes32{0xbb})
	require.NoError(t, pool.costs.reserve(externalOwner, signer.Address, externalCost, balance))
	t.Cleanup(func() { _ = pool.costs.release(externalOwner) })

	nonce0 := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, tip, signer)
	require.NoError(t, pool.AddRemote(nonce0))
	assert.Equal(t, uint64(1), pool.PoolNonce(signer.Address), "unaffordable queued suffix must not be promoted")

	pool.all.lock.RLock()
	defer pool.all.lock.RUnlock()
	sender := pool.all.senders[signer.Address]
	require.NotNil(t, sender)
	assert.NotNil(t, sender.pending[0])
	assert.NotNil(t, sender.queue[1])
}

func TestCoordinatorRoutesOnlyEthAddRemoteAndRelaysEvent(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	coordinator := NewCoordinator(tchain.Repo(), tchain.Stater(), Options{
		Limit: 100, LimitPerAccount: 100,
		EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	}, &thor.SoloFork)
	defer coordinator.Close()

	events := make(chan *TxEvent, 1)
	sub := coordinator.SubscribeTxEvent(events)
	defer sub.Unsubscribe()

	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	remote := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, new(big.Int).Mul(baseFee, big.NewInt(2)), big.NewInt(100), devAccounts[8])
	require.NoError(t, coordinator.AddRemote(remote))
	assert.NotNil(t, coordinator.eth.GetByHash(remote.Hash()))
	assert.Nil(t, coordinator.vechain.GetByHash(remote.Hash()))

	select {
	case ev := <-events:
		assert.Equal(t, remote.Hash(), ev.Tx.Hash())
		require.NotNil(t, ev.Executable)
		assert.True(t, *ev.Executable)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for relayed Ethereum transaction event")
	}

	local := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, new(big.Int).Mul(baseFee, big.NewInt(2)), big.NewInt(100), devAccounts[9])
	require.NoError(t, coordinator.AddLocal(local))
	assert.NotNil(t, coordinator.vechain.GetByHash(local.Hash()), "non-remote flows must retain their current routing")
	assert.Nil(t, coordinator.eth.GetByHash(local.Hash()))
}

func TestCoordinatorRemovesAndDumpsEthereumTransactions(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	coordinator := NewCoordinator(tchain.Repo(), tchain.Stater(), Options{
		Limit: 100, LimitPerAccount: 100,
		EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	}, &thor.SoloFork)
	defer coordinator.Close()

	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	ethTx := buildEthPoolTx(
		t,
		tchain.Repo().ChainID(),
		0,
		new(big.Int).Mul(baseFee, big.NewInt(2)),
		big.NewInt(100),
		devAccounts[7],
	)
	require.NoError(t, coordinator.AddRemote(ethTx))

	assert.Contains(t, coordinator.Dump(), ethTx)
	assert.True(t, coordinator.Remove(ethTx.Hash(), ethTx.ID()))
	assert.NotContains(t, coordinator.Dump(), ethTx)
	assert.Nil(t, coordinator.GetByHash(ethTx.Hash()))
}

func TestEthPoolReinjectFromForkAndReplacementPolicy(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[2]
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))
	original := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(100), signer)
	events := make(chan *TxEvent, 4)
	sub := pool.SubscribeTxEvent(events)
	defer sub.Unsubscribe()

	require.NoError(t, pool.ReinjectFromFork(ForkReinjection{
		Discarded: tx.Transactions{original},
	}))
	assert.Equal(t, original, pool.GetByHash(original.Hash()))
	assert.Equal(t, uint64(1), pool.PoolNonce(signer.Address))
	event := nextTxEvent(t, events)
	assert.Equal(t, original.Hash(), event.Tx.Hash())
	require.NotNil(t, event.Executable)
	assert.True(t, *event.Executable)

	// Reinjection is idempotent for a hash already retained by the pool.
	require.NoError(t, pool.ReinjectFromFork(ForkReinjection{
		Discarded: tx.Transactions{original},
	}))
	assert.Equal(t, 1, pool.Len())
	select {
	case duplicateEvent := <-events:
		t.Fatalf("duplicate reinjection emitted event for %v", duplicateEvent.Tx.ID())
	case <-time.After(20 * time.Millisecond):
	}

	underpriced := buildEthPoolTx(
		t, tchain.Repo().ChainID(), 0,
		new(big.Int).Add(fee, big.NewInt(1)), big.NewInt(109), signer,
	)
	require.NoError(t, pool.ReinjectFromFork(ForkReinjection{
		Discarded: tx.Transactions{underpriced},
	}))
	assert.Equal(t, original, pool.GetByHash(original.Hash()))
	assert.Nil(t, pool.GetByHash(underpriced.Hash()))

	replacement := buildEthPoolTx(
		t, tchain.Repo().ChainID(), 0,
		new(big.Int).Div(new(big.Int).Mul(fee, big.NewInt(110)), big.NewInt(100)),
		big.NewInt(110), signer,
	)
	require.NoError(t, pool.ReinjectFromFork(ForkReinjection{
		Discarded: tx.Transactions{replacement},
	}))
	assert.Nil(t, pool.GetByHash(original.Hash()))
	assert.Equal(t, replacement, pool.GetByHash(replacement.Hash()))
}

func TestEthPoolReinjectDuplicateResetsBackwardNonce(t *testing.T) {
	pool, tchain := newEthPoolTest(t, Options{
		Limit: 100, EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	})
	signer := devAccounts[5]
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	trx := buildEthPoolTx(
		t, tchain.Repo().ChainID(), 0,
		new(big.Int).Mul(baseFee, big.NewInt(2)), big.NewInt(100), signer,
	)
	require.NoError(t, pool.AddRemote(trx))

	// Simulate a pool sender initialized against the orphaned head where nonce
	// had advanced. Reinjection of the already-retained hash must still reset it.
	pool.all.lock.Lock()
	pool.all.senders[signer.Address].stateNonce = 1
	pool.all.lock.Unlock()

	events := make(chan *TxEvent, 2)
	sub := pool.SubscribeTxEvent(events)
	defer sub.Unsubscribe()
	require.NoError(t, pool.ReinjectFromFork(ForkReinjection{
		Discarded: tx.Transactions{trx},
	}))
	assert.Equal(t, uint64(1), pool.PoolNonce(signer.Address))
	event := nextTxEvent(t, events)
	assert.Equal(t, trx.Hash(), event.Tx.Hash())
	assert.True(t, *event.Executable)
}

func TestCoordinatorPartitionsForkReinjection(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	coordinator := NewCoordinator(tchain.Repo(), tchain.Stater(), Options{
		Limit: 100, LimitPerAccount: 100,
		EthAccountSlots: 16, EthAccountQueue: 64, EthPriceBump: 10,
	}, &thor.SoloFork)
	defer coordinator.Close()

	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	ethTx := buildEthPoolTx(
		t, tchain.Repo().ChainID(), 0,
		new(big.Int).Mul(baseFee, big.NewInt(2)), big.NewInt(100), devAccounts[1],
	)
	to := devAccounts[1].Address
	nativeTx := tx.MustSign(tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(tchain.Repo().ChainTag()).
		Gas(21_000).
		MaxFeePerGas(new(big.Int).Mul(baseFee, big.NewInt(2))).
		MaxPriorityFeePerGas(big.NewInt(100)).
		Clause(tx.NewClause(&to)).
		Expiration(100).
		Build(), devAccounts[0].PrivateKey)

	require.NoError(t, coordinator.ReinjectFromFork(ForkReinjection{
		Discarded: tx.Transactions{nativeTx, ethTx},
	}))
	assert.NotNil(t, coordinator.eth.GetByHash(ethTx.Hash()))
	assert.Nil(t, coordinator.vechain.GetByHash(ethTx.Hash()))
	assert.NotNil(t, coordinator.vechain.GetByHash(nativeTx.Hash()))
}
