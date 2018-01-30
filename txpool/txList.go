package txpool

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"sort"
	"time"
)

type txList struct {
	txs            map[thor.Hash]*transaction // the map of all transactions
	items          transactions               // Heap of prices of all the stored transactions
	discardCounter int
}

// newTxList creates a new price-sorted transaction heap.
func newTxList() *txList {
	return &txList{
		txs:   make(map[thor.Hash]*transaction),
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
	if _, ok := l.txs[tx.ID()]; ok {
		return true
	}
	return false
}

func (l *txList) Delete(tx *tx.Transaction) {
	txID := tx.ID()
	if _, ok := l.txs[txID]; !ok {
		return
	}
	delete(l.txs, txID)
	l.discardCounter++
}

func (l *txList) AddTransaction(tx *tx.Transaction, converstion uint64) {
	trans := newTransaction(tx, converstion, time.Now().Unix())
	txID := tx.ID()
	if _, ok := l.txs[txID]; !ok {
		l.items.Push(trans)
	}
	l.txs[txID] = trans
}

func (l *txList) Reset() {
	if l.discardCounter < l.Len()/4 {
		return
	}
	all := l.txs
	txs := make(transactions, len(all))
	for _, tx := range all {
		txs.Push(tx)
	}
	l.items = txs
}

// func (l *txList) List() transactions {

// }
