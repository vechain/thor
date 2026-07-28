// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"bytes"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newEthCoreTestObject(t *testing.T, nonce uint64, fee int64, signer int) *TxObject {
	return newEthCoreTestObjectWithTip(t, nonce, fee, 1, signer)
}

func newEthCoreTestObjectWithTip(t *testing.T, nonce uint64, fee, tip int64, signer int) *TxObject {
	t.Helper()
	to := devAccounts[(signer+1)%len(devAccounts)].Address
	trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(1).
		Nonce(nonce).
		Gas(21_000).
		MaxFeePerGas(big.NewInt(fee)).
		MaxPriorityFeePerGas(big.NewInt(tip)).
		To(&to).
		Build(), devAccounts[signer].PrivateKey)
	txObj, err := ResolveTx(trx, false)
	require.NoError(t, err)
	return txObj
}

func TestEthPoolCoreQueuedReplacementAtGlobalLimit(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	incumbent := newEthCoreTestObjectWithTip(t, 2, 10, 1, 0)
	underpriced := newEthCoreTestObjectWithTip(t, 2, 10, 2, 0)
	replacement := newEthCoreTestObjectWithTip(t, 2, 11, 2, 0)
	newNonce := newEthCoreTestObjectWithTip(t, 3, 12, 2, 0)

	executable, promoted, err := m.add(incumbent, 0, 1, 16, 64, 10, fixedEthPrepare(1, 100))
	require.NoError(t, err)
	assert.False(t, executable)
	assert.Empty(t, promoted)
	assert.Equal(t, 1, m.Len())

	_, _, err = m.add(underpriced, 0, 1, 16, 64, 10, fixedEthPrepare(1, 100))
	require.ErrorIs(t, err, errEthReplaceUnderpriced)
	assert.Same(t, incumbent, m.GetByHash(incumbent.Hash()))
	assert.Nil(t, m.GetByHash(underpriced.Hash()))

	_, _, err = m.add(newNonce, 0, 1, 16, 64, 10, fixedEthPrepare(1, 100))
	require.EqualError(t, err, "pool is full")
	assert.Same(t, incumbent, m.GetByHash(incumbent.Hash()))

	executable, promoted, err = m.add(replacement, 0, 1, 16, 64, 10, fixedEthPrepare(1, 100))
	require.NoError(t, err)
	assert.False(t, executable)
	assert.Empty(t, promoted)
	assert.Equal(t, 1, m.Len())
	assert.Nil(t, m.GetByHash(incumbent.Hash()))
	assert.Same(t, replacement, m.GetByHash(replacement.Hash()))

	m.lock.RLock()
	defer m.lock.RUnlock()
	sender := m.senders[replacement.Origin()]
	require.NotNil(t, sender)
	assert.Same(t, replacement, sender.queue[2])
	assert.Empty(t, sender.pending)
}

func TestEthPoolCoreConcurrentAddRemoveAndSnapshot(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	txObjs := make([]*TxObject, 32)
	for nonce := range txObjs {
		txObjs[nonce] = newEthCoreTestObjectWithTip(t, uint64(nonce), 100, 10, 1)
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		for _, txObj := range txObjs {
			_, _, _ = m.add(txObj, 0, 100, 32, 64, 10, fixedEthPrepare(1, 1_000))
		}
	})
	wg.Go(func() {
		for range 4 {
			for _, txObj := range txObjs {
				m.removeByHash(txObj.Hash())
			}
		}
	})
	wg.Go(func() {
		for range 128 {
			snapshot := m.executableSnapshot()
			assert.LessOrEqual(t, len(snapshot.transactions()), len(txObjs))
		}
	})
	wg.Wait()

	assert.Equal(t, m.Len(), len(m.ToTxs()))
}

func TestEthPoolCorePrepareRunsOutsideWriteLock(t *testing.T) {
	assertUnlockedPrepare := func(t *testing.T, m *ethPoolCore) ethPrepare {
		t.Helper()
		delegate := fixedEthPrepare(1, 1_000)
		return func(txObj *TxObject) ethPreparation {
			if !m.lock.TryRLock() {
				t.Fatal("prepare called while ethPoolCore write lock is held")
			}
			m.lock.RUnlock()
			return delegate(txObj)
		}
	}

	t.Run("add", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 100, 0)
		_, _, err := m.add(txObj, 0, 100, 16, 64, 10, assertUnlockedPrepare(t, m))
		require.NoError(t, err)
	})

	t.Run("sync head", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 100, 1)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj

		_, err := m.syncHead(
			map[thor.Address]uint64{origin: 0},
			16,
			assertUnlockedPrepare(t, m),
		)
		require.NoError(t, err)
	})

	t.Run("revalidate", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 100, 2)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj

		_, _, err := m.revalidate(
			map[thor.Address]uint64{origin: 0},
			16,
			assertUnlockedPrepare(t, m),
		)
		require.NoError(t, err)
	})

	t.Run("fork", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 100, 3)
		_, err := m.reconcileFork(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			map[thor.Address]uint64{txObj.Origin(): 0},
			100,
			16,
			64,
			10,
			assertUnlockedPrepare(t, m),
		)
		require.NoError(t, err)
	})
}

func TestEthPoolCoreReadersProceedDuringPrepare(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	txObj := newEthCoreTestObject(t, 0, 100, 0)
	prepareEntered := make(chan struct{})
	releasePrepare := make(chan struct{})
	addDone := make(chan error, 1)
	delegate := fixedEthPrepare(1, 1_000)

	go func() {
		_, _, err := m.add(
			txObj,
			0,
			100,
			16,
			64,
			10,
			func(txObj *TxObject) ethPreparation {
				close(prepareEntered)
				<-releasePrepare
				return delegate(txObj)
			},
		)
		addDone <- err
	}()

	<-prepareEntered
	readDone := make(chan struct{})
	go func() {
		assert.Zero(t, m.Len())
		assert.Nil(t, m.GetByHash(txObj.Hash()))
		assert.Empty(t, m.executableSnapshot())
		close(readDone)
	}()

	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("map readers blocked while transaction preparation was running")
	}

	close(releasePrepare)
	require.NoError(t, <-addDone)
	assert.Same(t, txObj, m.GetByHash(txObj.Hash()))
}

func TestEthPoolCoreConcurrentAddWashAndSnapshot(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	const senderCount = 8
	stateNonces := make(map[thor.Address]uint64, senderCount)
	txObjs := make([]*TxObject, senderCount*8)
	for senderIndex := range senderCount {
		for nonce := range 8 {
			txObj := newEthCoreTestObjectWithTip(t, uint64(nonce), 100, 10, senderIndex)
			txObjs[senderIndex*8+nonce] = txObj
			stateNonces[txObj.Origin()] = 0
		}
	}
	options := ethSweepOptions{
		pendingLimit: 16,
		queueLimit:   64,
		globalLimit:  len(txObjs),
	}
	prepare := fixedEthPrepare(1, 1_000_000)

	var wg sync.WaitGroup
	wg.Go(func() {
		for _, txObj := range txObjs {
			_, _, _ = m.add(txObj, 0, len(txObjs), 16, 64, 10, prepare)
		}
	})
	wg.Go(func() {
		for range 16 {
			_, _ = m.sweep(options)
			_, _, _ = m.revalidate(stateNonces, 16, prepare)
		}
	})
	wg.Go(func() {
		for range 256 {
			_ = m.executableSnapshot()
			_ = m.Len()
		}
	})
	wg.Wait()

	assert.Equal(t, m.Len(), len(m.ToTxs()))
}

func TestEthPoolCorePruneEmptySenders(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	emptyOrigin := thor.Address{0xa1}
	liveOrigin := thor.Address{0xa2}
	m.senders[emptyOrigin] = newEthSender(emptyOrigin, 0)
	live := newEthSender(liveOrigin, 0)
	live.queue[1] = feeTx(10, 1)
	m.senders[liveOrigin] = live

	m.pruneEmptySenders()

	assert.NotContains(t, m.senders, emptyOrigin)
	assert.Same(t, live, m.senders[liveOrigin])
}

func fixedEthPrepare(cost, balance int64) ethPrepare {
	return func(txObj *TxObject) ethPreparation {
		payer := txObj.Origin()
		return ethPreparation{
			request: reservationRequest{
				owner:   ethReservationOwner(txObj.Origin(), txObj.Nonce()),
				payer:   payer,
				cost:    big.NewInt(cost),
				balance: big.NewInt(balance),
			},
			viable: true,
		}
	}
}

func TestSortedEthOrigins(t *testing.T) {
	a := thor.Address{0x03}
	b := thor.Address{0x01}
	c := thor.Address{0x02}

	assert.Equal(t, []thor.Address{b, c, a}, sortedEthOrigins(map[thor.Address]uint64{
		a: 3,
		b: 1,
		c: 2,
	}))
	assert.Empty(t, sortedEthOrigins(nil))
}

func TestExecutableObjectsLockedScope(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	executable := newEthCoreTestObject(t, 0, 10, 0)
	queued := newEthCoreTestObject(t, 1, 10, 0)
	executable.executable = true
	sender := newEthSender(executable.Origin(), 0)
	sender.pending[0] = executable
	sender.queue[1] = queued
	m.senders[executable.Origin()] = sender

	// A second sender outside the requested scope must not be snapshotted.
	unscoped := newEthCoreTestObject(t, 0, 10, 1)
	unscoped.executable = true
	unscopedSender := newEthSender(unscoped.Origin(), 0)
	unscopedSender.pending[0] = unscoped
	m.senders[unscoped.Origin()] = unscopedSender

	m.lock.Lock()
	objects := m.executableObjectsLocked([]thor.Address{executable.Origin(), {0xff}})
	m.lock.Unlock()

	assert.Contains(t, objects, executable.Hash())
	assert.NotContains(t, objects, queued.Hash())
	assert.NotContains(t, objects, unscoped.Hash())
}

func TestEthExecutableSnapshot(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	first := newEthCoreTestObject(t, 5, 10, 1)
	second := newEthCoreTestObject(t, 6, 10, 1)
	queued := newEthCoreTestObject(t, 7, 10, 1)
	other := newEthCoreTestObject(t, 0, 10, 2)

	first.priorityGasPrice, second.priorityGasPrice = big.NewInt(10), big.NewInt(20)
	other.priorityGasPrice = big.NewInt(30)
	first.executable, second.executable, other.executable = true, true, true

	firstOrigin, otherOrigin := first.Origin(), other.Origin()
	firstSender := newEthSender(firstOrigin, 5)
	firstSender.pending[5], firstSender.pending[6] = first, second
	firstSender.queue[7] = queued
	otherSender := newEthSender(otherOrigin, 0)
	otherSender.pending[0] = other
	m.senders[firstOrigin], m.senders[otherOrigin] = firstSender, otherSender

	snapshot := m.executableSnapshot()
	require.Len(t, snapshot, 2)
	assert.Len(t, snapshot.transactions(), 3)

	expected := [][]*tx.Transaction{{first.Transaction, second.Transaction}, {other.Transaction}}
	if bytes.Compare(firstOrigin[:], otherOrigin[:]) > 0 {
		expected[0], expected[1] = expected[1], expected[0]
	}
	for i, group := range snapshot {
		actual := make([]*tx.Transaction, 0, len(group))
		for _, entry := range group {
			actual = append(actual, entry.tx)
		}
		assert.Equal(t, expected[i], actual)
	}

	// The snapshot owns its slices and ordering keys after the map changes.
	delete(firstSender.pending, 5)
	first.priorityGasPrice = big.NewInt(99)
	var firstEntry executableTx
	for _, group := range snapshot {
		for _, entry := range group {
			if entry.tx == first.Transaction {
				firstEntry = entry
			}
		}
	}
	assert.Equal(t, int64(10), firstEntry.priorityGasPrice.Int64())
	for _, group := range snapshot {
		for _, entry := range group {
			assert.NotSame(t, queued.Transaction, entry.tx)
		}
	}
}

func TestEthPoolCoreRemoveByHash(t *testing.T) {
	t.Run("pending removal demotes suffix and releases reservations", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		tx0 := newEthCoreTestObject(t, 0, 10, 3)
		tx1 := newEthCoreTestObject(t, 1, 10, 3)
		tx2 := newEthCoreTestObject(t, 2, 10, 3)
		origin := tx0.Origin()
		sender := newEthSender(origin, 0)

		for nonce, txObj := range []*TxObject{tx0, tx1, tx2} {
			txObj.executable = true
			sender.pending[uint64(nonce)] = txObj
			m.allByHash[txObj.Hash()] = txObj
			require.NoError(t, costs.reserve(
				ethReservationOwner(origin, uint64(nonce)),
				origin,
				big.NewInt(10),
				big.NewInt(100),
			))
		}
		m.senders[origin] = sender

		assert.True(t, m.removeByHash(tx1.Hash()))
		assert.False(t, m.removeByHash(tx1.Hash()), "removal must be idempotent")
		assert.Same(t, tx0, sender.pending[0])
		assert.Nil(t, sender.pending[1])
		assert.Nil(t, sender.pending[2])
		assert.Same(t, tx2, sender.queue[2])
		assert.False(t, tx1.executable)
		assert.False(t, tx2.executable)
		assert.Nil(t, m.GetByHash(tx1.Hash()))
		assert.NotNil(t, m.GetByHash(tx2.Hash()))
		assert.Equal(t, uint64(1), sender.poolNonce())
		assert.Equal(t, int64(10), costs.pendingCost(origin).Int64())
	})

	t.Run("queued removal deletes empty sender without releasing costs", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		queued := newEthCoreTestObject(t, 2, 10, 4)
		origin := queued.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[2] = queued
		m.senders[origin] = sender
		m.allByHash[queued.Hash()] = queued

		assert.True(t, m.removeByHash(queued.Hash()))
		assert.Nil(t, m.GetByHash(queued.Hash()))
		assert.NotContains(t, m.senders, origin)
		assert.Zero(t, costs.pendingCost(origin).Sign())
	})

	t.Run("inconsistent index is not partially removed", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 10, 5)
		m.allByHash[txObj.Hash()] = txObj

		assert.False(t, m.removeByHash(txObj.Hash()))
		assert.Same(t, txObj, m.GetByHash(txObj.Hash()))
	})
}

func TestEthPoolCoreToTxsIncludesPendingAndQueued(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	pending := newEthCoreTestObject(t, 0, 10, 6)
	queued := newEthCoreTestObject(t, 2, 10, 6)
	origin := pending.Origin()
	sender := newEthSender(origin, 0)
	sender.pending[0] = pending
	sender.queue[2] = queued
	m.senders[origin] = sender
	m.allByHash[pending.Hash()] = pending
	m.allByHash[queued.Hash()] = queued

	dump := m.ToTxs()
	require.Len(t, dump, 2)
	assert.ElementsMatch(t, tx.Transactions{pending.Transaction, queued.Transaction}, dump)

	empty := newEthPoolCore(newCostTracker()).ToTxs()
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestEthPoolCoreSyncHead(t *testing.T) {
	t.Run("settles mined nonce, preserves suffix, and promotes queue", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		tx0 := newEthCoreTestObject(t, 0, 10, 0)
		tx1 := newEthCoreTestObject(t, 1, 10, 0)
		tx2 := newEthCoreTestObject(t, 2, 10, 0)
		origin := tx0.Origin()
		sender := newEthSender(origin, 0)
		tx0.executable, tx1.executable = true, true
		sender.pending[0], sender.pending[1] = tx0, tx1
		sender.queue[2] = tx2
		m.senders[origin] = sender
		m.allByHash[tx0.Hash()] = tx0
		m.allByHash[tx1.Hash()] = tx1
		m.allByHash[tx2.Hash()] = tx2
		require.NoError(t, costs.reserve(
			ethReservationOwner(origin, 0), origin, big.NewInt(10), big.NewInt(100),
		))
		require.NoError(t, costs.reserve(
			ethReservationOwner(origin, 1), origin, big.NewInt(10), big.NewInt(100),
		))

		promoted, err := m.syncHead(
			map[thor.Address]uint64{origin: 1},
			16,
			fixedEthPrepare(10, 100),
		)
		require.NoError(t, err)
		assert.Equal(t, []*TxObject{tx2}, promoted)
		assert.Nil(t, m.GetByHash(tx0.Hash()))
		assert.Same(t, tx1, sender.pending[1])
		assert.Same(t, tx2, sender.pending[2])
		assert.Empty(t, sender.queue)
		assert.Equal(t, uint64(3), sender.poolNonce())
		assert.Equal(t, int64(20), costs.pendingCost(origin).Int64())

		promoted, err = m.syncHead(
			map[thor.Address]uint64{origin: 1},
			16,
			fixedEthPrepare(10, 100),
		)
		require.NoError(t, err)
		assert.Empty(t, promoted)
		assert.Equal(t, int64(20), costs.pendingCost(origin).Int64())
	})

	t.Run("prunes sender after its final nonce settles", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		txObj := newEthCoreTestObject(t, 0, 10, 1)
		origin := txObj.Origin()
		txObj.executable = true
		sender := newEthSender(origin, 0)
		sender.pending[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj
		require.NoError(t, costs.reserve(
			ethReservationOwner(origin, 0), origin, big.NewInt(10), big.NewInt(100),
		))

		promoted, err := m.syncHead(
			map[thor.Address]uint64{origin: 1},
			16,
			fixedEthPrepare(10, 100),
		)
		require.NoError(t, err)
		assert.Empty(t, promoted)
		assert.NotContains(t, m.senders, origin)
		assert.Zero(t, m.Len())
		assert.Zero(t, costs.pendingCost(origin).Sign())
	})

	t.Run("cost error leaves nonce state retryable", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		txObj := newEthCoreTestObject(t, 0, 10, 2)
		origin := txObj.Origin()
		txObj.executable = true
		sender := newEthSender(origin, 0)
		sender.pending[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj
		owner := ethReservationOwner(origin, 0)
		costs.reservations[owner] = reservation{payer: origin, cost: big.NewInt(10)}
		costs.pending[origin] = big.NewInt(5)

		promoted, err := m.syncHead(
			map[thor.Address]uint64{origin: 1},
			16,
			fixedEthPrepare(10, 100),
		)
		assert.ErrorIs(t, err, errCostTrackerState)
		assert.Nil(t, promoted)
		assert.Equal(t, uint64(0), sender.stateNonce)
		assert.Same(t, txObj, sender.pending[0])
		assert.Same(t, txObj, m.GetByHash(txObj.Hash()))

		costs.pending[origin] = big.NewInt(10)
		promoted, err = m.syncHead(
			map[thor.Address]uint64{origin: 1},
			16,
			fixedEthPrepare(10, 100),
		)
		require.NoError(t, err)
		assert.Empty(t, promoted)
		assert.Zero(t, m.Len())
	})
}

func TestEthPoolCoreSweep(t *testing.T) {
	t.Run("expires pending and queued transactions", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		expired := newEthCoreTestObject(t, 0, 10, 3)
		retained := newEthCoreTestObject(t, 1, 10, 3)
		expiredQueued := newEthCoreTestObject(t, 3, 10, 3)
		origin := expired.Origin()
		sender := newEthSender(origin, 0)
		expired.executable, retained.executable = true, true
		sender.pending[0], sender.pending[1] = expired, retained
		sender.queue[3] = expiredQueued
		m.senders[origin] = sender
		for _, txObj := range []*TxObject{expired, retained, expiredQueued} {
			m.allByHash[txObj.Hash()] = txObj
		}
		require.NoError(t, costs.reserve(
			ethReservationOwner(origin, 0), origin, big.NewInt(10), big.NewInt(100),
		))
		require.NoError(t, costs.reserve(
			ethReservationOwner(origin, 1), origin, big.NewInt(10), big.NewInt(100),
		))
		now := time.Now().UnixNano()
		expired.timeAdded = now - int64(2*time.Hour)
		expiredQueued.timeAdded = expired.timeAdded

		result, err := m.sweep(ethSweepOptions{
			now: now, maxLifetime: time.Hour,
			pendingLimit: 16, queueLimit: 64, globalLimit: 100,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, result.removed)
		assert.Nil(t, m.GetByHash(expired.Hash()))
		assert.Nil(t, m.GetByHash(expiredQueued.Hash()))
		assert.Same(t, retained, sender.queue[1])
		assert.False(t, retained.executable)
		assert.Equal(t, []*TxObject{retained}, result.demoted)
		assert.Zero(t, costs.pendingCost(origin).Sign())
	})

	t.Run("enforces account limits without nonce holes", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		origin := devAccounts[6].Address
		sender := newEthSender(origin, 0)
		for nonce := range uint64(3) {
			txObj := newEthCoreTestObject(t, nonce, 10, 6)
			txObj.executable = true
			sender.pending[nonce] = txObj
			m.allByHash[txObj.Hash()] = txObj
			require.NoError(t, costs.reserve(
				ethReservationOwner(origin, nonce), origin, big.NewInt(10), big.NewInt(100),
			))
		}
		queued := newEthCoreTestObject(t, 3, 10, 6)
		sender.queue[3] = queued
		m.allByHash[queued.Hash()] = queued
		m.senders[origin] = sender

		result, err := m.sweep(ethSweepOptions{pendingLimit: 2, queueLimit: 1, globalLimit: 100})
		require.NoError(t, err)
		assert.Equal(t, 1, result.removed)
		assert.Len(t, sender.pending, 2)
		assert.NotNil(t, sender.pending[0])
		assert.NotNil(t, sender.pending[1])
		assert.Len(t, sender.queue, 1)
		assert.NotNil(t, sender.queue[2])
		assert.Nil(t, sender.queue[3])
		assert.Equal(t, int64(20), costs.pendingCost(origin).Int64())
	})

	t.Run("global trimming removes queues before pending tails", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		for signer := 7; signer <= 8; signer++ {
			pending := newEthCoreTestObject(t, 0, 10, signer)
			queue1 := newEthCoreTestObject(t, 1, 10, signer)
			queue2 := newEthCoreTestObject(t, 2, 10, signer)
			origin := pending.Origin()
			pending.executable = true
			sender := newEthSender(origin, 0)
			sender.pending[0] = pending
			sender.queue[1], sender.queue[2] = queue1, queue2
			m.senders[origin] = sender
			for _, txObj := range []*TxObject{pending, queue1, queue2} {
				m.allByHash[txObj.Hash()] = txObj
			}
			require.NoError(t, costs.reserve(
				ethReservationOwner(origin, 0), origin, big.NewInt(10), big.NewInt(100),
			))
		}

		result, err := m.sweep(ethSweepOptions{pendingLimit: 16, queueLimit: 64, globalLimit: 1})
		require.NoError(t, err)
		assert.Equal(t, 5, result.removed)
		assert.Equal(t, 1, m.Len())
		for _, sender := range m.senders {
			assert.Empty(t, sender.queue)
			assert.Len(t, sender.pending, 1)
		}
	})

	// Sweeping never reads chain state, so it must be safe to run without any
	// state nonces or preparation at all.
	t.Run("leaves a healthy pool untouched", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		pending := newEthCoreTestObject(t, 0, 10, 0)
		queued := newEthCoreTestObject(t, 1, 10, 0)
		origin := pending.Origin()
		pending.executable = true
		sender := newEthSender(origin, 0)
		sender.pending[0], sender.queue[1] = pending, queued
		m.senders[origin] = sender
		m.allByHash[pending.Hash()], m.allByHash[queued.Hash()] = pending, queued

		result, err := m.sweep(ethSweepOptions{pendingLimit: 16, queueLimit: 64, globalLimit: 100})
		require.NoError(t, err)
		assert.Zero(t, result.removed)
		assert.Empty(t, result.demoted)
		assert.Same(t, pending, sender.pending[0])
		assert.Same(t, queued, sender.queue[1])
	})
}

func TestEthPoolCoreRevalidate(t *testing.T) {
	t.Run("keeps only affordable pending prefix", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		origin := devAccounts[4].Address
		sender := newEthSender(origin, 0)
		for nonce := range uint64(3) {
			txObj := newEthCoreTestObject(t, nonce, 10, 4)
			txObj.executable = true
			sender.pending[nonce] = txObj
			m.allByHash[txObj.Hash()] = txObj
			require.NoError(t, costs.reserve(
				ethReservationOwner(origin, nonce), origin, big.NewInt(10), big.NewInt(100),
			))
		}
		m.senders[origin] = sender

		promoted, _, err := m.revalidate(
			map[thor.Address]uint64{origin: 0},
			16,
			fixedEthPrepare(10, 15),
		)
		require.NoError(t, err)
		assert.Empty(t, promoted)
		assert.Len(t, sender.pending, 1)
		assert.Len(t, sender.queue, 2)
		assert.NotNil(t, sender.pending[0])
		assert.NotNil(t, sender.queue[1])
		assert.NotNil(t, sender.queue[2])
		assert.Equal(t, int64(10), costs.pendingCost(origin).Int64())
	})

	t.Run("demotes non-viable pending suffix", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		origin := devAccounts[5].Address
		sender := newEthSender(origin, 0)
		var pending []*TxObject
		for nonce := range uint64(2) {
			txObj := newEthCoreTestObject(t, nonce, 10, 5)
			txObj.executable = true
			pending = append(pending, txObj)
			sender.pending[nonce] = txObj
			m.allByHash[txObj.Hash()] = txObj
			require.NoError(t, costs.reserve(
				ethReservationOwner(origin, nonce), origin, big.NewInt(10), big.NewInt(100),
			))
		}
		m.senders[origin] = sender
		notViable := func(*TxObject) ethPreparation {
			return ethPreparation{}
		}

		promoted, demoted, err := m.revalidate(
			map[thor.Address]uint64{origin: 0},
			16,
			notViable,
		)
		require.NoError(t, err)
		assert.Empty(t, promoted)
		assert.Equal(t, pending, demoted)
		assert.Empty(t, sender.pending)
		assert.Len(t, sender.queue, 2)
		assert.Zero(t, costs.pendingCost(origin).Sign())
	})

	t.Run("promotes a newly affordable queue prefix", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		tx0 := newEthCoreTestObject(t, 0, 10, 5)
		tx1 := newEthCoreTestObject(t, 1, 10, 5)
		origin := tx0.Origin()
		tx0.executable = true
		sender := newEthSender(origin, 0)
		sender.pending[0], sender.queue[1] = tx0, tx1
		m.senders[origin] = sender
		m.allByHash[tx0.Hash()], m.allByHash[tx1.Hash()] = tx0, tx1
		require.NoError(t, costs.reserve(
			ethReservationOwner(origin, 0), origin, big.NewInt(10), big.NewInt(100),
		))

		promoted, _, err := m.revalidate(
			map[thor.Address]uint64{origin: 0},
			16,
			fixedEthPrepare(10, 100),
		)
		require.NoError(t, err)
		assert.Equal(t, []*TxObject{tx1}, promoted)
		assert.Same(t, tx1, sender.pending[1])
		assert.Empty(t, sender.queue)
		assert.Equal(t, int64(20), costs.pendingCost(origin).Int64())
	})

	t.Run("tracker failure leaves pending state retryable", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		txObj := newEthCoreTestObject(t, 0, 10, 9)
		origin := txObj.Origin()
		txObj.executable = true
		sender := newEthSender(origin, 0)
		sender.pending[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj
		owner := ethReservationOwner(origin, 0)
		costs.reservations[owner] = reservation{payer: origin, cost: big.NewInt(10)}
		costs.pending[origin] = big.NewInt(5)

		_, _, err := m.revalidate(
			map[thor.Address]uint64{origin: 0},
			16,
			fixedEthPrepare(10, 100),
		)
		assert.ErrorIs(t, err, errCostTrackerState)
		assert.Same(t, txObj, sender.pending[0])
		assert.Same(t, txObj, m.GetByHash(txObj.Hash()))

		costs.pending[origin] = big.NewInt(10)
		_, _, err = m.revalidate(
			map[thor.Address]uint64{origin: 0},
			16,
			fixedEthPrepare(10, 100),
		)
		require.NoError(t, err)
		assert.Same(t, txObj, sender.pending[0])
	})
}

func TestEthPoolCoreGlobalLimitHelpers(t *testing.T) {
	t.Run("global enforcement skips disabled and satisfied limits", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		queued := newEthCoreTestObject(t, 1, 10, 0)
		origin := queued.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[1] = queued
		m.senders[origin] = sender
		m.allByHash[queued.Hash()] = queued
		var result ethSweepResult

		m.lock.Lock()
		errDisabled := m.enforceGlobalLimitLocked([]thor.Address{origin}, 0, &result)
		errSatisfied := m.enforceGlobalLimitLocked([]thor.Address{origin}, 1, &result)
		m.lock.Unlock()

		require.NoError(t, errDisabled)
		require.NoError(t, errSatisfied)
		assert.Zero(t, result.removed)
		assert.Same(t, queued, m.GetByHash(queued.Hash()))
	})

	t.Run("queue cursors include only queued senders in nonce order", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		queued1 := newEthCoreTestObject(t, 1, 10, 0)
		queued3 := newEthCoreTestObject(t, 3, 10, 0)
		origin := queued1.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[1], sender.queue[3] = queued1, queued3
		m.senders[origin] = sender
		emptyOrigin := devAccounts[1].Address
		m.senders[emptyOrigin] = newEthSender(emptyOrigin, 0)

		m.lock.Lock()
		cursors := m.queueEvictionCursorsLocked([]thor.Address{origin, emptyOrigin, {0xff}})
		none := m.queueEvictionCursorsLocked(nil)
		m.lock.Unlock()

		require.Len(t, cursors, 1)
		assert.Same(t, sender, cursors[0].sender)
		assert.Equal(t, []uint64{3, 1}, cursors[0].nonces)
		assert.Zero(t, cursors[0].next)
		assert.Empty(t, none)
	})

	t.Run("queued eviction is round-robin and tolerates exhausted cursors", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		var origins []thor.Address
		for signer := range 2 {
			queue1 := newEthCoreTestObject(t, 1, 10, signer)
			queue2 := newEthCoreTestObject(t, 2, 10, signer)
			origin := queue1.Origin()
			origins = append(origins, origin)
			sender := newEthSender(origin, 0)
			sender.queue[1], sender.queue[2] = queue1, queue2
			m.senders[origin] = sender
			m.allByHash[queue1.Hash()], m.allByHash[queue2.Hash()] = queue1, queue2
		}
		var result ethSweepResult
		m.lock.Lock()
		m.evictQueuedUntilLimitLocked(m.queueEvictionCursorsLocked(origins), 2, &result)
		m.lock.Unlock()

		assert.Equal(t, 2, result.removed)
		assert.Equal(t, 2, m.Len())
		for _, origin := range origins {
			assert.NotNil(t, m.senders[origin].queue[1])
			assert.Nil(t, m.senders[origin].queue[2])
		}

		// A stale cursor cannot occur while the map lock is respected, but the
		// helper still fails safely if handed one.
		orphan := newEthCoreTestObject(t, 9, 10, 2)
		m.allByHash[orphan.Hash()] = orphan
		stale := []queuedEvictionCursor{{
			sender: newEthSender(orphan.Origin(), 0),
			nonces: []uint64{9},
		}}
		m.lock.Lock()
		m.evictQueuedUntilLimitLocked(stale, 0, &result)
		m.lock.Unlock()
		assert.NotNil(t, m.GetByHash(orphan.Hash()))
	})

	t.Run("pending tail batches respect bounds and skip invalid senders", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		tx0 := newEthCoreTestObject(t, 0, 10, 3)
		tx1 := newEthCoreTestObject(t, 1, 10, 3)
		other := newEthCoreTestObject(t, 0, 10, 4)
		firstOrigin, otherOrigin := tx0.Origin(), other.Origin()
		first := newEthSender(firstOrigin, 0)
		first.pending[0], first.pending[1] = tx0, tx1
		second := newEthSender(otherOrigin, 0)
		second.pending[0] = other
		m.senders[firstOrigin], m.senders[otherOrigin] = first, second

		m.lock.Lock()
		tails, releases := m.pendingTailBatchLocked(
			[]thor.Address{firstOrigin, {0xff}, otherOrigin},
			2,
		)
		none, noReleases := m.pendingTailBatchLocked(
			[]thor.Address{firstOrigin, otherOrigin},
			0,
		)
		m.lock.Unlock()

		require.Len(t, tails, 2)
		assert.Same(t, tx1, tails[0].txObj)
		assert.Same(t, other, tails[1].txObj)
		assert.Equal(t, []reservationOwner{
			ethReservationOwner(firstOrigin, 1),
			ethReservationOwner(otherOrigin, 0),
		}, releases)
		assert.Nil(t, none)
		assert.Nil(t, noReleases)
	})

	t.Run("pending tail eviction batches releases and handles empty capacity", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		var origins []thor.Address
		for signer, count := range []int{2, 1} {
			origin := devAccounts[signer+5].Address
			origins = append(origins, origin)
			sender := newEthSender(origin, 0)
			for nonce := range count {
				txObj := newEthCoreTestObject(t, uint64(nonce), 10, signer+5)
				txObj.executable = true
				sender.pending[uint64(nonce)] = txObj
				m.allByHash[txObj.Hash()] = txObj
				require.NoError(t, costs.reserve(
					ethReservationOwner(origin, uint64(nonce)),
					origin,
					big.NewInt(10),
					big.NewInt(100),
				))
			}
			m.senders[origin] = sender
		}
		var result ethSweepResult
		m.lock.Lock()
		err := m.evictPendingTailsUntilLimitLocked(origins, 1, &result)
		m.lock.Unlock()

		require.NoError(t, err)
		assert.Equal(t, 2, result.removed)
		assert.Equal(t, 1, m.Len())
		assert.NotNil(t, m.senders[origins[0]].pending[0])
		assert.NotContains(t, m.senders[origins[0]].pending, uint64(1))
		assert.Empty(t, m.senders[origins[1]].pending)

		orphan := newEthCoreTestObject(t, 4, 10, 9)
		m.allByHash[orphan.Hash()] = orphan
		m.lock.Lock()
		err = m.evictPendingTailsUntilLimitLocked(nil, 0, &result)
		m.lock.Unlock()
		require.NoError(t, err)
		assert.NotNil(t, m.GetByHash(orphan.Hash()))
	})

	t.Run("pending tail release failure leaves maps unchanged", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		txObj := newEthCoreTestObject(t, 0, 10, 0)
		origin := txObj.Origin()
		txObj.executable = true
		sender := newEthSender(origin, 0)
		sender.pending[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj
		owner := ethReservationOwner(origin, 0)
		costs.reservations[owner] = reservation{payer: origin, cost: big.NewInt(10)}
		costs.pending[origin] = big.NewInt(5)
		var result ethSweepResult

		m.lock.Lock()
		err := m.evictPendingTailsUntilLimitLocked([]thor.Address{origin}, 0, &result)
		m.lock.Unlock()

		assert.ErrorIs(t, err, errCostTrackerState)
		assert.Zero(t, result.removed)
		assert.Same(t, txObj, sender.pending[0])
		assert.Same(t, txObj, m.GetByHash(txObj.Hash()))
	})

	t.Run("empty sender pruning ignores live and unknown origins", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		emptyOrigin := devAccounts[1].Address
		liveTx := newEthCoreTestObject(t, 0, 10, 2)
		liveOrigin := liveTx.Origin()
		m.senders[emptyOrigin] = newEthSender(emptyOrigin, 0)
		live := newEthSender(liveOrigin, 0)
		live.queue[0] = liveTx
		m.senders[liveOrigin] = live

		m.lock.Lock()
		m.pruneEmptyOriginsLocked([]thor.Address{emptyOrigin, liveOrigin, {0xff}})
		m.lock.Unlock()

		assert.NotContains(t, m.senders, emptyOrigin)
		assert.Same(t, live, m.senders[liveOrigin])
	})
}

func TestResetForkSendersLocked(t *testing.T) {
	t.Run("settles old nonce and releases all affected reservations", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		settled := newEthCoreTestObject(t, 0, 10, 1)
		retained := newEthCoreTestObject(t, 1, 10, 1)
		origin := settled.Origin()
		sender := newEthSender(origin, 0)
		sender.pending[0], sender.pending[1] = settled, retained
		settled.executable, retained.executable = true, true
		m.senders[origin] = sender
		m.allByHash[settled.Hash()], m.allByHash[retained.Hash()] = settled, retained
		require.NoError(t, costs.reserve(ethReservationOwner(origin, 0), origin, big.NewInt(10), big.NewInt(100)))
		require.NoError(t, costs.reserve(ethReservationOwner(origin, 1), origin, big.NewInt(10), big.NewInt(100)))

		m.lock.Lock()
		err := m.resetForkSendersLocked([]thor.Address{origin}, map[thor.Address]uint64{origin: 1})
		m.lock.Unlock()

		require.NoError(t, err)
		assert.Nil(t, m.GetByHash(settled.Hash()))
		assert.NotNil(t, m.GetByHash(retained.Hash()))
		assert.Same(t, retained, sender.queue[1])
		assert.False(t, retained.executable)
		assert.Zero(t, costs.pendingCost(origin).Sign())
	})

	t.Run("reports inconsistent cost tracker state", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolCore(costs)
		txObj := newEthCoreTestObject(t, 0, 10, 2)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.pending[0] = txObj
		m.senders[origin] = sender
		costs.reservations[ethReservationOwner(origin, 0)] = reservation{
			payer: origin,
			cost:  big.NewInt(10),
		}

		m.lock.Lock()
		err := m.resetForkSendersLocked([]thor.Address{origin}, map[thor.Address]uint64{origin: 0})
		m.lock.Unlock()

		assert.ErrorIs(t, err, errCostTrackerState)
	})
}

func TestPromoteForkSendersLocked(t *testing.T) {
	t.Run("promotes a newly executable queued transaction", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 10, 3)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj

		m.lock.Lock()
		results, err := m.promoteForkSendersLocked(
			[]thor.Address{origin},
			nil,
			16,
			prepareEthObjects([]*TxObject{txObj}, fixedEthPrepare(10, 100)),
		)
		m.lock.Unlock()

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Same(t, txObj, results[0].txObj)
		assert.True(t, results[0].executable)
		assert.Same(t, txObj, sender.pending[0])
	})

	t.Run("returns a fatal invalid reservation error", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 10, 4)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[0] = txObj
		m.senders[origin] = sender
		invalidPrepare := func(*TxObject) ethPreparation {
			return ethPreparation{viable: true}
		}

		m.lock.Lock()
		results, err := m.promoteForkSendersLocked(
			[]thor.Address{origin},
			nil,
			16,
			prepareEthObjects([]*TxObject{txObj}, invalidPrepare),
		)
		m.lock.Unlock()

		assert.Nil(t, results)
		assert.ErrorIs(t, err, errInvalidCost)
	})
}

func TestAddForkCandidatesLocked(t *testing.T) {
	t.Run("adds a valid candidate", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 10, 5)

		m.lock.Lock()
		results, err := m.addForkCandidatesLocked(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			nil,
			100,
			16,
			64,
			10,
			prepareEthObjects([]*TxObject{txObj}, fixedEthPrepare(10, 100)),
		)
		m.lock.Unlock()

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.NoError(t, results[0].err)
		assert.True(t, results[0].executable)
		assert.NotNil(t, m.GetByHash(txObj.Hash()))
	})

	t.Run("records policy rejection without aborting the batch", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 10, 6)
		m.allByHash[txObj.Hash()] = txObj

		m.lock.Lock()
		results, err := m.addForkCandidatesLocked(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			nil,
			100,
			16,
			64,
			10,
			prepareEthObjects([]*TxObject{txObj}, fixedEthPrepare(10, 100)),
		)
		m.lock.Unlock()

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.ErrorIs(t, results[0].err, errEthAlreadyKnown)
	})

	t.Run("aborts on fatal cost corruption", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		txObj := newEthCoreTestObject(t, 0, 10, 7)
		invalidPrepare := func(*TxObject) ethPreparation {
			return ethPreparation{viable: true}
		}

		m.lock.Lock()
		results, err := m.addForkCandidatesLocked(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			nil,
			100,
			16,
			64,
			10,
			prepareEthObjects([]*TxObject{txObj}, invalidPrepare),
		)
		m.lock.Unlock()

		assert.Nil(t, results)
		assert.ErrorIs(t, err, errInvalidCost)
	})
}

func TestNewPromotionsLocked(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	alreadyExecutable := newEthCoreTestObject(t, 0, 10, 8)
	promoted := newEthCoreTestObject(t, 1, 10, 8)
	removed := newEthCoreTestObject(t, 2, 10, 8)
	demoted := newEthCoreTestObject(t, 3, 10, 8)

	alreadyExecutable.executable = true
	promoted.executable = true
	removed.executable = true
	m.allByHash[alreadyExecutable.Hash()] = alreadyExecutable
	m.allByHash[promoted.Hash()] = promoted
	m.allByHash[demoted.Hash()] = demoted

	m.lock.Lock()
	retained := m.newPromotionsLocked(
		[]*TxObject{alreadyExecutable, promoted, removed, demoted},
		map[thor.Bytes32]*TxObject{alreadyExecutable.Hash(): alreadyExecutable},
	)
	m.lock.Unlock()

	// Already executable, gone from the pool, and still queued are all excluded.
	assert.Equal(t, []*TxObject{promoted}, retained)
	assert.Empty(t, m.newPromotionsLocked(nil, nil))
}

func TestPruneForkSendersLocked(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	affected := thor.Address{0x01}
	candidate := newEthCoreTestObject(t, 0, 10, 9)
	untouched := thor.Address{0x03}
	m.senders[affected] = newEthSender(affected, 0)
	m.senders[candidate.Origin()] = newEthSender(candidate.Origin(), 0)
	untouchedSender := newEthSender(untouched, 0)
	untouchedSender.queue[2] = newEthCoreTestObject(t, 2, 10, 0)
	m.senders[untouched] = untouchedSender

	m.lock.Lock()
	m.pruneForkSendersLocked(forkScopeOrigins(
		[]thor.Address{affected, {0xff}},
		[]ethForkCandidate{{txObj: candidate}},
	))
	m.lock.Unlock()

	assert.NotContains(t, m.senders, affected)
	assert.NotContains(t, m.senders, candidate.Origin())
	assert.Contains(t, m.senders, untouched)
}

func TestForkScopeOrigins(t *testing.T) {
	inSnapshot := thor.Address{0x01}
	candidate := newEthCoreTestObject(t, 0, 10, 0)
	alsoInSnapshot := newEthCoreTestObject(t, 0, 10, 1)

	// Candidate origins already present in the snapshot are not duplicated,
	// missing ones are appended.
	assert.Equal(t,
		[]thor.Address{inSnapshot, alsoInSnapshot.Origin(), candidate.Origin()},
		forkScopeOrigins(
			[]thor.Address{inSnapshot, alsoInSnapshot.Origin()},
			[]ethForkCandidate{
				{txObj: alsoInSnapshot},
				{txObj: candidate},
				{txObj: candidate},
			},
		),
	)
	assert.Empty(t, forkScopeOrigins(nil, nil))
}

func TestPromoteForkSendersLockedPrepareFailureKeepsQueued(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	txObj := newEthCoreTestObject(t, 0, 10, 0)
	origin := txObj.Origin()
	sender := newEthSender(origin, 0)
	sender.queue[0] = txObj
	m.senders[origin] = sender
	prepareErr := errors.New("state unavailable")
	prepare := func(*TxObject) ethPreparation {
		return ethPreparation{err: prepareErr}
	}

	m.lock.Lock()
	results, err := m.promoteForkSendersLocked(
		[]thor.Address{origin},
		nil,
		16,
		prepareEthObjects([]*TxObject{txObj}, prepare),
	)
	m.lock.Unlock()

	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Same(t, txObj, sender.queue[0])
	assert.Empty(t, sender.pending)
}
