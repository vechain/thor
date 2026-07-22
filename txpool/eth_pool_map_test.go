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
