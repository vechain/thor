// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

// ethSender tracks per-account nonce state for Ethereum-family transactions.
// Concrete place/promote/drop logic will be added in a follow-up.
type ethSender struct {
	stateNonce uint64
	pending    map[uint64]*TxObject
	queue      map[uint64]*TxObject
}

// poolNonce returns the next expected nonce (stateNonce + contiguous pending).
func (s *ethSender) poolNonce() uint64 {
	return s.stateNonce + uint64(len(s.pending))
}

func (s *ethSender) isEmpty() bool {
	return len(s.pending) == 0 && len(s.queue) == 0
}
