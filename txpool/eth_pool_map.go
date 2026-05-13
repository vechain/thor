// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"sync"

	"github.com/vechain/thor/v2/thor"
)

// ethPoolMap is the single-mutex container holding all EthPool state:
//   - senders: per-sender pending/queue + stateNonce (see ethSender).
//   - allByHash: Ethereum wire-hash index, the source of truth for dedupe
//     and for eth_getTransactionByHash lookups against pending txs.
//
// All mutating methods take the write lock; read paths (Get, PoolNonce,
// snapshot) take the read lock. EthPool talks to this map exclusively;
// higher-level invariants (head-reset, executable re-check) live in EthPool.
type ethPoolMap struct {
	lock            sync.RWMutex
	senders         map[thor.Address]*ethSender
	allByHash       map[thor.Bytes32]*TxObject
	limit           int
	limitPerAccount int
}

func newEthPoolMap(limitPerAccount int) *ethPoolMap {
	return newEthPoolMapWithLimit(int(^uint(0)>>1), limitPerAccount)
}

func newEthPoolMapWithLimit(limit, limitPerAccount int) *ethPoolMap {
	return &ethPoolMap{
		senders:         make(map[thor.Address]*ethSender),
		allByHash:       make(map[thor.Bytes32]*TxObject),
		limit:           limit,
		limitPerAccount: limitPerAccount,
	}
}

// containsHash reports whether the given Ethereum wire hash is already indexed.
func (m *ethPoolMap) containsHash(hash thor.Bytes32) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	_, ok := m.allByHash[hash]
	return ok
}

// add installs an EthereumTx into the map, applying per-account limits and
// replacement-at-slot semantics via ethSender.place. If a tx at the same
// (sender, nonce) slot was replaced, it is removed from allByHash and returned
// to the caller (for event emission).
//
// chainNonce is consulted when the sender is seen for the first time: it
// becomes the sender's initial stateNonce. The caller reads this value from
// the canonical chain state (state.GetNonce).
func (m *ethPoolMap) add(txObj *TxObject, chainNonce uint64) (replaced *TxObject, err error) {
	origin := txObj.Origin()
	hash := txObj.Hash()

	m.lock.Lock()
	defer m.lock.Unlock()

	if _, found := m.allByHash[hash]; found {
		return nil, nil
	}

	sender, senderExists := m.senders[origin]
	if senderExists && chainNonce > sender.stateNonce {
		evicted := sender.bumpStateNonce(chainNonce)
		for _, ev := range evicted {
			delete(m.allByHash, ev.Hash())
		}
		if sender.empty() {
			delete(m.senders, origin)
			senderExists = false
			sender = nil
		}
	}

	// Enforce per-account caps *excluding* same-slot replacements (a replacement
	// does not grow the footprint).
	nonce := txObj.Nonce()
	isReplacement := false
	if senderExists {
		if nonce < sender.nextPendingNonce() {
			_, isReplacement = sender.pending[nonce]
		} else if nonce > sender.nextPendingNonce() {
			_, isReplacement = sender.queue[nonce]
		}
	}
	if !isReplacement {
		if len(m.allByHash) >= m.limit {
			return nil, errors.New("pool is full")
		}
		accountTxs := 0
		if senderExists {
			accountTxs = len(sender.pending) + len(sender.queue)
		}
		if accountTxs >= m.limitPerAccount {
			return nil, errors.New("account quota exceeded")
		}
		if !senderExists {
			sender = m.ensureSender(origin, chainNonce)
		}
	}

	replaced, accepted := sender.place(txObj)
	if !accepted {
		return nil, errors.New("replacement rejected: fee bump insufficient")
	}

	if replaced != nil {
		delete(m.allByHash, replaced.Hash())
	}
	m.allByHash[hash] = txObj

	if sender.empty() {
		// Shouldn't happen on add, but keep the invariant tight.
		delete(m.senders, origin)
	}
	return replaced, nil
}

// removeByHash drops an entry by its wire hash. The matching (sender, nonce)
// slot is purged as well; if the slot was in pending, subsequent nonces are
// demoted to queue by dropNonce to preserve contiguity. Returns true iff a tx
// was present and removed.
func (m *ethPoolMap) removeByHash(hash thor.Bytes32) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	txObj, ok := m.allByHash[hash]
	if !ok {
		return false
	}
	delete(m.allByHash, hash)

	origin := txObj.Origin()
	if sender, exists := m.senders[origin]; exists {
		sender.dropNonce(txObj.Nonce())
		if sender.empty() {
			delete(m.senders, origin)
		}
	}
	return true
}

// bumpStateNonce advances a sender's stateNonce, evicting settled txs and
// returning the evicted objects so the caller can purge allByHash.
func (m *ethPoolMap) bumpStateNonce(origin thor.Address, newStateNonce uint64) []*TxObject {
	m.lock.Lock()
	defer m.lock.Unlock()

	sender, ok := m.senders[origin]
	if !ok {
		return nil
	}
	evicted := sender.bumpStateNonce(newStateNonce)
	for _, ev := range evicted {
		delete(m.allByHash, ev.Hash())
	}
	if sender.empty() {
		delete(m.senders, origin)
	}
	return evicted
}

// getByHash looks up by Ethereum wire hash. Returns nil if absent.
func (m *ethPoolMap) getByHash(hash thor.Bytes32) *TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.allByHash[hash]
}

// poolNonce returns the next expected nonce for addr: stateNonce +
// len(contiguous pending). chainNonce is the canonical nonce from state; if
// the sender is unknown it is seeded from chainNonce, and if the sender's
// in-memory stateNonce is stale it is bumped to match.
func (m *ethPoolMap) poolNonce(addr thor.Address, chainNonce uint64) uint64 {
	m.lock.Lock()
	defer m.lock.Unlock()

	sender, ok := m.senders[addr]
	if !ok {
		return chainNonce
	}
	if chainNonce > sender.stateNonce {
		return chainNonce + uint64(len(sender.pending))
	}
	return sender.nextPendingNonce()
}

// ensureSender returns the ethSender for addr, creating one seeded at
// chainNonce if absent, or bumping stateNonce if the chain has advanced past
// the in-memory value. Must be called with m.lock held for writing.
func (m *ethPoolMap) ensureSender(addr thor.Address, chainNonce uint64) *ethSender {
	sender, ok := m.senders[addr]
	if !ok {
		sender = newEthSender(chainNonce)
		m.senders[addr] = sender
		return sender
	}
	// If the chain nonce is ahead of what we track, bump to catch up.
	if chainNonce > sender.stateNonce {
		evicted := sender.bumpStateNonce(chainNonce)
		for _, ev := range evicted {
			delete(m.allByHash, ev.Hash())
		}
	}
	return sender
}

// snapshot returns a consistent view of the map's data under a single lock
// acquisition. allTxs holds every tx currently indexed; pendingGroups groups
// pending-only txs by sender in ascending nonce order, ready for the
// coordinator's k-way EFS merge.
func (m *ethPoolMap) snapshot() (allTxs []*TxObject, pendingGroups [][]*TxObject) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	allTxs = make([]*TxObject, 0, len(m.allByHash))
	for _, t := range m.allByHash {
		allTxs = append(allTxs, t)
	}
	pendingGroups = make([][]*TxObject, 0, len(m.senders))
	for _, s := range m.senders {
		if g := s.sortedPending(); len(g) > 0 {
			pendingGroups = append(pendingGroups, g)
		}
	}
	return
}

// len returns the total number of indexed txs (pending + queued).
func (m *ethPoolMap) len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.allByHash)
}
