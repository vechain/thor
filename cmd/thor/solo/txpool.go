package solo

import (
	"sync"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type OnDemandTxPool struct {
	engine *Engine

	txsByHash map[thor.Bytes32]*tx.Transaction
	txsByID   map[thor.Bytes32]*tx.Transaction

	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   co.Goes

	mu sync.Mutex
}

func NewOnDemandTxPool(engine *Engine) *OnDemandTxPool {
	return &OnDemandTxPool{
		engine:    engine,
		txsByHash: make(map[thor.Bytes32]*tx.Transaction),
		txsByID:   make(map[thor.Bytes32]*tx.Transaction),
	}
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

	o.txsByHash[newTx.Hash()] = newTx
	o.txsByID[newTx.ID()] = newTx

	executable, err := o.engine.IsExecutable(newTx)
	if err != nil {
		return err
	}

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
			delete(o.txsByHash, tx.Hash())
			delete(o.txsByID, tx.ID())
		}
	}

	return nil
}

func (o *OnDemandTxPool) Dump() tx.Transactions {
	o.mu.Lock()
	defer o.mu.Unlock()

	txs := make(tx.Transactions, 0, len(o.txsByHash))
	for _, tx := range o.txsByHash {
		txs = append(txs, tx)
	}
	return txs
}

func (o *OnDemandTxPool) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	return len(o.txsByHash)
}

func (o *OnDemandTxPool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	return o.scope.Track(o.txFeed.Subscribe(ch))
}

func (o *OnDemandTxPool) Executables() tx.Transactions {
	o.mu.Lock()
	defer o.mu.Unlock()

	txs := make(tx.Transactions, 0, len(o.txsByHash))
	for _, tx := range o.txsByHash {
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

	delete(o.txsByHash, txHash)
	delete(o.txsByID, txID)

	return true
}
