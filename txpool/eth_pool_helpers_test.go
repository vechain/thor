// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"bytes"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/tx"
)

func ethPoolTestOptions() Options {
	return Options{
		Limit:           100,
		EthAccountSlots: 16,
		EthAccountQueue: 64,
		EthPriceBump:    10,
		LimitPerAccount: 100,
		MaxLifetime:     time.Hour,
	}
}

func TestEthAdmissionContextStateNonceCache(t *testing.T) {
	pool, _ := newEthPoolTest(t, ethPoolTestOptions())
	ctx := pool.newAdmissionContext()
	origin := devAccounts[0].Address

	nonce, err := ctx.stateNonce(origin)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), nonce)

	// A cached value is returned without another state lookup.
	ctx.stateNonces[origin] = 42
	nonce, err = ctx.stateNonce(origin)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), nonce)
}

func TestCollectIncludedForkNonces(t *testing.T) {
	pool, tchain := newEthPoolTest(t, ethPoolTestOptions())
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	ethTx := buildEthPoolTx(
		t,
		tchain.Repo().ChainID(),
		0,
		new(big.Int).Mul(baseFee, big.NewInt(2)),
		big.NewInt(1),
		devAccounts[0],
	)
	nativeTx := tx.MustSign(tx.NewBuilder(tx.TypeLegacy).
		ChainTag(tchain.Repo().ChainTag()).
		Gas(21_000).
		Clause(tx.NewClause(&devAccounts[1].Address)).
		Build(), devAccounts[1].PrivateKey)

	t.Run("collects Ethereum origins and ignores other entries", func(t *testing.T) {
		ctx := pool.newAdmissionContext()
		err := pool.collectIncludedForkNonces(ctx, tx.Transactions{nil, nativeTx, ethTx})

		require.NoError(t, err)
		assert.Equal(t, uint64(0), ctx.stateNonces[devAccounts[0].Address])
		assert.NotContains(t, ctx.stateNonces, devAccounts[1].Address)
	})

	t.Run("rejects an included transaction with invalid signature", func(t *testing.T) {
		ctx := pool.newAdmissionContext()
		invalid := ethTx.WithSignature(make([]byte, 65))

		err := pool.collectIncludedForkNonces(ctx, tx.Transactions{invalid})

		require.Error(t, err)
		assert.Empty(t, ctx.stateNonces)
	})
}

func TestCollectForkCandidates(t *testing.T) {
	pool, tchain := newEthPoolTest(t, ethPoolTestOptions())
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))

	t.Run("collects valid transactions and caches sender nonce", func(t *testing.T) {
		ctx := pool.newAdmissionContext()
		valid := buildEthPoolTx(t, tchain.Repo().ChainID(), 2, fee, big.NewInt(1), devAccounts[2])

		candidates, err := pool.collectForkCandidates(ctx, tx.Transactions{nil, valid})

		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Same(t, valid, candidates[0].txObj.Transaction)
		assert.Equal(t, uint64(0), candidates[0].stateNonce)
		assert.Equal(t, uint64(0), ctx.stateNonces[devAccounts[2].Address])
	})

	t.Run("skips malformed and wrong-chain policy rejections", func(t *testing.T) {
		ctx := pool.newAdmissionContext()
		malformed := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(1), devAccounts[3]).
			WithSignature(make([]byte, 65))
		wrongChain := buildEthPoolTx(t, tchain.Repo().ChainID()+1, 0, fee, big.NewInt(1), devAccounts[4])

		candidates, err := pool.collectForkCandidates(ctx, tx.Transactions{malformed, wrongChain})

		require.NoError(t, err)
		assert.Empty(t, candidates)
	})

	t.Run("skips a duplicate but still records its origin for reset", func(t *testing.T) {
		duplicate := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(1), devAccounts[5])
		require.NoError(t, pool.AddRemote(duplicate))
		ctx := pool.newAdmissionContext()

		candidates, err := pool.collectForkCandidates(ctx, tx.Transactions{duplicate})

		require.NoError(t, err)
		assert.Empty(t, candidates)
		assert.Contains(t, ctx.stateNonces, devAccounts[5].Address)
	})
}

func TestSortEthForkCandidates(t *testing.T) {
	_, tchain := newEthPoolTest(t, ethPoolTestOptions())
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))

	makeCandidate := func(nonce uint64, signer int) ethForkCandidate {
		trx := buildEthPoolTx(t, tchain.Repo().ChainID(), nonce, fee, big.NewInt(1), devAccounts[signer])
		txObj, err := ResolveTx(trx, false)
		require.NoError(t, err)
		return ethForkCandidate{txObj: txObj}
	}
	candidates := []ethForkCandidate{
		makeCandidate(2, 6),
		makeCandidate(1, 7),
		makeCandidate(0, 6),
	}

	sortEthForkCandidates(candidates)

	for i := 1; i < len(candidates); i++ {
		prevOrigin := candidates[i-1].txObj.Origin()
		currentOrigin := candidates[i].txObj.Origin()
		addressOrder := bytes.Compare(prevOrigin[:], currentOrigin[:])
		assert.LessOrEqual(t, addressOrder, 0)
		if addressOrder == 0 {
			assert.LessOrEqual(t, candidates[i-1].txObj.Nonce(), candidates[i].txObj.Nonce())
		}
	}
}

func TestEmitForkResults(t *testing.T) {
	pool, tchain := newEthPoolTest(t, ethPoolTestOptions())
	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	fee := new(big.Int).Mul(baseFee, big.NewInt(2))
	admittedTx := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(1), devAccounts[8])
	rejectedTx := buildEthPoolTx(t, tchain.Repo().ChainID(), 0, fee, big.NewInt(1), devAccounts[9])
	promotedTx := buildEthPoolTx(t, tchain.Repo().ChainID(), 1, fee, big.NewInt(1), devAccounts[8])
	admittedObj, err := ResolveTx(admittedTx, false)
	require.NoError(t, err)
	rejectedObj, err := ResolveTx(rejectedTx, false)
	require.NoError(t, err)
	promotedObj, err := ResolveTx(promotedTx, false)
	require.NoError(t, err)

	events := make(chan *TxEvent, 3)
	sub := pool.SubscribeTxEvent(events)
	defer sub.Unsubscribe()
	pool.emitForkResults([]ethForkResult{
		{txObj: rejectedObj, err: errors.New("policy rejection")},
		{txObj: admittedObj, executable: false, promoted: []*TxObject{promotedObj}},
	})

	event := nextTxEvent(t, events)
	assert.Equal(t, admittedTx.Hash(), event.Tx.Hash())
	require.NotNil(t, event.Executable)
	assert.False(t, *event.Executable)

	event = nextTxEvent(t, events)
	assert.Equal(t, promotedTx.Hash(), event.Tx.Hash())
	require.NotNil(t, event.Executable)
	assert.True(t, *event.Executable)
	select {
	case unexpected := <-events:
		t.Fatalf("rejected result emitted event for %v", unexpected.Tx.ID())
	case <-time.After(20 * time.Millisecond):
	}
}
