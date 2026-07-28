// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"github.com/vechain/thor/v2/thor"
)

// The preparation window bounds the chain state a commit can need, so that state
// is fetched before the core write lock is taken rather than under it.
//
// Two facts make the bound exact without simulating any transition. Every commit
// path assigns sender.stateNonce from the same stateNonce it is handed, and
// pending is contiguous from stateNonce by invariant. So promoteLocked can only
// ever promote into [stateNonce, stateNonce+pendingLimit), and revalidation can
// only ever touch the contiguous pending run. Their union is the window.
//
// The window is an enumeration of live state, not a model of it, so unlike a
// simulation it cannot drift out of agreement with the committer. It is still
// only an optimisation: ethPreparations.get falls back to preparing inline, so a
// miss costs a state read under the lock and never a rejection.

// preparationWindow enumerates the transactions the sync and wash commits can
// need chain state for.
func (m *ethPoolCore) preparationWindow(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var objects []*TxObject
	for _, origin := range sortedEthOrigins(stateNonces) {
		objects = m.appendSenderWindowLocked(objects, origin, stateNonces[origin], pendingLimit)
	}
	return objects
}

// addPreparationWindow is the window for the one sender an add touches, plus the
// incoming transaction when its nonce falls inside it. A candidate above the
// window can only be queued by this commit, never promoted, so preparing it
// would be a wasted state read.
func (m *ethPoolCore) addPreparationWindow(
	txObj *TxObject,
	stateNonce uint64,
	pendingLimit int,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if m.allByHash[txObj.Hash()] != nil {
		return nil
	}
	origin := txObj.Origin()
	objects := m.appendSenderWindowLocked(nil, origin, stateNonce, pendingLimit)
	if offset := txObj.Nonce() - stateNonce; txObj.Nonce() >= stateNonce &&
		offset < uint64(senderWindowSpan(m.senders[origin], pendingLimit)) {
		objects = append(objects, txObj)
	}
	return objects
}

// forkPreparationWindow is the window for every sender a fork reconcile resets,
// plus the candidates being reinjected. Candidates are always prepared: they were
// canonical a moment ago, so they are the transactions most likely to promote.
func (m *ethPoolCore) forkPreparationWindow(
	candidates []ethForkCandidate,
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var objects []*TxObject
	covered := make(map[thor.Address]struct{}, len(stateNonces)+len(candidates))
	for _, origin := range sortedEthOrigins(stateNonces) {
		covered[origin] = struct{}{}
		objects = m.appendSenderWindowLocked(objects, origin, stateNonces[origin], pendingLimit)
	}
	for _, candidate := range candidates {
		objects = append(objects, candidate.txObj)
		origin := candidate.txObj.Origin()
		if _, exists := covered[origin]; exists {
			continue
		}
		covered[origin] = struct{}{}
		objects = m.appendSenderWindowLocked(objects, origin, candidate.stateNonce, pendingLimit)
	}
	return objects
}

// appendSenderWindowLocked appends one sender's window. Callers must hold m.lock.
func (m *ethPoolCore) appendSenderWindowLocked(
	objects []*TxObject,
	origin thor.Address,
	stateNonce uint64,
	pendingLimit int,
) []*TxObject {
	sender := m.senders[origin]
	if sender == nil {
		return objects
	}
	nonce := stateNonce
	for range senderWindowSpan(sender, pendingLimit) {
		if txObj := sender.get(nonce); txObj != nil {
			objects = append(objects, txObj)
		}
		nonce++
	}
	return objects
}

// senderWindowSpan is how many nonces above stateNonce a commit can reach for one
// sender: the promotion room pendingLimit allows, or the pending run to
// revalidate, whichever is longer. A sync that moves stateNonce can only shorten
// the pending run, never lengthen it, so reading its current length is safe.
func senderWindowSpan(sender *ethSender, pendingLimit int) int {
	pendingCount := 0
	if sender != nil {
		pendingCount = len(sender.pending)
	}
	return max(pendingLimit, pendingCount, 0)
}
