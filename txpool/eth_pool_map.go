// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"sync"

	"github.com/vechain/thor/v2/thor"
)

// ethPoolMap is a thread-safe index of Ethereum-family pooled transactions.
// Mutation helpers will be added when EthPool admission is implemented.
type ethPoolMap struct {
	lock      sync.RWMutex
	allByHash map[thor.Bytes32]*TxObject
	senders   map[thor.Address]*ethSender
}

func newEthPoolMap() *ethPoolMap {
	return &ethPoolMap{
		allByHash: make(map[thor.Bytes32]*TxObject),
		senders:   make(map[thor.Address]*ethSender),
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
	m.lock.RLock()
	defer m.lock.RUnlock()
	if s := m.senders[addr]; s != nil {
		return s.poolNonce()
	}
	return 0
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
