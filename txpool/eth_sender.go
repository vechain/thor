// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"slices"
)

// ethSender holds the in-pool EthereumTx state for a single sender (origin
// address). Transactions are bucketed by their nonce relative to stateNonce:
//
//   - pending: nonces in [stateNonce, stateNonce+len(pending)) — contiguous,
//     executable from the packer's perspective. Nonce N must not be emitted
//     before nonce N-1.
//   - queue: nonces in the future (> stateNonce+len(pending)) — gapped,
//     awaiting a preceding nonce to arrive.
//
// stateNonce is the nonce the chain will consume next. It is seeded from the
// canonical chain state (state.GetNonce) on first contact and bumped whenever
// head-reset observes inclusion of an EthereumTx from this sender.
type ethSender struct {
	stateNonce uint64
	pending    map[uint64]*TxObject
	queue      map[uint64]*TxObject
}

func newEthSender(stateNonce uint64) *ethSender {
	return &ethSender{
		stateNonce: stateNonce,
		pending:    make(map[uint64]*TxObject),
		queue:      make(map[uint64]*TxObject),
	}
}

// nextPendingNonce returns the first unoccupied nonce slot in the pending chain
// (i.e. stateNonce + len(pending)).
func (s *ethSender) nextPendingNonce() uint64 {
	return s.stateNonce + uint64(len(s.pending))
}

// place slots txObj into pending or queue based on its nonce relative to
// stateNonce. Returns the existing tx it replaces (if any) so the caller can
// update secondary indexes (allByHash) and emit events accordingly.
//
// Replacement policy (PoC): a colliding (sender, nonce) slot is replaced only
// if BOTH maxPriorityFeePerGas and maxFeePerGas are strictly greater than the
// incumbent's. Real EIP-1559 10% bump is deferred to production.
func (s *ethSender) place(txObj *TxObject) (replaced *TxObject, accepted bool) {
	nonce := txObj.Nonce()
	if nonce < s.stateNonce {
		// stale — below the chain's expected next nonce
		return nil, false
	}

	if nonce < s.nextPendingNonce() {
		// collision with an existing pending slot — candidate for replacement
		incumbent := s.pending[nonce]
		if !isFeeBumpSufficient(incumbent, txObj) {
			return nil, false
		}
		s.pending[nonce] = txObj
		return incumbent, true
	}

	if nonce == s.nextPendingNonce() {
		s.pending[nonce] = txObj
		s.promoteFromQueue()
		return nil, true
	}

	// future nonce — goes into queue (or replaces a queued entry at the same nonce)
	if incumbent, ok := s.queue[nonce]; ok {
		if !isFeeBumpSufficient(incumbent, txObj) {
			return nil, false
		}
		s.queue[nonce] = txObj
		return incumbent, true
	}
	s.queue[nonce] = txObj
	return nil, true
}

// promoteFromQueue walks the queue and moves any contiguous entries into the
// pending chain. Call this whenever pending grows or the queue gains a new
// head-aligned entry.
func (s *ethSender) promoteFromQueue() {
	for {
		next := s.nextPendingNonce()
		qtx, ok := s.queue[next]
		if !ok {
			return
		}
		delete(s.queue, next)
		s.pending[next] = qtx
	}
}

// dropNonce removes a specific nonce from either pending or queue. After a
// pending drop the chain fragments — subsequent pending nonces are demoted
// back into the queue to preserve the pending-is-contiguous invariant.
// Returns the removed TxObject (or nil if not present).
func (s *ethSender) dropNonce(nonce uint64) *TxObject {
	if txObj, ok := s.pending[nonce]; ok {
		delete(s.pending, nonce)
		// Anything with nonce > dropped nonce is no longer contiguous; demote.
		for n, pt := range s.pending {
			if n > nonce {
				delete(s.pending, n)
				s.queue[n] = pt
			}
		}
		return txObj
	}
	if txObj, ok := s.queue[nonce]; ok {
		delete(s.queue, nonce)
		return txObj
	}
	return nil
}

// bumpStateNonce is called on chain-head reset when a block includes an Eth tx
// from this sender with nonce N. All pending entries with nonce <= N are now
// settled and must be evicted. Any contiguity gap created this way leaves
// queue entries stranded until another bump or a pending re-fill occurs.
// Returns the list of evicted txObjects so the caller can remove them from
// allByHash.
func (s *ethSender) bumpStateNonce(newStateNonce uint64) []*TxObject {
	if newStateNonce <= s.stateNonce {
		return nil
	}

	var evicted []*TxObject
	for n, pt := range s.pending {
		if n < newStateNonce {
			delete(s.pending, n)
			evicted = append(evicted, pt)
		}
	}
	for n, qt := range s.queue {
		if n < newStateNonce {
			delete(s.queue, n)
			evicted = append(evicted, qt)
		}
	}
	s.stateNonce = newStateNonce
	// Some queue entries might now be pending-aligned.
	s.promoteFromQueue()
	return evicted
}

// sortedPending returns pending txs in ascending nonce order. O(n log n); n is
// small (bounded by per-account pending limit).
func (s *ethSender) sortedPending() []*TxObject {
	if len(s.pending) == 0 {
		return nil
	}
	out := make([]*TxObject, 0, len(s.pending))
	for _, pt := range s.pending {
		out = append(out, pt)
	}
	slices.SortFunc(out, func(a, b *TxObject) int {
		if a.Nonce() < b.Nonce() {
			return -1
		}
		if a.Nonce() > b.Nonce() {
			return 1
		}
		return 0
	})
	return out
}

// empty reports whether the sender holds no pending or queued txs. The ethPoolMap
// garbage-collects empty sender entries unconditionally; the canonical chain state
// (passed as chainNonce to ensureSender/poolNonce) re-seeds the correct stateNonce
// whenever the sender reappears.
func (s *ethSender) empty() bool {
	return len(s.pending) == 0 && len(s.queue) == 0
}

// isFeeBumpSufficient implements the PoC replacement rule: a candidate must
// strictly exceed the incumbent on BOTH maxPriorityFeePerGas and maxFeePerGas.
// EthereumTx types are always 1559-style here, so both fields are present.
func isFeeBumpSufficient(incumbent, candidate *TxObject) bool {
	if incumbent == nil {
		return true
	}
	if candidate.MaxPriorityFeePerGas().Cmp(incumbent.MaxPriorityFeePerGas()) <= 0 {
		return false
	}
	if candidate.MaxFeePerGas().Cmp(incumbent.MaxFeePerGas()) <= 0 {
		return false
	}
	return true
}
