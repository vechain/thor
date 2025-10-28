// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type OnDemandTxPool struct {
	engine *Core

	txsByID     map[thor.Bytes32]*tx.Transaction
	bufferedTxs []*tx.Transaction

	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   co.Goes

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.Mutex
}

func NewOnDemandTxPool(engine *Core) *OnDemandTxPool {
	ctx, cancel := context.WithCancel(context.Background())
	o := &OnDemandTxPool{
		engine:      engine,
		txsByID:     make(map[thor.Bytes32]*tx.Transaction),
		bufferedTxs: make([]*tx.Transaction, 0),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start background goroutine for periodic packing
	go o.packingLoop()

	return o
}

var _ TxPool = (*OnDemandTxPool)(nil)

func (o *OnDemandTxPool) Get(txID thor.Bytes32) *tx.Transaction {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.txsByID[txID]
}

func (o *OnDemandTxPool) AddLocal(newTx *tx.Transaction) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if newTx.ChainTag() != o.engine.repo.ChainTag() {
		return restutil.BadRequest(errors.New("bad tx: chain tag mismatch"))
	}

	if newTx.Size() > txpool.MaxTxSize {
		return restutil.Forbidden(errors.New("tx rejected: size too large"))
	}
	executable, err := o.engine.IsExecutable(newTx)
	if err != nil {
		// simulate API call for adding a transaction that gets rejected
		return restutil.Forbidden(errors.New("tx rejected: " + err.Error()))
	}

	o.txsByID[newTx.ID()] = newTx
	o.goes.Go(func() {
		o.txFeed.Send(&txpool.TxEvent{
			Tx:         newTx,
			Executable: &executable,
		})
	})

	// Add executable transactions to buffer instead of immediate packing
	if executable {
		o.bufferedTxs = append(o.bufferedTxs, newTx)
	}

	return nil
}

func (o *OnDemandTxPool) Dump() tx.Transactions {
	o.mu.Lock()
	defer o.mu.Unlock()

	txs := make(tx.Transactions, 0, len(o.txsByID))
	for _, tx := range o.txsByID {
		txs = append(txs, tx)
	}
	return txs
}

func (o *OnDemandTxPool) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	return len(o.txsByID)
}

func (o *OnDemandTxPool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	return o.scope.Track(o.txFeed.Subscribe(ch))
}

func (o *OnDemandTxPool) Executables() tx.Transactions {
	o.mu.Lock()
	defer o.mu.Unlock()

	txs := make(tx.Transactions, 0, len(o.txsByID))
	for _, tx := range o.txsByID {
		executable, err := o.engine.IsExecutable(tx)
		if err != nil {
			continue
		}
		if executable {
			txs = append(txs, tx)
		}
	}

	return txs
}

func (o *OnDemandTxPool) packingLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.tryPack()
		}
	}
}

func (o *OnDemandTxPool) tryPack() {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check if we have buffered transactions
	if len(o.bufferedTxs) == 0 {
		return
	}

	// Check if at least 1 second has passed since the last block
	bestBlock := o.engine.repo.BestBlockSummary()
	now := uint64(time.Now().Unix())
	if now < bestBlock.Header.Timestamp()+1 {
		return
	}

	// Pack all buffered transactions
	toRemove, err := o.engine.Pack(o.bufferedTxs, true)
	if err != nil {
		log.Error("failed to pack buffered transactions", "error", err)
	}

	// Remove packed transactions from both buffer and txsByID
	for _, txToRemove := range toRemove {
		delete(o.txsByID, txToRemove.ID())
		// Remove from buffer
		for i, bufferedTx := range o.bufferedTxs {
			if bufferedTx.ID() == txToRemove.ID() {
				o.bufferedTxs = slices.Delete(o.bufferedTxs, i, i+1)
				break
			}
		}
	}
}

func (o *OnDemandTxPool) Close() {
	o.cancel()
}

func (o *OnDemandTxPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.txsByID, txID)

	// Also remove from buffer if present
	for i, bufferedTx := range o.bufferedTxs {
		if bufferedTx.ID() == txID {
			o.bufferedTxs = slices.Delete(o.bufferedTxs, i, i+1)
			break
		}
	}

	return true
}
