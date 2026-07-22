// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"bytes"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var (
	errEthAlreadyKnown           = errors.New("already known")
	errEthNonceTooLow            = errors.New("nonce too low")
	errEthReplaceUnderpriced     = errors.New("replacement transaction underpriced")
	errEthAccountPendingOverflow = errors.New("account pending limit exceeded")
	errEthAccountQueueOverflow   = errors.New("account queue limit exceeded")
)

type ethPrepare func(*TxObject) (reservationRequest, bool, error)

type ethForkCandidate struct {
	txObj      *TxObject
	stateNonce uint64
}

type ethForkResult struct {
	txObj      *TxObject
	executable bool
	promoted   []*TxObject
	err        error
}

type ethWashOptions struct {
	now          int64
	maxLifetime  time.Duration
	pendingLimit int
	queueLimit   int
	globalLimit  int
}

type ethWashResult struct {
	promoted []*TxObject
	removed  int
}

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

func (m *ethPoolMap) ToTxs() tx.Transactions {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txs := make(tx.Transactions, 0, len(m.allByHash))
	for _, txObj := range m.allByHash {
		txs = append(txs, txObj.Transaction)
	}
	return txs
}

func (m *ethPoolMap) origins() []thor.Address {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.sortedOriginsLocked()
}

func (m *ethPoolMap) sortedOriginsLocked() []thor.Address {
	origins := make([]thor.Address, 0, len(m.senders))
	for origin := range m.senders {
		origins = append(origins, origin)
	}
	slices.SortFunc(origins, func(a, b thor.Address) int {
		return bytes.Compare(a[:], b[:])
	})
	return origins
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

// executableSnapshot returns one nonce-ordered executable stream per sender.
// It copies only merge metadata, so the map lock is not held during heap work.
func (m *ethPoolMap) executableSnapshot() ethExecutablesSnapshot {
	m.lock.RLock()
	defer m.lock.RUnlock()

	origins := make([]thor.Address, 0, len(m.senders))
	for origin, sender := range m.senders {
		if len(sender.pending) > 0 {
			origins = append(origins, origin)
		}
	}
	slices.SortFunc(origins, func(a, b thor.Address) int {
		return bytes.Compare(a[:], b[:])
	})

	snapshot := ethExecutablesSnapshot{
		groups: make([][]executableTx, 0, len(origins)),
	}
	for _, origin := range origins {
		sender := m.senders[origin]
		group := make([]executableTx, 0, len(sender.pending))
		for nonce := sender.stateNonce; nonce < sender.poolNonce(); nonce++ {
			txObj := sender.pending[nonce]
			if txObj == nil || !txObj.executable || txObj.priorityGasPrice == nil {
				break
			}
			group = append(group, executableTxFromObject(txObj))
		}
		if len(group) > 0 {
			snapshot.groups = append(snapshot.groups, group)
			snapshot.total += len(group)
		}
	}
	return snapshot
}

func (m *ethPoolMap) removeByHash(hash thor.Bytes32) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	txObj := m.allByHash[hash]
	if txObj == nil {
		return false
	}
	origin, nonce := txObj.Origin(), txObj.Nonce()
	sender := m.senders[origin]
	if sender == nil {
		return false
	}

	var releases []reservationOwner
	switch {
	case sender.pending[nonce] == txObj:
		var removed bool
		releases, removed = sender.dropNonce(nonce)
		if !removed {
			return false
		}
	case sender.queue[nonce] == txObj:
		delete(sender.queue, nonce)
	default:
		return false
	}

	delete(m.allByHash, hash)
	if err := m.costs.release(releases...); err != nil {
		logger.Error("failed to release Ethereum transaction costs", "hash", hash, "err", err)
	}
	if sender.isEmpty() {
		delete(m.senders, origin)
	}
	return true
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
	return m.addLocked(txObj, stateNonce, globalLimit, pendingLimit, queueLimit, priceBump, prepare)
}

// addLocked inserts one transaction. The caller must hold m.lock for writing.
func (m *ethPoolMap) addLocked(
	txObj *TxObject,
	stateNonce uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
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

	promotions, err := m.promoteLocked(sender, pendingLimit, prepare)
	if err != nil {
		return false, nil, err
	}

	if len(sender.pending) > pendingLimit {
		return false, nil, errEthAccountPendingOverflow
	}
	return sender.isPending(txObj.Nonce()), promotions, nil
}

// promoteLocked moves the affordable contiguous queue prefix into pending.
func (m *ethPoolMap) promoteLocked(
	sender *ethSender,
	pendingLimit int,
	prepare ethPrepare,
) ([]*TxObject, error) {
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
		return nil, err
	}
	for _, promoted := range promotions[:accepted] {
		promoted.executable = true
		sender.pending[promoted.Nonce()] = promoted
		delete(sender.queue, promoted.Nonce())
	}
	return promotions[:accepted], nil
}

// syncHead reconciles affected senders with the canonical head nonce and
// promotes newly contiguous, affordable queued transactions.
func (m *ethPoolMap) syncHead(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
	prepare ethPrepare,
) ([]*TxObject, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	origins := sortedEthOrigins(stateNonces)
	wasExecutable := m.executableHashesLocked(origins)
	var newlyPromoted []*TxObject
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}

		if err := m.syncSenderNonceLocked(sender, stateNonces[origin]); err != nil {
			return nil, err
		}

		promoted, err := m.promoteLocked(sender, pendingLimit, prepare)
		if err != nil {
			return nil, err
		}
		newlyPromoted = append(newlyPromoted, filterNewPromotions(promoted, wasExecutable)...)
		if sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
	return newlyPromoted, nil
}

func (m *ethPoolMap) syncSenderNonceLocked(sender *ethSender, stateNonce uint64) error {
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

func (m *ethPoolMap) wash(
	stateNonces map[thor.Address]uint64,
	options ethWashOptions,
	prepare ethPrepare,
) (ethWashResult, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

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

		promoted, err := m.revalidateSenderLocked(sender, options.pendingLimit, prepare)
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
	return result, nil
}

func (m *ethPoolMap) removeExpiredLocked(
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
func (m *ethPoolMap) evictPendingFromLocked(
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

func (m *ethPoolMap) revalidateSenderLocked(
	sender *ethSender,
	pendingLimit int,
	prepare ethPrepare,
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
			request, viable, err := prepare(txObj)
			if err != nil || !viable {
				preparing = false
			} else {
				requests = append(requests, request)
			}
		}
	}

	accepted, err := m.costs.reconcile(releases, requests, acceptAffordablePrefix)
	if err != nil {
		return nil, err
	}
	for i := range accepted {
		pending[i].executable = true
	}
	if accepted < len(pending) {
		sender.demoteFrom(pending[accepted].Nonce())
	}
	if pendingLimit < 0 {
		pendingLimit = len(sender.pending) + len(sender.queue)
	}
	return m.promoteLocked(sender, pendingLimit, prepare)
}

func (m *ethPoolMap) enforceSenderLimitsLocked(
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

func (m *ethPoolMap) enforceGlobalLimitLocked(
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

func (m *ethPoolMap) queueEvictionCursorsLocked(origins []thor.Address) []queuedEvictionCursor {
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

func (m *ethPoolMap) evictQueuedUntilLimitLocked(
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

func (m *ethPoolMap) evictPendingTailsUntilLimitLocked(
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

func (m *ethPoolMap) pendingTailBatchLocked(
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

func (m *ethPoolMap) pruneEmptyOriginsLocked(origins []thor.Address) {
	for _, origin := range origins {
		if sender := m.senders[origin]; sender != nil && sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
}

func (m *ethPoolMap) retainedPromotionsLocked(
	promoted []*TxObject,
	wasExecutable map[thor.Bytes32]struct{},
) []*TxObject {
	retained := promoted[:0]
	for _, txObj := range promoted {
		if _, existed := wasExecutable[txObj.Hash()]; existed {
			continue
		}
		if m.allByHash[txObj.Hash()] == txObj && txObj.executable {
			retained = append(retained, txObj)
		}
	}
	return retained
}

func (m *ethPoolMap) reconcileFork(
	candidates []ethForkCandidate,
	stateNonces map[thor.Address]uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) ([]ethForkResult, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	origins := sortedEthOrigins(stateNonces)
	wasExecutable := m.executableHashesLocked(origins)
	if err := m.resetForkSendersLocked(origins, stateNonces); err != nil {
		return nil, err
	}

	results, err := m.promoteForkSendersLocked(origins, wasExecutable, pendingLimit, prepare)
	if err != nil {
		return nil, err
	}
	candidateResults, err := m.addForkCandidatesLocked(
		candidates,
		wasExecutable,
		globalLimit,
		pendingLimit,
		queueLimit,
		priceBump,
		prepare,
	)
	if err != nil {
		return nil, err
	}
	results = append(results, candidateResults...)
	m.pruneForkSendersLocked(origins, candidates)
	return results, nil
}

func sortedEthOrigins(stateNonces map[thor.Address]uint64) []thor.Address {
	origins := make([]thor.Address, 0, len(stateNonces))
	for origin := range stateNonces {
		origins = append(origins, origin)
	}
	slices.SortFunc(origins, func(a, b thor.Address) int {
		return bytes.Compare(a[:], b[:])
	})
	return origins
}

// executableHashesLocked snapshots affected executable transactions before reset.
func (m *ethPoolMap) executableHashesLocked(origins []thor.Address) map[thor.Bytes32]struct{} {
	hashes := make(map[thor.Bytes32]struct{})
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		for _, txObj := range sender.pending {
			if txObj.executable {
				hashes[txObj.Hash()] = struct{}{}
			}
		}
	}
	return hashes
}

// resetForkSendersLocked releases every stale reservation before promotion.
func (m *ethPoolMap) resetForkSendersLocked(
	origins []thor.Address,
	stateNonces map[thor.Address]uint64,
) error {
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		settled, releases := sender.resetStateNonce(stateNonces[origin])
		if err := m.costs.release(releases...); err != nil {
			return err
		}
		for _, old := range settled {
			delete(m.allByHash, old.Hash())
		}
	}
	return nil
}

func (m *ethPoolMap) promoteForkSendersLocked(
	origins []thor.Address,
	wasExecutable map[thor.Bytes32]struct{},
	pendingLimit int,
	prepare ethPrepare,
) ([]ethForkResult, error) {
	var results []ethForkResult
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		promoted, err := m.promoteLocked(sender, pendingLimit, prepare)
		if err != nil {
			return nil, err
		}
		for _, txObj := range filterNewPromotions(promoted, wasExecutable) {
			results = append(results, ethForkResult{txObj: txObj, executable: true})
		}
	}
	return results, nil
}

func (m *ethPoolMap) addForkCandidatesLocked(
	candidates []ethForkCandidate,
	wasExecutable map[thor.Bytes32]struct{},
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) ([]ethForkResult, error) {
	results := make([]ethForkResult, 0, len(candidates))
	for _, candidate := range candidates {
		executable, promoted, err := m.addLocked(
			candidate.txObj,
			candidate.stateNonce,
			globalLimit,
			pendingLimit,
			queueLimit,
			priceBump,
			prepare,
		)
		if errors.Is(err, errCostTrackerState) || errors.Is(err, errInvalidCost) {
			return nil, err
		}
		results = append(results, ethForkResult{
			txObj:      candidate.txObj,
			executable: executable,
			promoted:   filterNewPromotions(promoted, wasExecutable),
			err:        err,
		})
	}
	return results, nil
}

func filterNewPromotions(
	promoted []*TxObject,
	wasExecutable map[thor.Bytes32]struct{},
) []*TxObject {
	filtered := promoted[:0]
	for _, txObj := range promoted {
		if _, alreadyExecutable := wasExecutable[txObj.Hash()]; !alreadyExecutable {
			filtered = append(filtered, txObj)
		}
	}
	return filtered
}

func (m *ethPoolMap) pruneForkSendersLocked(origins []thor.Address, candidates []ethForkCandidate) {
	for _, origin := range origins {
		if sender := m.senders[origin]; sender != nil && sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
	// Candidate origins normally appear in stateNonces, but include them
	// defensively without scanning every sender in the pool.
	for _, candidate := range candidates {
		origin := candidate.txObj.Origin()
		if sender := m.senders[origin]; sender != nil && sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
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
