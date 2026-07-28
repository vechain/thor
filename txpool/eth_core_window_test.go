// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

// seedEthWindowSender places pending and queued objects for one signer, returning
// them keyed by nonce so assertions can name them.
func seedEthWindowSender(
	t *testing.T,
	m *ethPoolCore,
	signer int,
	stateNonce uint64,
	pendingNonces, queuedNonces []uint64,
) (thor.Address, map[uint64]*TxObject) {
	t.Helper()

	objects := make(map[uint64]*TxObject, len(pendingNonces)+len(queuedNonces))
	var origin thor.Address
	sender := (*ethSender)(nil)
	place := func(nonce uint64, pending bool) {
		txObj := newEthCoreTestObject(t, nonce, 10, signer)
		if sender == nil {
			origin = txObj.Origin()
			sender = newEthSender(origin, stateNonce)
			m.senders[origin] = sender
		}
		objects[nonce] = txObj
		m.allByHash[txObj.Hash()] = txObj
		if pending {
			txObj.executable = true
			sender.pending[nonce] = txObj
			return
		}
		sender.queue[nonce] = txObj
	}
	for _, nonce := range pendingNonces {
		place(nonce, true)
	}
	for _, nonce := range queuedNonces {
		place(nonce, false)
	}
	return origin, objects
}

func TestSenderWindowSpan(t *testing.T) {
	sender := newEthSender(thor.Address{0x01}, 0)
	sender.pending[0] = &TxObject{}
	sender.pending[1] = &TxObject{}
	sender.pending[2] = &TxObject{}

	assert.Equal(t, 5, senderWindowSpan(sender, 5), "promotion room exceeds the pending run")
	assert.Equal(t, 3, senderWindowSpan(sender, 2), "pending run exceeds the promotion room")
	assert.Equal(t, 3, senderWindowSpan(sender, 0), "a full pool still revalidates its pending run")
	assert.Equal(t, 4, senderWindowSpan(nil, 4), "an unknown sender can fill its whole allowance")
	assert.Equal(t, 0, senderWindowSpan(nil, -1), "a negative limit means no capacity, not unlimited")
}

func TestPreparationWindowCoversPendingAndPromotionRoom(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	// Pending 0-1, queued 2-3 contiguous, plus a far-future 9.
	origin, objects := seedEthWindowSender(t, m, 0, 0, []uint64{0, 1}, []uint64{2, 3, 9})

	window := m.preparationWindow(map[thor.Address]uint64{origin: 0}, 4)

	assert.Equal(t, []*TxObject{objects[0], objects[1], objects[2], objects[3]}, window,
		"window spans max(pendingLimit, len(pending)) nonces from stateNonce")
}

func TestPreparationWindowFollowsStateNonce(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	origin, objects := seedEthWindowSender(t, m, 0, 0, []uint64{0, 1, 2}, []uint64{3, 4})

	// The head moved two nonces on: 0 and 1 are settled and cannot be revalidated,
	// but the pending run keeps the span wide enough to reach the queue.
	window := m.preparationWindow(map[thor.Address]uint64{origin: 2}, 1)

	assert.Equal(t, []*TxObject{objects[2], objects[3], objects[4]}, window)
}

func TestPreparationWindowSkipsUnknownAndAbsentSenders(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	origin, objects := seedEthWindowSender(t, m, 0, 0, []uint64{0}, nil)
	// A second sender in the pool but absent from the state snapshot is untouched
	// by the commit, so it must not be prepared.
	seedEthWindowSender(t, m, 1, 0, []uint64{0}, nil)

	window := m.preparationWindow(map[thor.Address]uint64{origin: 0, {0xff}: 3}, 1)

	assert.Equal(t, []*TxObject{objects[0]}, window)
}

func TestAddPreparationWindow(t *testing.T) {
	t.Run("candidate inside the window is prepared", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		origin, objects := seedEthWindowSender(t, m, 0, 0, []uint64{0}, []uint64{1})
		candidate := newEthCoreTestObject(t, 2, 10, 0)

		window := m.addPreparationWindow(candidate, 0, 4)

		assert.Equal(t, []*TxObject{objects[0], objects[1], candidate}, window)
		assert.Equal(t, origin, candidate.Origin())
	})

	t.Run("candidate above the window is not prepared", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		_, objects := seedEthWindowSender(t, m, 0, 0, []uint64{0}, nil)
		// Nonce 4 cannot be promoted while the pending limit is 2, so its cost
		// never needs checking on this path.
		candidate := newEthCoreTestObject(t, 4, 10, 0)

		window := m.addPreparationWindow(candidate, 0, 2)

		assert.Equal(t, []*TxObject{objects[0]}, window)
	})

	t.Run("candidate below the state nonce is not prepared", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		seedEthWindowSender(t, m, 0, 3, []uint64{3}, nil)
		candidate := newEthCoreTestObject(t, 1, 10, 0)

		assert.NotContains(t, m.addPreparationWindow(candidate, 3, 4), candidate)
	})

	t.Run("unknown sender prepares only the candidate", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		candidate := newEthCoreTestObject(t, 0, 10, 0)

		assert.Equal(t, []*TxObject{candidate}, m.addPreparationWindow(candidate, 0, 4))
	})

	t.Run("already known transaction prepares nothing", func(t *testing.T) {
		m := newEthPoolCore(newCostTracker())
		_, objects := seedEthWindowSender(t, m, 0, 0, []uint64{0}, nil)

		assert.Nil(t, m.addPreparationWindow(objects[0], 0, 4))
	})
}

func TestForkPreparationWindow(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	resetOrigin, resetObjects := seedEthWindowSender(t, m, 0, 0, []uint64{0, 1}, nil)
	// A candidate whose origin is absent from the state snapshot still gets a
	// window, derived from the nonce carried on the candidate.
	uncoveredOrigin, uncoveredObjects := seedEthWindowSender(t, m, 1, 0, []uint64{0}, nil)
	candidate := newEthCoreTestObject(t, 1, 20, 1)

	window := m.forkPreparationWindow(
		[]ethForkCandidate{{txObj: candidate, stateNonce: 0}},
		map[thor.Address]uint64{resetOrigin: 0},
		2,
	)

	assert.Equal(t, []*TxObject{
		resetObjects[0], resetObjects[1],
		candidate,
		uncoveredObjects[0],
	}, window)
	assert.Equal(t, uncoveredOrigin, candidate.Origin())
}
