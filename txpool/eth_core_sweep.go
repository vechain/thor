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

// Sweeping enforces expiry and capacity. It reads no chain state and prepares
// nothing, so it can run on a cheap timer independently of the head. It does not
// publish pool events.
type ethSweepResult struct {
	removed int
	demoted []*TxObject
}

// sweep drops expired transactions and trims the pool back inside its per-sender
// and global limits. It only removes and demotes, never promotes: expiry leaves a
// nonce gap behind it and limit enforcement leaves the limit full, so nothing it
// does can make a queued transaction contiguous and affordable.
func (m *ethPoolCore) sweep() (ethSweepResult, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	m.lock.Lock()
	defer m.lock.Unlock()

	origins := m.sortedOriginsLocked()
	wasExecutable := m.executableObjectsLocked(origins)
	var result ethSweepResult
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		if err := m.removeExpiredLocked(sender, m.options, &result); err != nil {
			return ethSweepResult{}, err
		}
		if err := m.enforceSenderLimitsLocked(sender, m.options, &result); err != nil {
			return ethSweepResult{}, err
		}
		if sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
	if err := m.enforceGlobalLimitLocked(origins, m.options.Limit, &result); err != nil {
		return ethSweepResult{}, err
	}
	result.demoted = m.retainedDemotionsLocked(wasExecutable)
	return result, nil
}

func (m *ethPoolCore) removeExpiredLocked(
	sender *ethSender,
	options Options,
	result *ethSweepResult,
) error {
	if options.MaxLifetime <= 0 {
		return nil
	}
	now := time.Now().UnixNano()
	expired := func(txObj *TxObject) bool {
		return !txObj.localSubmitted() &&
			now > txObj.timeAdded &&
			now-txObj.timeAdded > int64(options.MaxLifetime)
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
	result *ethSweepResult,
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

func (m *ethPoolCore) enforceSenderLimitsLocked(
	sender *ethSender,
	options Options,
	result *ethSweepResult,
) error {
	if options.EthAccountSlots >= 0 && len(sender.pending) > options.EthAccountSlots {
		cutoff := sender.stateNonce + uint64(options.EthAccountSlots)
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
	if options.EthAccountQueue >= 0 && len(sender.queue) > options.EthAccountQueue {
		nonces := sortedNoncesDesc(sender.queue)
		excess := len(nonces) - options.EthAccountQueue
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
	result *ethSweepResult,
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
	result *ethSweepResult,
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
	result *ethSweepResult,
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
