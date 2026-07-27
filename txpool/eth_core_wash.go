// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"slices"
	"time"

	"github.com/vechain/thor/v2/thor"
)

// Washing reconciles canonical nonces and enforces expiry and capacity limits;
// it does not fetch chain state or publish transaction events.
type ethWashOptions struct {
	now          int64
	maxLifetime  time.Duration
	pendingLimit int
	queueLimit   int
	globalLimit  int
}

type ethWashResult struct {
	promoted []*TxObject
	demoted  []*TxObject
	removed  int
}

// syncHead reconciles affected senders with the canonical head nonce and
// promotes newly contiguous, affordable queued transactions.
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
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	objects := m.syncPreparationObjects(stateNonces, pendingLimit)
	prepared := prepareEthObjects(objects, prepare)

	m.lock.Lock()
	defer m.lock.Unlock()
	wasExecutableObjects := m.executableObjectsLocked()

	origins := sortedEthOrigins(stateNonces)
	wasExecutable := m.executableHashesLocked(origins)
	var newlyPromoted []*TxObject
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}

		if err := m.syncSenderNonceLocked(sender, stateNonces[origin]); err != nil {
			return nil, nil, err
		}

		promoted, err := m.promoteLocked(sender, pendingLimit, prepared)
		if err != nil {
			return nil, nil, err
		}
		newlyPromoted = append(newlyPromoted, filterNewPromotions(promoted, wasExecutable)...)
		if sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
	return newlyPromoted, m.retainedDemotionsLocked(wasExecutableObjects), nil
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

func (m *ethPoolCore) wash(
	stateNonces map[thor.Address]uint64,
	options ethWashOptions,
	prepare ethPrepare,
) (ethWashResult, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	objects := m.washPreparationObjects(stateNonces, options)
	prepared := prepareEthObjects(objects, prepare)

	m.lock.Lock()
	defer m.lock.Unlock()

	wasExecutableObjects := m.executableObjectsLocked()
	origins := m.sortedOriginsLocked()
	wasExecutable := m.executableHashesLocked(origins)
	var result ethWashResult
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		stateNonce, present := stateNonces[origin]
		if !present {
			// A sender admitted after the state snapshot is washed next time.
			continue
		}
		if err := m.syncSenderNonceLocked(sender, stateNonce); err != nil {
			return ethWashResult{}, err
		}
		if err := m.removeExpiredLocked(sender, options, &result); err != nil {
			return ethWashResult{}, err
		}
		if sender.isEmpty() {
			delete(m.senders, origin)
			continue
		}

		promoted, err := m.revalidateSenderLocked(sender, options.pendingLimit, prepared)
		if err != nil {
			return ethWashResult{}, err
		}
		result.promoted = append(result.promoted, promoted...)
		if err := m.enforceSenderLimitsLocked(sender, options, &result); err != nil {
			return ethWashResult{}, err
		}
		if sender.isEmpty() {
			delete(m.senders, origin)
		}
	}

	if err := m.enforceGlobalLimitLocked(origins, options.globalLimit, &result); err != nil {
		return ethWashResult{}, err
	}
	result.promoted = m.retainedPromotionsLocked(result.promoted, wasExecutable)
	result.demoted = m.retainedDemotionsLocked(wasExecutableObjects)
	return result, nil
}

func (m *ethPoolCore) removeExpiredLocked(
	sender *ethSender,
	options ethWashOptions,
	result *ethWashResult,
) error {
	if options.maxLifetime <= 0 {
		return nil
	}
	expired := func(txObj *TxObject) bool {
		return !txObj.localSubmitted &&
			options.now > txObj.timeAdded &&
			options.now-txObj.timeAdded > int64(options.maxLifetime)
	}

	for nonce := sender.stateNonce; nonce < sender.poolNonce(); nonce++ {
		txObj := sender.pending[nonce]
		if txObj != nil && expired(txObj) {
			if err := m.evictPendingFromLocked(sender, nonce, txObj, result); err != nil {
				return err
			}
			break
		}
	}
	for nonce, txObj := range sender.queue {
		if expired(txObj) {
			delete(sender.queue, nonce)
			delete(m.allByHash, txObj.Hash())
			result.removed++
		}
	}
	return nil
}

// evictPendingFromLocked removes target and demotes its higher nonce suffix.
func (m *ethPoolCore) evictPendingFromLocked(
	sender *ethSender,
	nonce uint64,
	target *TxObject,
	result *ethWashResult,
) error {
	var releases []reservationOwner
	for pendingNonce := range sender.pending {
		if pendingNonce >= nonce {
			releases = append(releases, ethReservationOwner(sender.origin, pendingNonce))
		}
	}
	if err := m.costs.release(releases...); err != nil {
		return err
	}
	_, removed := sender.dropNonce(nonce)
	if !removed {
		return nil
	}
	delete(m.allByHash, target.Hash())
	result.removed++
	return nil
}

func (m *ethPoolCore) revalidateSenderLocked(
	sender *ethSender,
	pendingLimit int,
	prepared ethPreparations,
) ([]*TxObject, error) {
	pending := make([]*TxObject, 0, len(sender.pending))
	releases := make([]reservationOwner, 0, len(sender.pending))
	requests := make([]reservationRequest, 0, len(sender.pending))
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
			}
		}
	}

	accepted, err := m.costs.reconcile(releases, requests, acceptAffordablePrefix)
	if err != nil {
		return nil, err
	}
	for i := range accepted {
		prepared[pending[i]].apply(pending[i])
		pending[i].executable = true
	}
	if accepted < len(pending) {
		sender.demoteFrom(pending[accepted].Nonce())
	}
	if pendingLimit < 0 {
		pendingLimit = len(sender.pending) + len(sender.queue)
	}
	return m.promoteLocked(sender, pendingLimit, prepared)
}

func (m *ethPoolCore) enforceSenderLimitsLocked(
	sender *ethSender,
	options ethWashOptions,
	result *ethWashResult,
) error {
	if options.pendingLimit >= 0 && len(sender.pending) > options.pendingLimit {
		cutoff := sender.stateNonce + uint64(options.pendingLimit)
		var releases []reservationOwner
		for nonce := range sender.pending {
			if nonce >= cutoff {
				releases = append(releases, ethReservationOwner(sender.origin, nonce))
			}
		}
		if err := m.costs.release(releases...); err != nil {
			return err
		}
		sender.demoteFrom(cutoff)
	}
	if options.queueLimit >= 0 && len(sender.queue) > options.queueLimit {
		nonces := sortedNoncesDesc(sender.queue)
		excess := len(nonces) - options.queueLimit
		for _, nonce := range nonces[:excess] {
			txObj := sender.queue[nonce]
			delete(sender.queue, nonce)
			delete(m.allByHash, txObj.Hash())
			result.removed++
		}
	}
	return nil
}

func sortedNoncesDesc(txObjs map[uint64]*TxObject) []uint64 {
	nonces := make([]uint64, 0, len(txObjs))
	for nonce := range txObjs {
		nonces = append(nonces, nonce)
	}
	slices.Sort(nonces)
	slices.Reverse(nonces)
	return nonces
}

type queuedEvictionCursor struct {
	sender *ethSender
	nonces []uint64
	next   int
}

type pendingTail struct {
	sender *ethSender
	nonce  uint64
	txObj  *TxObject
}

func (m *ethPoolCore) enforceGlobalLimitLocked(
	origins []thor.Address,
	limit int,
	result *ethWashResult,
) error {
	if limit <= 0 || len(m.allByHash) <= limit {
		return nil
	}

	m.evictQueuedUntilLimitLocked(m.queueEvictionCursorsLocked(origins), limit, result)
	if err := m.evictPendingTailsUntilLimitLocked(origins, limit, result); err != nil {
		return err
	}
	m.pruneEmptyOriginsLocked(origins)
	return nil
}

func (m *ethPoolCore) queueEvictionCursorsLocked(origins []thor.Address) []queuedEvictionCursor {
	cursors := make([]queuedEvictionCursor, 0, len(origins))
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender != nil && len(sender.queue) > 0 {
			cursors = append(cursors, queuedEvictionCursor{
				sender: sender,
				nonces: sortedNoncesDesc(sender.queue),
			})
		}
	}
	return cursors
}

func (m *ethPoolCore) evictQueuedUntilLimitLocked(
	cursors []queuedEvictionCursor,
	limit int,
	result *ethWashResult,
) {
	for len(m.allByHash) > limit {
		removed := false
		for i := range cursors {
			cursor := &cursors[i]
			if cursor.next >= len(cursor.nonces) {
				continue
			}
			nonce := cursor.nonces[cursor.next]
			cursor.next++
			txObj := cursor.sender.queue[nonce]
			if txObj == nil {
				continue
			}
			delete(cursor.sender.queue, nonce)
			delete(m.allByHash, txObj.Hash())
			result.removed++
			removed = true
			if len(m.allByHash) <= limit {
				break
			}
		}
		if !removed {
			return
		}
	}
}

func (m *ethPoolCore) evictPendingTailsUntilLimitLocked(
	origins []thor.Address,
	limit int,
	result *ethWashResult,
) error {
	for len(m.allByHash) > limit {
		tails, releases := m.pendingTailBatchLocked(origins, len(m.allByHash)-limit)
		if len(tails) == 0 {
			return nil
		}
		if err := m.costs.release(releases...); err != nil {
			return err
		}
		for _, tail := range tails {
			tail.txObj.executable = false
			delete(tail.sender.pending, tail.nonce)
			delete(m.allByHash, tail.txObj.Hash())
			result.removed++
		}
	}
	return nil
}

func (m *ethPoolCore) pendingTailBatchLocked(
	origins []thor.Address,
	maxCount int,
) ([]pendingTail, []reservationOwner) {
	if maxCount <= 0 {
		return nil, nil
	}
	capacity := min(len(origins), maxCount)
	tails := make([]pendingTail, 0, capacity)
	releases := make([]reservationOwner, 0, capacity)
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil || len(sender.pending) == 0 {
			continue
		}
		nonce := sender.poolNonce() - 1
		txObj := sender.pending[nonce]
		if txObj == nil {
			continue
		}
		tails = append(tails, pendingTail{sender, nonce, txObj})
		releases = append(releases, ethReservationOwner(origin, nonce))
		if len(tails) == maxCount {
			break
		}
	}
	return tails, releases
}

func (m *ethPoolCore) pruneEmptyOriginsLocked(origins []thor.Address) {
	for _, origin := range origins {
		if sender := m.senders[origin]; sender != nil && sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
}
