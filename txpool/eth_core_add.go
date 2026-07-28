// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"

	"github.com/vechain/thor/v2/thor"
)

// Addition commits already planned transaction and reservation changes to the
// core; chain/state preparation remains outside the live core lock.
// add places a transaction and performs all nonce-index and reservation changes
// while holding the map lock. costTracker is a leaf lock and never calls back
// into the pool.
func (m *ethPoolCore) add(
	txObj *TxObject,
	stateNonce uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) (bool, []*TxObject, error) {
	executable, promoted, _, err := m.addWithTransitions(
		txObj, stateNonce, globalLimit, pendingLimit, queueLimit, priceBump, prepare,
	)
	return executable, promoted, err
}

func (m *ethPoolCore) addWithTransitions(
	txObj *TxObject,
	stateNonce uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) (bool, []*TxObject, []*TxObject, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	objects := m.addPreparationWindow(txObj, stateNonce, pendingLimit)
	prepared := prepareEthObjects(objects, prepare)

	m.lock.Lock()
	defer m.lock.Unlock()
	// addLocked only ever mutates this transaction's own sender.
	wasExecutable := m.executableObjectsLocked([]thor.Address{txObj.Origin()})
	executable, promoted, err := m.addLocked(
		txObj, stateNonce, globalLimit, pendingLimit, queueLimit, priceBump, prepared,
	)
	if err != nil {
		return false, nil, m.retainedDemotionsLocked(wasExecutable), err
	}
	return executable, promoted, m.retainedDemotionsLocked(wasExecutable), nil
}

// addLocked inserts one transaction. The caller must hold m.lock for writing.
func (m *ethPoolCore) addLocked(
	txObj *TxObject,
	stateNonce uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepared ethPreparations,
) (bool, []*TxObject, error) {
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
		preparation, err := prepared.get(txObj)
		if err != nil {
			return false, nil, err
		}
		if preparation.err != nil {
			return false, nil, preparation.err
		}
		if preparation.viable {
			if err := m.costs.reserve(
				preparation.request.owner,
				preparation.request.payer,
				preparation.request.cost,
				preparation.request.balance,
			); err != nil {
				return false, nil, err
			}
			preparation.apply(txObj)
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

	promotions, err := m.promoteLocked(sender, pendingLimit, prepared)
	if err != nil {
		return false, nil, err
	}

	if len(sender.pending) > pendingLimit {
		return false, nil, errEthAccountPendingOverflow
	}
	return sender.isPending(txObj.Nonce()), promotions, nil
}

// promoteLocked moves the affordable contiguous queue prefix into pending.
// pendingLimit is the sender's pending capacity; any value at or below zero means
// no capacity, which is also what senderWindowSpan assumes.
func (m *ethPoolCore) promoteLocked(
	sender *ethSender,
	pendingLimit int,
	prepared ethPreparations,
) ([]*TxObject, error) {
	var (
		promotions   []*TxObject
		preparations []ethPreparation
		requests     []reservationRequest
	)
	for len(sender.pending) < pendingLimit {
		next := sender.poolNonce()
		queued := sender.queue[next]
		if queued == nil {
			break
		}
		preparation, err := prepared.get(queued)
		if err != nil {
			return nil, err
		}
		if preparation.err != nil || !preparation.viable {
			break
		}
		promotions = append(promotions, queued)
		preparations = append(preparations, preparation)
		requests = append(requests, preparation.request)
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
		return nil, err
	}
	for i, promoted := range promotions[:accepted] {
		preparations[i].apply(promoted)
		promoted.executable = true
		sender.pending[promoted.Nonce()] = promoted
		delete(sender.queue, promoted.Nonce())
	}
	return promotions[:accepted], nil
}
