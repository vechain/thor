// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package subscriptions

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type pendingTx struct {
	txPool    *txpool.TxPool
	listeners map[chan *tx.Transaction]struct{}
	mu        sync.Mutex
}

func newPendingTx(txPool *txpool.TxPool) *pendingTx {
	p := &pendingTx{
		txPool:    txPool,
		listeners: make(map[chan *tx.Transaction]struct{}),
	}

	return p
}

func (p *pendingTx) Subscribe(ch chan *tx.Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.listeners[ch] = struct{}{}
}

func (p *pendingTx) Unsubscribe(ch chan *tx.Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.listeners, ch)
}

func (p *pendingTx) DispatchLoop(done <-chan struct{}) {
	txCh := make(chan *txpool.TxEvent)
	sub := p.txPool.SubscribeTxEvent(txCh)
	defer sub.Unsubscribe()

	knownTx, _ := simplelru.NewLRU(2000, nil)

	for {
		select {
		case txEv := <-txCh:
			if txEv.Executable == nil || !*txEv.Executable {
				continue
			}
			now := time.Now().Unix()
			// ignored if seen within half block interval
			if seen, ok := knownTx.Get(txEv.Tx.ID()); ok && now-seen.(int64) <= int64(thor.BlockInterval/2) {
				continue
			}
			knownTx.Add(txEv.Tx.ID(), now)

			p.dispatch(txEv.Tx, done)
		case <-done:
			return
		}
	}
}

func (p *pendingTx) dispatch(tx *tx.Transaction, done <-chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for lsn := range p.listeners {
		select {
		case lsn <- tx:
		case <-done:
			return
		default: // broadcast in a non-blocking manner, so there's no guarantee that all subscriber receives it
		}
	}
}
