// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"sync"

	"github.com/vechain/thor/v2/block"
)

// blockMailbox holds at most one pending block per peer, decoupling the
// producer (peer read loop) from the consumer (deliverBlocksLoop). A new block
// from the same peer overwrites the pending one instead of queuing.
type blockMailbox struct {
	mu     sync.Mutex
	slots  map[*Peer]*block.Block
	signal chan struct{} // cap 1, coalescing wakeup
}

func newBlockMailbox() *blockMailbox {
	return &blockMailbox{
		slots:  make(map[*Peer]*block.Block),
		signal: make(chan struct{}, 1),
	}
}

// put stores blk as the pending block for peer, overwriting any block
// already waiting for that peer. It reports whether an overwrite happened.
func (m *blockMailbox) put(peer *Peer, blk *block.Block) (overwritten bool) {
	m.mu.Lock()
	_, overwritten = m.slots[peer]
	m.slots[peer] = blk
	m.mu.Unlock()

	select {
	case m.signal <- struct{}{}:
	default:
	}
	return
}

// take swaps out the whole pending table and returns it.
func (m *blockMailbox) take() map[*Peer]*block.Block {
	m.mu.Lock()
	defer m.mu.Unlock()
	batch := m.slots
	m.slots = make(map[*Peer]*block.Block)
	return batch
}

// remove clears the pending slot of peer, if any. It is best effort: a block
// the peer's read loop is still handling may be put afterwards, and is then
// delivered with the next batch.
func (m *blockMailbox) remove(peer *Peer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.slots, peer)
}
