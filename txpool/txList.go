package txpool

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"sort"
)

type txList struct {
	objs           map[thor.Hash]*TxObject // the map of all TxObjects
	items          TxObjects               // price-sorted TxObjects
	discardCounter int
}

// newTxList creates a new price-sorted TxObjects.
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

func (l *txList) Index(i int) *TxObject {
	if l.Len() == 0 {
		return nil
	}
	txs := l.items
	return txs[i]
}

func (l *txList) DiscardTail(count int) {
	if l.Len() < count {
		return
	}
	for i := l.Len() - count; i < l.Len(); i++ {
		l.Delete(l.items[i])
	}
}

func (l *txList) IsExists(tx *tx.Transaction) bool {
	if _, ok := l.objs[tx.ID()]; ok {
		return true
	}
	return false
}

func (l *txList) Delete(obj *TxObject) {
	txID := obj.tx.ID()
	if _, ok := l.objs[txID]; !ok {
		return
	}
	delete(l.objs, txID)
	l.discardCounter++
}

func (l *txList) AddTxObject(obj *TxObject) {
	txID := obj.tx.ID()
	if _, ok := l.objs[txID]; !ok {
		l.items.Push(obj)
	}
	l.objs[txID] = obj
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

//GetObj
func (l *txList) GetObj(objID thor.Hash) *TxObject {
	return l.objs[objID]
}
