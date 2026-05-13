// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func TestEthPoolMapAddIndexesAndDeduplicates(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[0]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)

	replaced, err := m.add(tx0, 0)
	require.NoError(t, err)
	assert.Nil(t, replaced)
	assert.True(t, m.containsHash(tx0.Hash()))
	assert.Equal(t, tx0.ID(), m.getByHash(tx0.Hash()).ID())
	assert.Equal(t, 1, m.len())
	assert.Equal(t, uint64(1), m.poolNonce(sender.Address, 0))

	replaced, err = m.add(tx0, 0)
	require.NoError(t, err)
	assert.Nil(t, replaced)
	assert.Equal(t, 1, m.len(), "duplicate hash must be a no-op")
}

func TestEthPoolMapReplacementUpdatesHashIndex(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[1]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)
	original := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	replacement := ethTxObjectForSenderTest(t, tchain, sender, 0, 2*base, 3*base)

	_, err := m.add(original, 0)
	require.NoError(t, err)
	replaced, err := m.add(replacement, 0)
	require.NoError(t, err)

	require.NotNil(t, replaced)
	assert.Equal(t, original.ID(), replaced.ID())
	assert.Nil(t, m.getByHash(original.Hash()))
	assert.Equal(t, replacement.ID(), m.getByHash(replacement.Hash()).ID())
	assert.Equal(t, 1, m.len())
}

func TestEthPoolMapEnforcesTotalLimit(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	senderA := genesis.DevAccounts()[0]
	senderB := genesis.DevAccounts()[1]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMapWithLimit(1, 4)

	original := ethTxObjectForSenderTest(t, tchain, senderA, 0, base, 2*base)
	_, err := m.add(original, 0)
	require.NoError(t, err)

	_, err = m.add(ethTxObjectForSenderTest(t, tchain, senderB, 0, base, 2*base), 0)
	require.ErrorContains(t, err, "pool is full")
	assert.Equal(t, 1, m.len())

	replacement := ethTxObjectForSenderTest(t, tchain, senderA, 0, 2*base, 3*base)
	replaced, err := m.add(replacement, 0)
	require.NoError(t, err)
	require.NotNil(t, replaced)
	assert.Equal(t, original.ID(), replaced.ID())
	assert.Equal(t, replacement.ID(), m.getByHash(replacement.Hash()).ID())
	assert.Equal(t, 1, m.len())
}

func TestEthPoolMapStressManySendersStopsAtTotalLimit(t *testing.T) {
	const limit = 256

	m := newEthPoolMapWithLimit(limit, 1)
	for sender := range limit {
		_, err := m.add(synthEthPoolTxObj(sender, 0), 0)
		require.NoError(t, err)
	}
	_, err := m.add(synthEthPoolTxObj(limit, 0), 0)
	require.ErrorContains(t, err, "pool is full")

	m.lock.RLock()
	assert.Equal(t, limit, len(m.allByHash))
	assert.Equal(t, limit, len(m.senders))
	m.lock.RUnlock()
	assert.Equal(t, limit, m.len())
}

func TestEthPoolMapEnforcesAccountQuotaAcrossPendingAndQueue(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[2]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(2)

	_, err := m.add(ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base), 0)
	require.NoError(t, err)
	_, err = m.add(ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base), 0)
	require.NoError(t, err)

	_, err = m.add(ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base), 0)
	require.ErrorContains(t, err, "account quota exceeded")

	_, err = m.add(ethTxObjectForSenderTest(t, tchain, sender, 3, base, 2*base), 0)
	require.ErrorContains(t, err, "account quota exceeded")
}

func TestEthPoolMapAccountQuotaRejectsNewSenderWithoutSideEffects(t *testing.T) {
	m := newEthPoolMap(0)

	_, err := m.add(synthEthPoolTxObj(0, 0), 0)
	require.ErrorContains(t, err, "account quota exceeded")

	m.lock.RLock()
	assert.Empty(t, m.senders)
	m.lock.RUnlock()
	assert.Equal(t, 0, m.len())
}

func TestEthPoolMapStressQuotaRejectsOverflowButAllowsReplacement(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[2]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMapWithLimit(8, 4)

	original := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	queuedOriginal := ethTxObjectForSenderTest(t, tchain, sender, 3, base, 2*base)
	for _, txObj := range []*TxObject{
		original,
		ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base),
		queuedOriginal,
		ethTxObjectForSenderTest(t, tchain, sender, 4, base, 2*base),
	} {
		_, err := m.add(txObj, 0)
		require.NoError(t, err)
	}

	_, err := m.add(ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base), 0)
	require.ErrorContains(t, err, "account quota exceeded")
	_, err = m.add(ethTxObjectForSenderTest(t, tchain, sender, 5, base, 2*base), 0)
	require.ErrorContains(t, err, "account quota exceeded")

	replacement := ethTxObjectForSenderTest(t, tchain, sender, 0, 2*base, 3*base)
	replaced, err := m.add(replacement, 0)
	require.NoError(t, err)
	require.NotNil(t, replaced)
	assert.Equal(t, original.ID(), replaced.ID())
	assert.Equal(t, 4, m.len())
	assert.Nil(t, m.getByHash(original.Hash()))
	assert.Equal(t, replacement.ID(), m.getByHash(replacement.Hash()).ID())

	queuedReplacement := ethTxObjectForSenderTest(t, tchain, sender, 3, 2*base, 3*base)
	replaced, err = m.add(queuedReplacement, 0)
	require.NoError(t, err)
	require.NotNil(t, replaced)
	assert.Equal(t, queuedOriginal.ID(), replaced.ID())
	assert.Equal(t, 4, m.len())
	assert.Nil(t, m.getByHash(queuedOriginal.Hash()))
	assert.Equal(t, queuedReplacement.ID(), m.getByHash(queuedReplacement.Hash()).ID())
}

func TestEthPoolMapRemoveDemotesTrailingPending(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[3]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	tx1 := ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base)
	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)

	for _, txObj := range []*TxObject{tx0, tx1, tx2} {
		_, err := m.add(txObj, 0)
		require.NoError(t, err)
	}
	assert.True(t, m.removeByHash(tx0.Hash()))
	assert.False(t, m.containsHash(tx0.Hash()))
	assert.Equal(t, 2, m.len())

	_, pendingGroups := m.snapshot()
	assert.Empty(t, pendingGroups, "removing nonce 0 breaks contiguity and demotes later nonces")
	assert.Equal(t, uint64(0), m.poolNonce(sender.Address, 0))

	tx0Again := ethTxObjectForSenderTest(t, tchain, sender, 0, 2*base, 3*base)
	_, err := m.add(tx0Again, 0)
	require.NoError(t, err)
	_, pendingGroups = m.snapshot()
	require.Len(t, pendingGroups, 1)
	require.Len(t, pendingGroups[0], 3)
	assert.Equal(t, tx0Again.ID(), pendingGroups[0][0].ID())
	assert.Equal(t, tx1.ID(), pendingGroups[0][1].ID())
	assert.Equal(t, tx2.ID(), pendingGroups[0][2].ID())
}

func TestEthPoolMapBumpStateNoncePrunesHashIndex(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[4]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)

	_, err := m.add(tx0, 0)
	require.NoError(t, err)
	_, err = m.add(tx2, 0)
	require.NoError(t, err)

	evicted := m.bumpStateNonce(sender.Address, 2)
	require.Len(t, evicted, 1)
	assert.Equal(t, tx0.ID(), evicted[0].ID())
	assert.Nil(t, m.getByHash(tx0.Hash()))
	assert.NotNil(t, m.getByHash(tx2.Hash()))
	assert.Equal(t, uint64(3), m.poolNonce(sender.Address, 0))
}

// ---------------------------------------------------------------------------
// ensureSender tests — exercised indirectly through add() and poolNonce().
// ---------------------------------------------------------------------------

func TestEnsureSender_CreatesNewSenderAtChainNonce(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[0]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	// add() calls ensureSender; for a never-seen address it creates an
	// ethSender seeded at chainNonce.
	tx5 := ethTxObjectForSenderTest(t, tchain, sender, 5, base, 2*base)
	_, err := m.add(tx5, 5)
	require.NoError(t, err)

	m.lock.RLock()
	s := m.senders[sender.Address]
	m.lock.RUnlock()
	require.NotNil(t, s, "sender entry must be created")
	assert.Equal(t, uint64(5), s.stateNonce)
	assert.Len(t, s.pending, 1)
	assert.Empty(t, s.queue)
}

func TestPoolNonce_UnknownSenderReturnsChainNonce(t *testing.T) {
	m := newEthPoolMap(4)
	addr := thor.BytesToAddress([]byte{0x01})

	// poolNonce for a never-seen address returns chainNonce without
	// creating a sender entry.
	assert.Equal(t, uint64(5), m.poolNonce(addr, 5))

	m.lock.RLock()
	s := m.senders[addr]
	m.lock.RUnlock()
	assert.Nil(t, s, "poolNonce must not create a sender entry")
}

func TestEnsureSender_ChainNonceBehindIsNoop(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[5]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	// Seed sender with nonces 0 and 1 pending.
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	tx1 := ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)
	_, err = m.add(tx1, 0)
	require.NoError(t, err)

	// Advance stateNonce to 1 (evicts tx0, tx1 remains pending).
	m.bumpStateNonce(sender.Address, 1)

	// Now call add with chainNonce=0 (behind stateNonce=1) — ensureSender
	// must NOT regress stateNonce.
	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)
	_, err = m.add(tx2, 0)
	require.NoError(t, err)

	m.lock.RLock()
	s := m.senders[sender.Address]
	m.lock.RUnlock()
	require.NotNil(t, s)
	assert.Equal(t, uint64(1), s.stateNonce, "stateNonce must not regress when chainNonce is behind")
}

func TestEnsureSender_ChainNonceEqualIsNoop(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[6]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), m.poolNonce(sender.Address, 0))

	// Call with chainNonce == stateNonce (both 0 at seed, but now pending
	// advances poolNonce to 1). Passing chainNonce=0 should not evict the
	// pending tx.
	assert.Equal(t, uint64(1), m.poolNonce(sender.Address, 0))
	assert.Equal(t, 1, m.len(), "pending tx must survive when chainNonce == stateNonce")
}

func TestEnsureSender_ChainNonceAheadBumpsWithoutEviction(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[2]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	// Seed sender at stateNonce 0 via add.
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)

	// Bump via bumpStateNonce to 1 (evicts tx0).
	m.bumpStateNonce(sender.Address, 1)

	// Now add a new tx with chainNonce=5 — ensureSender sees stateNonce=1
	// (sender was re-created by add) and bumps to 5 with nothing to evict.
	// First, re-create the sender via add with chainNonce=5.
	tx5 := ethTxObjectForSenderTest(t, tchain, sender, 5, base, 2*base)
	_, err = m.add(tx5, 5)
	require.NoError(t, err)

	m.lock.RLock()
	s := m.senders[sender.Address]
	m.lock.RUnlock()
	require.NotNil(t, s)
	assert.Equal(t, uint64(5), s.stateNonce)
	assert.Len(t, s.pending, 1)
}

func TestEnsureSender_ChainNonceAheadEvictsPendingAndPurgesHash(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[7]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	tx1 := ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)
	_, err = m.add(tx1, 0)
	require.NoError(t, err)
	require.Equal(t, 2, m.len())

	// Chain advanced past nonce 0 (chainNonce=1): ensureSender inside the
	// next add() evicts tx0 from both the sender's pending map and allByHash.
	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)
	_, err = m.add(tx2, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, m.len(), "tx0 evicted, tx1+tx2 remain")
	assert.Nil(t, m.getByHash(tx0.Hash()), "tx0 must be purged from allByHash")
	assert.NotNil(t, m.getByHash(tx1.Hash()), "tx1 must survive")
	assert.NotNil(t, m.getByHash(tx2.Hash()), "tx2 must be added")
}

func TestEnsureSender_ChainNonceAheadEvictsAndPromotesQueue(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[8]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	// Add nonce 0 (pending) and nonce 2 (queue, gap at 1).
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)
	_, err = m.add(tx2, 0)
	require.NoError(t, err)

	// Verify initial state: only nonce 0 is executable.
	_, groups := m.snapshot()
	require.Len(t, groups, 1)
	require.Len(t, groups[0], 1)
	assert.Equal(t, tx0.ID(), groups[0][0].ID())

	// bumpStateNonce to 2: evicts tx0 from pending, bumps stateNonce to 2,
	// promotes tx2 from queue into pending.
	evicted := m.bumpStateNonce(sender.Address, 2)
	require.Len(t, evicted, 1)
	assert.Equal(t, tx0.ID(), evicted[0].ID())
	assert.Equal(t, 1, m.len(), "only tx2 should remain")
	assert.Nil(t, m.getByHash(tx0.Hash()), "tx0 must be purged")
	assert.NotNil(t, m.getByHash(tx2.Hash()), "tx2 must survive and be promoted")

	_, groups = m.snapshot()
	require.Len(t, groups, 1, "tx2 must now be in a pending group")
	require.Len(t, groups[0], 1)
	assert.Equal(t, tx2.ID(), groups[0][0].ID())
}

func TestEnsureSender_IdempotentOnRepeatedBump(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[9]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)

	// First bump evicts tx0, sender becomes empty and is GC'd.
	evicted := m.bumpStateNonce(sender.Address, 1)
	require.Len(t, evicted, 1)
	assert.Equal(t, 0, m.len())

	m.lock.RLock()
	s := m.senders[sender.Address]
	m.lock.RUnlock()
	assert.Nil(t, s, "empty sender must be GC'd")

	// Second bump for a now-absent sender is a no-op.
	evicted2 := m.bumpStateNonce(sender.Address, 1)
	assert.Empty(t, evicted2)

	// poolNonce still returns the correct chain nonce.
	assert.Equal(t, uint64(1), m.poolNonce(sender.Address, 1))
}

func TestEnsureSender_ViaAddSeedsNewSender(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[0]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	// add() with chainNonce=5 for a new sender must seed stateNonce at 5.
	// A tx at nonce 5 should go straight into pending.
	tx5 := ethTxObjectForSenderTest(t, tchain, sender, 5, base, 2*base)
	_, err := m.add(tx5, 5)
	require.NoError(t, err)

	assert.Equal(t, uint64(6), m.poolNonce(sender.Address, 5),
		"nonce 5 is pending → next expected is 6")

	// A tx at nonce 3 (below stateNonce=5) must be rejected as stale.
	tx3 := ethTxObjectForSenderTest(t, tchain, sender, 3, 2*base, 3*base)
	_, err = m.add(tx3, 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fee bump insufficient",
		"stale nonce below stateNonce is rejected by place()")
}

func TestEnsureSender_ViaAddBumpsExistingSender(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[1]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	// Seed sender at stateNonce 0 with nonce 0 pending.
	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), m.poolNonce(sender.Address, 0))

	// Second add with chainNonce=1 (chain included nonce 0 since last call).
	// ensureSender must evict tx0 before placing the new tx.
	tx1 := ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base)
	_, err = m.add(tx1, 1)
	require.NoError(t, err)
	assert.Nil(t, m.getByHash(tx0.Hash()), "tx0 must be evicted by ensureSender bump")
	assert.NotNil(t, m.getByHash(tx1.Hash()))
	assert.Equal(t, uint64(2), m.poolNonce(sender.Address, 1))
}

// ---------------------------------------------------------------------------
// Sender GC tests — verify the senders map doesn't grow unboundedly.
// ---------------------------------------------------------------------------

func TestGC_BumpStateNonceCleansUpEmptySender(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[3]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)

	// All txs included → sender becomes empty after bump.
	m.bumpStateNonce(sender.Address, 1)

	m.lock.RLock()
	_, exists := m.senders[sender.Address]
	m.lock.RUnlock()
	assert.False(t, exists,
		"empty sender with advanced stateNonce must be GC'd")

	// Subsequent poolNonce returns the correct chain nonce.
	assert.Equal(t, uint64(1), m.poolNonce(sender.Address, 1))

	// Subsequent add re-creates the sender at the new chainNonce.
	tx1 := ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base)
	_, err = m.add(tx1, 1)
	require.NoError(t, err)

	m.lock.RLock()
	s := m.senders[sender.Address]
	m.lock.RUnlock()
	require.NotNil(t, s)
	assert.Equal(t, uint64(1), s.stateNonce)
	assert.Len(t, s.pending, 1)
}

func TestGC_RemoveByHashCleansUpEmptySender(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[4]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	_, err := m.add(tx0, 0)
	require.NoError(t, err)

	// Externally remove the only tx.
	m.removeByHash(tx0.Hash())

	m.lock.RLock()
	_, exists := m.senders[sender.Address]
	m.lock.RUnlock()
	assert.False(t, exists, "sender with no txs must be GC'd after removeByHash")
}

func TestGC_BumpOnUnknownSenderIsNoop(t *testing.T) {
	m := newEthPoolMap(4)
	addr := thor.BytesToAddress([]byte{0xAA})

	// Bumping a sender that was never seen must NOT create an entry.
	evicted := m.bumpStateNonce(addr, 5)
	assert.Empty(t, evicted)

	m.lock.RLock()
	_, exists := m.senders[addr]
	m.lock.RUnlock()
	assert.False(t, exists, "bumpStateNonce must not create entries for unknown senders")
}

func TestGC_MultipleSendersAreCollectedIndependently(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	senderA := genesis.DevAccounts()[5]
	senderB := genesis.DevAccounts()[6]
	base := int64(thor.InitialBaseFee)
	m := newEthPoolMap(4)

	txA := ethTxObjectForSenderTest(t, tchain, senderA, 0, base, 2*base)
	txB := ethTxObjectForSenderTest(t, tchain, senderB, 0, base, 2*base)
	_, err := m.add(txA, 0)
	require.NoError(t, err)
	_, err = m.add(txB, 0)
	require.NoError(t, err)

	m.lock.RLock()
	assert.Equal(t, 2, len(m.senders))
	m.lock.RUnlock()

	// Include only senderA's tx.
	m.bumpStateNonce(senderA.Address, 1)

	m.lock.RLock()
	_, aExists := m.senders[senderA.Address]
	_, bExists := m.senders[senderB.Address]
	m.lock.RUnlock()
	assert.False(t, aExists, "senderA is empty and must be GC'd")
	assert.True(t, bExists, "senderB still has pending tx and must remain")
	assert.Equal(t, 1, m.len(), "only senderB's tx remains")
}
