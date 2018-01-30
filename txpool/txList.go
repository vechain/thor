package txpool

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
	"sort"
	"time"
)

type txList struct {
	objs           map[thor.Hash]*TxObject // the map of all transactions
	items          TxObjects               // Heap of prices of all the stored transactions
	discardCounter int
}

// newTxList creates a new price-sorted transaction heap.
func newTxList() *txList {
	return &txList{
		objs:  make(map[thor.Hash]*TxObject),
		items: nil,
	}
}

func (l *txList) SortByPrice() {
	sort.Sort(l.items)
}

func (l *txList) Len() int {
	return len(l.items)
}

func (l *txList) Index(i int) *tx.Transaction {
	if l.Len() == 0 {
		return nil
	}
	txs := l.items
	return txs[i].tx
}

func (l *txList) DiscardTail(count int) {
	if l.Len() < count {
		return
	}
	for i := l.Len() - count; i < l.Len(); i++ {
		l.Delete(l.items[i].tx)
	}
}

func (l *txList) IsExists(tx *tx.Transaction) bool {
	if _, ok := l.objs[tx.ID()]; ok {
		return true
	}
	return false
}

func (l *txList) Delete(tx *tx.Transaction) {
	txID := tx.ID()
	if _, ok := l.objs[txID]; !ok {
		return
	}
	delete(l.objs, txID)
	l.discardCounter++
}

func (l *txList) AddTransaction(tx *tx.Transaction, converstion *big.Int) {
	trans := NewTxObject(tx, converstion, time.Now().Unix())
	txID := tx.ID()
	if _, ok := l.objs[txID]; !ok {
		l.items.Push(trans)
	}
	l.objs[txID] = trans
}

func (l *txList) Reset() {
	if l.discardCounter > l.Len()/4 {
		return
	}
	all := l.objs
	objs := make(TxObjects, len(all))
	for _, tx := range all {
		objs.Push(tx)
	}
	l.items, l.discardCounter = objs, 0
}
