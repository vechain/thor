// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"github.com/vechain/thor/v2/thor"
)

// Reconciliation aligns senders with the canonical head: it advances nonces and
// promotes whatever the resulting room allows. It is the only maintenance work
// that needs chain state, so it runs when the head moves rather than on a timer.
// It neither discovers head changes nor publishes pool events.

// syncHead advances the given senders to their canonical nonce and promotes newly
// contiguous, affordable queued transactions. This is the block-commit path: the
// reservations already held for pending transactions are trusted.
func (m *ethPoolCore) syncHead(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
	prepare ethPrepare,
) ([]*TxObject, error) {
	promoted, _, err := m.syncHeadWithTransitions(stateNonces, pendingLimit, prepare)
	return promoted, err
}

func (m *ethPoolCore) syncHeadWithTransitions(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
	prepare ethPrepare,
) ([]*TxObject, []*TxObject, error) {
	return m.reconcileSenders(stateNonces, pendingLimit, prepare, m.promoteLocked)
}

// revalidate is syncHead plus a re-check that each sender's pending run is still
// affordable, demoting the suffix that is not. It is the housekeeping safety net,
// so it re-derives what the commit paths trust.
func (m *ethPoolCore) revalidate(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
	prepare ethPrepare,
) ([]*TxObject, []*TxObject, error) {
	return m.reconcileSenders(stateNonces, pendingLimit, prepare, m.revalidateSenderLocked)
}

// reconcileSenders runs the shared reconcile shape: enumerate the preparation
// window and fetch chain state with no lock held, then sync each sender's nonce
// and apply step under the write lock.
func (m *ethPoolCore) reconcileSenders(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
	prepare ethPrepare,
	step func(*ethSender, int, ethPreparations) ([]*TxObject, error),
) ([]*TxObject, []*TxObject, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	prepared := prepareEthObjects(m.preparationWindow(stateNonces, pendingLimit), prepare)

	m.lock.Lock()
	defer m.lock.Unlock()

	origins := sortedEthOrigins(stateNonces)
	wasExecutable := m.executableObjectsLocked(origins)
	var promoted []*TxObject
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		if err := m.syncSenderNonceLocked(sender, stateNonces[origin]); err != nil {
			return nil, nil, err
		}
		if sender.isEmpty() {
			delete(m.senders, origin)
			continue
		}
		stepPromoted, err := step(sender, pendingLimit, prepared)
		if err != nil {
			return nil, nil, err
		}
		promoted = append(promoted, stepPromoted...)
		if sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
	return m.newPromotionsLocked(promoted, wasExecutable),
		m.retainedDemotionsLocked(wasExecutable),
		nil
}

func (m *ethPoolCore) syncSenderNonceLocked(sender *ethSender, stateNonce uint64) error {
	var releases []reservationOwner
	for nonce := range sender.pending {
		if stateNonce < sender.stateNonce || nonce < stateNonce {
			releases = append(releases, ethReservationOwner(sender.origin, nonce))
		}
	}
	if err := m.costs.release(releases...); err != nil {
		return err
	}
	settled, _ := sender.syncStateNonce(stateNonce)
	for _, txObj := range settled {
		delete(m.allByHash, txObj.Hash())
	}
	return nil
}

// revalidateSenderLocked re-reserves the pending run at current cost, demotes the
// suffix that no longer fits the balance, and promotes into the freed room.
func (m *ethPoolCore) revalidateSenderLocked(
	sender *ethSender,
	pendingLimit int,
	prepared ethPreparations,
) ([]*TxObject, error) {
	pending := make([]*TxObject, 0, len(sender.pending))
	releases := make([]reservationOwner, 0, len(sender.pending))
	requests := make([]reservationRequest, 0, len(sender.pending))
	preparations := make([]ethPreparation, 0, len(sender.pending))
	preparing := true
	for nonce := sender.stateNonce; nonce < sender.poolNonce(); nonce++ {
		txObj := sender.pending[nonce]
		if txObj == nil {
			break
		}
		pending = append(pending, txObj)
		releases = append(releases, ethReservationOwner(sender.origin, nonce))
		if preparing {
			preparation, err := prepared.get(txObj)
			if err != nil {
				return nil, err
			}
			if preparation.err != nil || !preparation.viable {
				preparing = false
			} else {
				requests = append(requests, preparation.request)
				preparations = append(preparations, preparation)
			}
		}
	}

	accepted, err := m.costs.reconcile(releases, requests, acceptAffordablePrefix)
	if err != nil {
		return nil, err
	}
	for i := range accepted {
		preparations[i].apply(pending[i])
		pending[i].executable = true
	}
	if accepted < len(pending) {
		sender.demoteFrom(pending[accepted].Nonce())
	}
	return m.promoteLocked(sender, pendingLimit, prepared)
}
