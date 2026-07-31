// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"bytes"
	"cmp"
	"errors"
	"slices"
	"sync"

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

// ethPoolCore owns the canonical Ethereum-family transaction indexes and
// coordinates their reservations through the injected costTracker. It
// serializes mutations while allowing lock-independent state preparation; it
// does not read chain state or publish pool events.
type ethPoolCore struct {
	// mutationMu preserves the original single-writer operation ordering while
	// allowing map-lock-free chain/state preparation between planning and commit.
	// Readers only use lock and remain available during preparation.
	mutationMu sync.Mutex
	lock       sync.RWMutex
	allByHash  map[thor.Bytes32]*TxObject
	senders    map[thor.Address]*ethSender
	costs      *costTracker
	options    Options
}

func newEthPoolCore(costs *costTracker, options Options) *ethPoolCore {
	return &ethPoolCore{
		allByHash: make(map[thor.Bytes32]*TxObject),
		senders:   make(map[thor.Address]*ethSender),
		costs:     costs,
		options:   options,
	}
}

func (m *ethPoolCore) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.allByHash)
}

func (m *ethPoolCore) GetByHash(hash thor.Bytes32) *TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.allByHash[hash]
}

func (m *ethPoolCore) ToTxs() tx.Transactions {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txs := make(tx.Transactions, 0, len(m.allByHash))
	for _, txObj := range m.allByHash {
		txs = append(txs, txObj.Transaction)
	}
	return txs
}

func (m *ethPoolCore) origins() []thor.Address {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.sortedOriginsLocked()
}

func (m *ethPoolCore) sortedOriginsLocked() []thor.Address {
	origins := make([]thor.Address, 0, len(m.senders))
	for origin := range m.senders {
		origins = append(origins, origin)
	}
	slices.SortFunc(origins, func(a, b thor.Address) int {
		return bytes.Compare(a[:], b[:])
	})
	return origins
}

// func (m *ethPoolCore) poolNonce(addr thor.Address) uint64 {
// 	nonce, _ := m.poolNonceOK(addr)
// 	return nonce
// }

func (m *ethPoolCore) poolNonceOK(addr thor.Address) (uint64, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if s := m.senders[addr]; s != nil {
		return s.poolNonce(), true
	}
	return 0, false
}

// executableSnapshot returns one nonce-ordered executable stream per sender.
// It copies only merge metadata, so the map lock is not held during heap work.
func (m *ethPoolCore) executableSnapshot() ethExecutablesSnapshot {
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

	snapshot := make(ethExecutablesSnapshot, 0, len(origins))
	for _, origin := range origins {
		sender := m.senders[origin]
		group := make([]executableTx, 0, len(sender.pending))
		for nonce := sender.stateNonce; nonce < sender.poolNonce(); nonce++ {
			txObj := sender.pending[nonce]
			if txObj == nil || !txObj.executable || txObj.priorityGasPrice() == nil {
				break
			}
			group = append(group, executableTxFromObject(txObj))
		}
		if len(group) > 0 {
			snapshot = append(snapshot, group)
		}
	}
	return snapshot
}

func (m *ethPoolCore) removeByHash(hash thor.Bytes32) bool {
	removed, _ := m.removeByHashWithTransitions(hash)
	return removed
}

func (m *ethPoolCore) removeByHashWithTransitions(hash thor.Bytes32) (bool, []*TxObject) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	m.lock.Lock()
	defer m.lock.Unlock()

	txObj := m.allByHash[hash]
	if txObj == nil {
		return false, nil
	}
	origin, nonce := txObj.Origin(), txObj.Nonce()
	sender := m.senders[origin]
	if sender == nil {
		return false, nil
	}
	wasExecutable := m.executableObjectsLocked([]thor.Address{origin})

	var releases []reservationOwner
	switch {
	case sender.pending[nonce] == txObj:
		var removed bool
		releases, removed = sender.dropNonce(nonce)
		if !removed {
			return false, nil
		}
	case sender.queue[nonce] == txObj:
		delete(sender.queue, nonce)
	default:
		return false, nil
	}

	delete(m.allByHash, hash)
	if err := m.costs.release(releases...); err != nil {
		logger.Error("failed to release Ethereum transaction costs", "hash", hash, "err", err)
	}
	if sender.isEmpty() {
		delete(m.senders, origin)
	}
	return true, m.retainedDemotionsLocked(wasExecutable)
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

// executableObjectsLocked snapshots the executable transactions of the given
// senders so a mutation can report the transitions it caused. Every demotion
// primitive acts on a single ethSender, so callers pass only the senders they
// touch and the snapshot stays proportional to the change, not to the pool.
func (m *ethPoolCore) executableObjectsLocked(origins []thor.Address) map[thor.Bytes32]*TxObject {
	objects := make(map[thor.Bytes32]*TxObject)
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		for _, txObj := range sender.pending {
			if txObj.executable {
				objects[txObj.Hash()] = txObj
			}
		}
	}
	return objects
}

// newPromotionsLocked filters promotions down to transactions that were not
// already executable in the snapshot and are still live and executable now.
// It filters in place, so the caller must not reuse promoted afterwards.
func (m *ethPoolCore) newPromotionsLocked(
	promoted []*TxObject,
	wasExecutable map[thor.Bytes32]*TxObject,
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

func (m *ethPoolCore) retainedDemotionsLocked(
	wasExecutable map[thor.Bytes32]*TxObject,
) []*TxObject {
	demoted := make([]*TxObject, 0)
	for hash, txObj := range wasExecutable {
		if m.allByHash[hash] == txObj && !txObj.executable {
			demoted = append(demoted, txObj)
		}
	}
	slices.SortFunc(demoted, func(a, b *TxObject) int {
		aOrigin, bOrigin := a.Origin(), b.Origin()
		if originCmp := bytes.Compare(aOrigin[:], bOrigin[:]); originCmp != 0 {
			return originCmp
		}
		return cmp.Compare(a.Nonce(), b.Nonce())
	})
	return demoted
}

// pruneEmptySenders drops senders with no pending or queued txs.
// Scaffold hook for post-mutation GC.
func (m *ethPoolCore) pruneEmptySenders() {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	m.lock.Lock()
	defer m.lock.Unlock()
	for addr, s := range m.senders {
		if s.isEmpty() {
			delete(m.senders, addr)
		}
	}
}
