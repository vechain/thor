// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newEthMapTestObject(t *testing.T, nonce uint64, fee int64, signer int) *TxObject {
	t.Helper()
	to := devAccounts[(signer+1)%len(devAccounts)].Address
	trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(1).
		Nonce(nonce).
		Gas(21_000).
		MaxFeePerGas(big.NewInt(fee)).
		MaxPriorityFeePerGas(big.NewInt(1)).
		To(&to).
		Build(), devAccounts[signer].PrivateKey)
	txObj, err := ResolveTx(trx, false)
	require.NoError(t, err)
	return txObj
}

func fixedEthPrepare(cost, balance int64) ethPrepare {
	return func(txObj *TxObject) (reservationRequest, bool, error) {
		payer := txObj.Origin()
		return reservationRequest{
			owner:   ethReservationOwner(txObj.Origin(), txObj.Nonce()),
			payer:   payer,
			cost:    big.NewInt(cost),
			balance: big.NewInt(balance),
		}, true, nil
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

func TestExecutableHashesLocked(t *testing.T) {
	m := newEthPoolMap(newCostTracker())
	executable := newEthMapTestObject(t, 0, 10, 0)
	queued := newEthMapTestObject(t, 1, 10, 0)
	executable.executable = true
	sender := newEthSender(executable.Origin(), 0)
	sender.pending[0] = executable
	sender.queue[1] = queued
	m.senders[executable.Origin()] = sender

	m.lock.Lock()
	hashes := m.executableHashesLocked([]thor.Address{executable.Origin(), {0xff}})
	m.lock.Unlock()

	assert.Contains(t, hashes, executable.Hash())
	assert.NotContains(t, hashes, queued.Hash())
}

func TestEthExecutableSnapshot(t *testing.T) {
	m := newEthPoolMap(newCostTracker())
	first := newEthMapTestObject(t, 5, 10, 1)
	second := newEthMapTestObject(t, 6, 10, 1)
	queued := newEthMapTestObject(t, 7, 10, 1)
	other := newEthMapTestObject(t, 0, 10, 2)

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
	require.Len(t, snapshot.groups, 2)
	assert.Equal(t, 3, snapshot.total)

	expected := [][]*tx.Transaction{{first.Transaction, second.Transaction}, {other.Transaction}}
	if bytes.Compare(firstOrigin[:], otherOrigin[:]) > 0 {
		expected[0], expected[1] = expected[1], expected[0]
	}
	for i, group := range snapshot.groups {
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
	for _, group := range snapshot.groups {
		for _, entry := range group {
			if entry.tx == first.Transaction {
				firstEntry = entry
			}
		}
	}
	assert.Equal(t, int64(10), firstEntry.priorityGasPrice.Int64())
	for _, group := range snapshot.groups {
		for _, entry := range group {
			assert.NotSame(t, queued.Transaction, entry.tx)
		}
	}
}

func TestEthPoolMapRemoveByHash(t *testing.T) {
	t.Run("pending removal demotes suffix and releases reservations", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolMap(costs)
		tx0 := newEthMapTestObject(t, 0, 10, 3)
		tx1 := newEthMapTestObject(t, 1, 10, 3)
		tx2 := newEthMapTestObject(t, 2, 10, 3)
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
		m := newEthPoolMap(costs)
		queued := newEthMapTestObject(t, 2, 10, 4)
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
		m := newEthPoolMap(newCostTracker())
		txObj := newEthMapTestObject(t, 0, 10, 5)
		m.allByHash[txObj.Hash()] = txObj

		assert.False(t, m.removeByHash(txObj.Hash()))
		assert.Same(t, txObj, m.GetByHash(txObj.Hash()))
	})
}

func TestEthPoolMapToTxsIncludesPendingAndQueued(t *testing.T) {
	m := newEthPoolMap(newCostTracker())
	pending := newEthMapTestObject(t, 0, 10, 6)
	queued := newEthMapTestObject(t, 2, 10, 6)
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

	empty := newEthPoolMap(newCostTracker()).ToTxs()
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestEthPoolMapSyncHead(t *testing.T) {
	t.Run("settles mined nonce, preserves suffix, and promotes queue", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolMap(costs)
		tx0 := newEthMapTestObject(t, 0, 10, 0)
		tx1 := newEthMapTestObject(t, 1, 10, 0)
		tx2 := newEthMapTestObject(t, 2, 10, 0)
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
		m := newEthPoolMap(costs)
		txObj := newEthMapTestObject(t, 0, 10, 1)
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
		m := newEthPoolMap(costs)
		txObj := newEthMapTestObject(t, 0, 10, 2)
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

func TestResetForkSendersLocked(t *testing.T) {
	t.Run("settles old nonce and releases all affected reservations", func(t *testing.T) {
		costs := newCostTracker()
		m := newEthPoolMap(costs)
		settled := newEthMapTestObject(t, 0, 10, 1)
		retained := newEthMapTestObject(t, 1, 10, 1)
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
		m := newEthPoolMap(costs)
		txObj := newEthMapTestObject(t, 0, 10, 2)
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
		m := newEthPoolMap(newCostTracker())
		txObj := newEthMapTestObject(t, 0, 10, 3)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[0] = txObj
		m.senders[origin] = sender
		m.allByHash[txObj.Hash()] = txObj

		m.lock.Lock()
		results, err := m.promoteForkSendersLocked(
			[]thor.Address{origin}, nil, 16, fixedEthPrepare(10, 100),
		)
		m.lock.Unlock()

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Same(t, txObj, results[0].txObj)
		assert.True(t, results[0].executable)
		assert.Same(t, txObj, sender.pending[0])
	})

	t.Run("returns a fatal invalid reservation error", func(t *testing.T) {
		m := newEthPoolMap(newCostTracker())
		txObj := newEthMapTestObject(t, 0, 10, 4)
		origin := txObj.Origin()
		sender := newEthSender(origin, 0)
		sender.queue[0] = txObj
		m.senders[origin] = sender
		invalidPrepare := func(*TxObject) (reservationRequest, bool, error) {
			return reservationRequest{}, true, nil
		}

		m.lock.Lock()
		results, err := m.promoteForkSendersLocked([]thor.Address{origin}, nil, 16, invalidPrepare)
		m.lock.Unlock()

		assert.Nil(t, results)
		assert.ErrorIs(t, err, errInvalidCost)
	})
}

func TestAddForkCandidatesLocked(t *testing.T) {
	t.Run("adds a valid candidate", func(t *testing.T) {
		m := newEthPoolMap(newCostTracker())
		txObj := newEthMapTestObject(t, 0, 10, 5)

		m.lock.Lock()
		results, err := m.addForkCandidatesLocked(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			nil,
			100,
			16,
			64,
			10,
			fixedEthPrepare(10, 100),
		)
		m.lock.Unlock()

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.NoError(t, results[0].err)
		assert.True(t, results[0].executable)
		assert.NotNil(t, m.GetByHash(txObj.Hash()))
	})

	t.Run("records policy rejection without aborting the batch", func(t *testing.T) {
		m := newEthPoolMap(newCostTracker())
		txObj := newEthMapTestObject(t, 0, 10, 6)
		m.allByHash[txObj.Hash()] = txObj

		m.lock.Lock()
		results, err := m.addForkCandidatesLocked(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			nil,
			100,
			16,
			64,
			10,
			fixedEthPrepare(10, 100),
		)
		m.lock.Unlock()

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.ErrorIs(t, results[0].err, errEthAlreadyKnown)
	})

	t.Run("aborts on fatal cost corruption", func(t *testing.T) {
		m := newEthPoolMap(newCostTracker())
		txObj := newEthMapTestObject(t, 0, 10, 7)
		invalidPrepare := func(*TxObject) (reservationRequest, bool, error) {
			return reservationRequest{}, true, nil
		}

		m.lock.Lock()
		results, err := m.addForkCandidatesLocked(
			[]ethForkCandidate{{txObj: txObj, stateNonce: 0}},
			nil,
			100,
			16,
			64,
			10,
			invalidPrepare,
		)
		m.lock.Unlock()

		assert.Nil(t, results)
		assert.ErrorIs(t, err, errInvalidCost)
	})
}

func TestFilterNewPromotions(t *testing.T) {
	oldTx := newEthMapTestObject(t, 0, 10, 8)
	newTx := newEthMapTestObject(t, 1, 10, 8)

	filtered := filterNewPromotions(
		[]*TxObject{oldTx, newTx},
		map[thor.Bytes32]struct{}{oldTx.Hash(): {}},
	)
	assert.Equal(t, []*TxObject{newTx}, filtered)
	assert.Empty(t, filterNewPromotions(nil, nil))
}

func TestPruneForkSendersLocked(t *testing.T) {
	m := newEthPoolMap(newCostTracker())
	affected := thor.Address{0x01}
	candidate := newEthMapTestObject(t, 0, 10, 9)
	untouched := thor.Address{0x03}
	m.senders[affected] = newEthSender(affected, 0)
	m.senders[candidate.Origin()] = newEthSender(candidate.Origin(), 0)
	untouchedSender := newEthSender(untouched, 0)
	untouchedSender.queue[2] = newEthMapTestObject(t, 2, 10, 0)
	m.senders[untouched] = untouchedSender

	m.lock.Lock()
	m.pruneForkSendersLocked(
		[]thor.Address{affected, {0xff}},
		[]ethForkCandidate{{txObj: candidate}},
	)
	m.lock.Unlock()

	assert.NotContains(t, m.senders, affected)
	assert.NotContains(t, m.senders, candidate.Origin())
	assert.Contains(t, m.senders, untouched)
}

func TestPromoteForkSendersLockedPrepareFailureKeepsQueued(t *testing.T) {
	m := newEthPoolMap(newCostTracker())
	txObj := newEthMapTestObject(t, 0, 10, 0)
	origin := txObj.Origin()
	sender := newEthSender(origin, 0)
	sender.queue[0] = txObj
	m.senders[origin] = sender
	prepareErr := errors.New("state unavailable")
	prepare := func(*TxObject) (reservationRequest, bool, error) {
		return reservationRequest{}, false, prepareErr
	}

	m.lock.Lock()
	results, err := m.promoteForkSendersLocked([]thor.Address{origin}, nil, 16, prepare)
	m.lock.Unlock()

	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Same(t, txObj, sender.queue[0])
	assert.Empty(t, sender.pending)
}
