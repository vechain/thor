// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newCoordinatorTest(t *testing.T) (*TxPoolCoordinator, *testchain.Chain) {
	t.Helper()
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	coordinator := NewCoordinator(tchain.Repo(), tchain.Stater(), Options{
		Limit:             100,
		LimitPerAccount:   100,
		MaxLifetime:       time.Hour,
		EthAccountSlots:   16,
		EthAccountQueue:   64,
		EthPriceBump:      10,
		BlocklistFetchURL: "",
	}, &thor.SoloFork)
	return coordinator, tchain
}

func coordinatorNativeTx(t *testing.T, tchain *testchain.Chain, signer int) *tx.Transaction {
	t.Helper()
	to := devAccounts[(signer+1)%len(devAccounts)].Address
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	return tx.MustSign(tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(tchain.Repo().ChainTag()).
		Gas(21_000).
		MaxFeePerGas(new(big.Int).Mul(baseFee, big.NewInt(2))).
		MaxPriorityFeePerGas(big.NewInt(100)).
		Clause(tx.NewClause(&to)).
		Expiration(100).
		Nonce(uint64(signer)).
		Build(), devAccounts[signer].PrivateKey)
}

func coordinatorEthTx(t *testing.T, tchain *testchain.Chain, signer int, nonce uint64) *tx.Transaction {
	t.Helper()
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	return buildEthPoolTx(
		t,
		tchain.Repo().ChainID(),
		nonce,
		new(big.Int).Mul(baseFee, big.NewInt(2)),
		big.NewInt(100),
		devAccounts[signer],
	)
}

func TestCoordinatorRelaysVeChainAdmissionEvent(t *testing.T) {
	coordinator, tchain := newCoordinatorTest(t)
	defer coordinator.Close()

	events := make(chan *TxEvent, 1)
	sub := coordinator.SubscribeTxEvent(events)
	defer sub.Unsubscribe()

	native := coordinatorNativeTx(t, tchain, 0)
	require.NoError(t, coordinator.AddRemote(native))

	event := nextTxEvent(t, events)
	require.NotNil(t, event)
	assert.Equal(t, native.Hash(), event.Tx.Hash())
	assert.Same(t, native, coordinator.vechain.Get(native.ID()))
	assert.Nil(t, coordinator.eth.GetByHash(native.Hash()))
}

func TestCoordinatorGetLenAndPoolNonce(t *testing.T) {
	coordinator, tchain := newCoordinatorTest(t)
	defer coordinator.Close()

	native := coordinatorNativeTx(t, tchain, 1)
	ethereum := coordinatorEthTx(t, tchain, 2, 0)
	coordinator.Fill(tx.Transactions{native})
	require.NoError(t, coordinator.AddRemote(ethereum))

	assert.Same(t, native, coordinator.Get(native.ID()))
	assert.Same(t, ethereum, coordinator.Get(ethereum.ID()),
		"Get must fall through VeChainPool to EthPool")
	assert.Nil(t, coordinator.Get(thor.Bytes32{0xff}))
	assert.Equal(t, coordinator.vechain.Len()+coordinator.eth.Len(), coordinator.Len())
	assert.Equal(t, 2, coordinator.Len())
	assert.Equal(t, uint64(1), coordinator.PoolNonce(devAccounts[2].Address))
	assert.Equal(t, coordinator.eth.PoolNonce(devAccounts[2].Address),
		coordinator.PoolNonce(devAccounts[2].Address))
}

func TestCoordinatorGetByHashAndRemoveAcrossFamilies(t *testing.T) {
	coordinator, tchain := newCoordinatorTest(t)
	defer coordinator.Close()

	native := coordinatorNativeTx(t, tchain, 8)
	ethereum := coordinatorEthTx(t, tchain, 9, 0)
	coordinator.Fill(tx.Transactions{native})
	require.NoError(t, coordinator.AddRemote(ethereum))

	assert.Same(t, native, coordinator.GetByHash(native.Hash()))
	assert.Same(t, ethereum, coordinator.GetByHash(ethereum.Hash()))
	assert.Nil(t, coordinator.GetByHash(thor.Bytes32{0xfe}))

	assert.True(t, coordinator.Remove(native.Hash(), native.ID()))
	assert.Nil(t, coordinator.GetByHash(native.Hash()))
	assert.False(t, coordinator.Remove(ethereum.Hash(), native.ID()))
	assert.Same(t, ethereum, coordinator.GetByHash(ethereum.Hash()))
	assert.True(t, coordinator.Remove(ethereum.Hash(), ethereum.ID()))
	assert.Nil(t, coordinator.GetByHash(ethereum.Hash()))
	assert.False(t, coordinator.Remove(thor.Bytes32{0xfd}, thor.Bytes32{0xfc}))
	assert.Zero(t, coordinator.Len())
}

func TestCoordinatorAddRemoteRejectsNil(t *testing.T) {
	coordinator, _ := newCoordinatorTest(t)
	defer coordinator.Close()
	events := make(chan *TxEvent, 1)
	sub := coordinator.SubscribeTxEvent(events)
	defer sub.Unsubscribe()

	err := coordinator.AddRemote(nil)

	require.Error(t, err)
	assert.True(t, IsBadTx(err))
	assert.False(t, IsTxRejected(err))
	assert.Zero(t, coordinator.Len())
	select {
	case event := <-events:
		t.Fatalf("nil admission emitted event: %#v", event)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestCoordinatorFillUsesVeChainPoolOnly(t *testing.T) {
	coordinator, tchain := newCoordinatorTest(t)
	defer coordinator.Close()

	native := coordinatorNativeTx(t, tchain, 3)
	ethereum := coordinatorEthTx(t, tchain, 4, 0)

	coordinator.Fill(tx.Transactions{native, ethereum})

	assert.Equal(t, 2, coordinator.vechain.Len())
	assert.Zero(t, coordinator.eth.Len())
	assert.Same(t, native, coordinator.vechain.Get(native.ID()))
	assert.Same(t, ethereum, coordinator.vechain.Get(ethereum.ID()),
		"the temporary Fill contract does not partition transaction families")
	assert.Nil(t, coordinator.eth.GetByHash(ethereum.Hash()))
	assert.Equal(t, 2, coordinator.Len())
}

func TestCoordinatorIncludedOnlyFork(t *testing.T) {
	t.Run("Ethereum inclusion reconciles nonce", func(t *testing.T) {
		coordinator, tchain := newCoordinatorTest(t)
		defer coordinator.Close()

		ethereum := coordinatorEthTx(t, tchain, 5, 0)
		require.NoError(t, coordinator.AddRemote(ethereum))
		require.NoError(t, tchain.MintBlock(ethereum))

		require.NoError(t, coordinator.ReinjectFromFork(ForkReinjection{
			Included: tx.Transactions{ethereum},
		}))

		assert.Nil(t, coordinator.eth.GetByHash(ethereum.Hash()))
		assert.Equal(t, uint64(1), coordinator.PoolNonce(devAccounts[5].Address))
	})

	t.Run("VeChain inclusion is intentionally ignored", func(t *testing.T) {
		coordinator, tchain := newCoordinatorTest(t)
		defer coordinator.Close()

		native := coordinatorNativeTx(t, tchain, 6)
		require.NoError(t, coordinator.AddRemote(native))

		require.NoError(t, coordinator.ReinjectFromFork(ForkReinjection{
			Included: tx.Transactions{native},
		}))

		assert.Same(t, native, coordinator.vechain.Get(native.ID()),
			"included-only VeChain forks are not reconciled by the coordinator")
	})
}

func TestCoordinatorCloseUnblocksInFlightEventRelay(t *testing.T) {
	coordinator, tchain := newCoordinatorTest(t)

	events := make(chan *TxEvent, 2)
	eventSub := coordinator.SubscribeTxEvent(events)
	blockedSub := coordinator.SubscribeTxEvent(make(chan *TxEvent))

	native := coordinatorNativeTx(t, tchain, 7)
	ethereum := coordinatorEthTx(t, tchain, 8, 0)
	require.NoError(t, coordinator.AddRemote(native))
	require.NoError(t, coordinator.AddRemote(ethereum))
	firstRelayed := nextTxEvent(t, events).Tx.Hash()
	assert.Contains(t, []thor.Bytes32{native.Hash(), ethereum.Hash()}, firstRelayed,
		"receiving proves a dual-family relay send reached the blocked subscriber")

	closed := make(chan struct{})
	go func() {
		coordinator.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("coordinator Close hung with an in-flight event relay")
	}

	for name, done := range map[string]<-chan struct{}{
		"coordinator": coordinator.ctx.Done(),
		"VeChainPool": coordinator.vechain.ctx.Done(),
		"EthPool":     coordinator.eth.ctx.Done(),
	} {
		select {
		case <-done:
		default:
			t.Errorf("%s context was not cancelled", name)
		}
	}
	for name, errCh := range map[string]<-chan error{
		"event subscriber":   eventSub.Err(),
		"blocked subscriber": blockedSub.Err(),
	} {
		select {
		case <-errCh:
		default:
			t.Errorf("%s was not closed", name)
		}
	}
}
