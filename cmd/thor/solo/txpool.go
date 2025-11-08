// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type OnDemandTxPool struct {
	engine *Core

	txsByID map[thor.Bytes32]*tx.Transaction

	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   co.Goes

	mu sync.Mutex
}

func NewOnDemandTxPool(engine *Core) *OnDemandTxPool {
	return &OnDemandTxPool{
		engine:  engine,
		txsByID: make(map[thor.Bytes32]*tx.Transaction),
	}
}

var _ txpool.Pool = (*OnDemandTxPool)(nil)

func (o *OnDemandTxPool) Get(txID thor.Bytes32) *tx.Transaction {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.txsByID[txID]
}

func (o *OnDemandTxPool) Add(newTx *tx.Transaction) error {
	return o.AddLocal(newTx)
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

	if executable {
		toRemove, err := o.engine.Pack(tx.Transactions{newTx}, true)
		if err != nil {
			return err
		}
		for _, tx := range toRemove {
			delete(o.txsByID, tx.ID())
		}
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

func (o *OnDemandTxPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.txsByID, txID)
	return true
}

func (o *OnDemandTxPool) Close() {}

func (o *OnDemandTxPool) Fill(txs tx.Transactions) {}

func (o *OnDemandTxPool) StrictlyAdd(newTx *tx.Transaction) error {
	return o.Add(newTx)
}
