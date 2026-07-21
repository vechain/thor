// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"sync"

	"github.com/vechain/thor/v2/thor"
)

var (
	errEthAlreadyKnown           = errors.New("already known")
	errEthNonceTooLow            = errors.New("nonce too low")
	errEthReplaceUnderpriced     = errors.New("replacement transaction underpriced")
	errEthAccountPendingOverflow = errors.New("account pending limit exceeded")
	errEthAccountQueueOverflow   = errors.New("account queue limit exceeded")
)

type ethPrepare func(*TxObject) (reservationRequest, bool, error)

// ethPoolMap is a thread-safe index of Ethereum-family pooled transactions.
type ethPoolMap struct {
	lock      sync.RWMutex
	allByHash map[thor.Bytes32]*TxObject
	senders   map[thor.Address]*ethSender
	costs     *costTracker
}

func newEthPoolMap(costs *costTracker) *ethPoolMap {
	return &ethPoolMap{
		allByHash: make(map[thor.Bytes32]*TxObject),
		senders:   make(map[thor.Address]*ethSender),
		costs:     costs,
	}
}

func (m *ethPoolMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.allByHash)
}

func (m *ethPoolMap) GetByHash(hash thor.Bytes32) *TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.allByHash[hash]
}

func (m *ethPoolMap) poolNonce(addr thor.Address) uint64 {
	nonce, _ := m.poolNonceOK(addr)
	return nonce
}

func (m *ethPoolMap) poolNonceOK(addr thor.Address) (uint64, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if s := m.senders[addr]; s != nil {
		return s.poolNonce(), true
	}
	return 0, false
}

// add places a transaction and performs all nonce-index and reservation changes
// while holding the map lock. costTracker is a leaf lock and never calls back
// into the pool.
func (m *ethPoolMap) add(
	txObj *TxObject,
	stateNonce uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) (bool, []*TxObject, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	hash := txObj.Hash()
	if m.allByHash[hash] != nil {
		return false, nil, errEthAlreadyKnown
	}

	origin := txObj.Origin()
	sender := m.senders[origin]
	if sender == nil {
		sender = newEthSender(origin, stateNonce)
		m.senders[origin] = sender
	}

	settled, releases := sender.syncStateNonce(stateNonce)
	if err := m.costs.release(releases...); err != nil {
		return false, nil, err
	}
	for _, old := range settled {
		delete(m.allByHash, old.Hash())
	}
	if txObj.Nonce() < sender.stateNonce {
		return false, nil, errEthNonceTooLow
	}

	incumbent := sender.get(txObj.Nonce())
	if incumbent == nil && globalLimit > 0 && len(m.allByHash) >= globalLimit {
		return false, nil, errors.New("pool is full")
	}
	if incumbent != nil && !isFeeBumpSufficient(incumbent, txObj, priceBump) {
		return false, nil, errEthReplaceUnderpriced
	}

	replacePending := incumbent != nil && sender.isPending(txObj.Nonce())
	canEnterPending := replacePending ||
		(txObj.Nonce() == sender.poolNonce() && len(sender.pending) < pendingLimit)
	if canEnterPending {
		request, viable, err := prepare(txObj)
		if err != nil {
			return false, nil, err
		}
		if viable {
			if err := m.costs.reserve(request.owner, request.payer, request.cost, request.balance); err != nil {
				return false, nil, err
			}
			txObj.executable = true
			sender.pending[txObj.Nonce()] = txObj
			delete(sender.queue, txObj.Nonce())
		} else {
			if replacePending && queueLimit >= 0 &&
				len(sender.queue)+sender.pendingCountFrom(txObj.Nonce()) > queueLimit {
				return false, nil, errEthAccountQueueOverflow
			}
			canEnterPending = false
		}
	}

	if !canEnterPending {
		if incumbent == nil && queueLimit >= 0 && len(sender.queue) >= queueLimit {
			return false, nil, errEthAccountQueueOverflow
		}
		txObj.executable = false
		if replacePending {
			if err := m.costs.release(sender.demoteFrom(txObj.Nonce())...); err != nil {
				return false, nil, err
			}
		}
		sender.queue[txObj.Nonce()] = txObj
		delete(sender.pending, txObj.Nonce())
	}

	if incumbent != nil {
		delete(m.allByHash, incumbent.Hash())
	}
	m.allByHash[hash] = txObj

	// Filling a gap can expose a contiguous queued suffix. Prepare all viable
	// entries and let the shared ledger accept only its affordable prefix.
	var (
		promotions []*TxObject
		requests   []reservationRequest
	)
	for len(sender.pending) < pendingLimit {
		next := sender.poolNonce()
		queued := sender.queue[next]
		if queued == nil {
			break
		}
		request, viable, err := prepare(queued)
		if err != nil || !viable {
			break
		}
		promotions = append(promotions, queued)
		requests = append(requests, request)
		// Temporarily advance the contiguous cursor. Restore before touching the
		// cost tracker so only the accepted prefix is committed.
		sender.pending[next] = queued
		delete(sender.queue, next)
	}
	for _, promoted := range promotions {
		delete(sender.pending, promoted.Nonce())
		sender.queue[promoted.Nonce()] = promoted
	}
	accepted, err := m.costs.reconcile(nil, requests, acceptAffordablePrefix)
	if err != nil {
		return false, nil, err
	}
	for _, promoted := range promotions[:accepted] {
		promoted.executable = true
		sender.pending[promoted.Nonce()] = promoted
		delete(sender.queue, promoted.Nonce())
	}

	if len(sender.pending) > pendingLimit {
		return false, nil, errEthAccountPendingOverflow
	}
	return sender.isPending(txObj.Nonce()), promotions[:accepted], nil
}

// pruneEmptySenders drops senders with no pending or queued txs.
// Scaffold hook for post-mutation GC.
func (m *ethPoolMap) pruneEmptySenders() {
	m.lock.Lock()
	defer m.lock.Unlock()
	for addr, s := range m.senders {
		if s.isEmpty() {
			delete(m.senders, addr)
		}
	}
}
