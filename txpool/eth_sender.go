// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import "math/big"

import "github.com/vechain/thor/v2/thor"

// ethSender tracks per-account nonce state for Ethereum-family transactions.
type ethSender struct {
	origin     thor.Address
	stateNonce uint64
	pending    map[uint64]*TxObject
	queue      map[uint64]*TxObject
}

func newEthSender(origin thor.Address, stateNonce uint64) *ethSender {
	return &ethSender{
		origin:     origin,
		stateNonce: stateNonce,
		pending:    make(map[uint64]*TxObject),
		queue:      make(map[uint64]*TxObject),
	}
}

// poolNonce returns the next expected nonce (stateNonce + contiguous pending).
func (s *ethSender) poolNonce() uint64 {
	return s.stateNonce + uint64(len(s.pending))
}

func (s *ethSender) isEmpty() bool {
	return len(s.pending) == 0 && len(s.queue) == 0
}

func (s *ethSender) get(nonce uint64) *TxObject {
	if txObj := s.pending[nonce]; txObj != nil {
		return txObj
	}
	return s.queue[nonce]
}

func (s *ethSender) isPending(nonce uint64) bool {
	return s.pending[nonce] != nil
}

func (s *ethSender) pendingCountFrom(nonce uint64) int {
	count := 0
	for pendingNonce := range s.pending {
		if pendingNonce >= nonce {
			count++
		}
	}
	return count
}

func (s *ethSender) demoteFrom(nonce uint64) []reservationOwner {
	var releases []reservationOwner
	for pendingNonce, txObj := range s.pending {
		if pendingNonce < nonce {
			continue
		}
		txObj.executable = false
		s.queue[pendingNonce] = txObj
		delete(s.pending, pendingNonce)
		releases = append(releases, ethReservationOwner(s.origin, pendingNonce))
	}
	return releases
}

// syncStateNonce reconciles the sender with the canonical account nonce. It
// returns transactions which became settled and reservation owners which must
// be released. A backwards move (reorg) conservatively queues all transactions
// until the newly-created nonce gap is filled.
func (s *ethSender) syncStateNonce(stateNonce uint64) (settled []*TxObject, releases []reservationOwner) {
	if stateNonce < s.stateNonce {
		for nonce, txObj := range s.pending {
			txObj.executable = false
			s.queue[nonce] = txObj
			delete(s.pending, nonce)
			releases = append(releases, ethReservationOwner(s.origin, nonce))
		}
		s.stateNonce = stateNonce
		return settled, releases
	}
	if stateNonce == s.stateNonce {
		return nil, nil
	}
	for nonce, txObj := range s.pending {
		if nonce < stateNonce {
			settled = append(settled, txObj)
			delete(s.pending, nonce)
			releases = append(releases, ethReservationOwner(s.origin, nonce))
		}
	}
	for nonce, txObj := range s.queue {
		if nonce < stateNonce {
			settled = append(settled, txObj)
			delete(s.queue, nonce)
		}
	}
	s.stateNonce = stateNonce
	return settled, releases
}

func isFeeBumpSufficient(incumbent, candidate *TxObject, priceBump uint64) bool {
	if incumbent == nil {
		return true
	}
	return feeBumped(incumbent.MaxFeePerGas(), candidate.MaxFeePerGas(), priceBump) &&
		feeBumped(incumbent.MaxPriorityFeePerGas(), candidate.MaxPriorityFeePerGas(), priceBump)
}

func feeBumped(old, candidate *big.Int, priceBump uint64) bool {
	if candidate.Cmp(old) <= 0 {
		return false
	}
	threshold := new(big.Int).Mul(old, new(big.Int).SetUint64(100+priceBump))
	threshold.Div(threshold, new(big.Int).SetUint64(100))
	return candidate.Cmp(threshold) >= 0
}
