package txpool

import (
	"fmt"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"sort"
	"time"
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
	objID := obj.Transaction().ID()
	if _, ok := l.objs[objID]; !ok {
		return
	}
	delete(l.objs, objID)
	l.discardCounter++
}

func (l *txList) AddTxObject(obj *TxObject) {
	objID := obj.Transaction().ID()
	if _, ok := l.objs[objID]; !ok {
		l.items.Push(obj)
	}
	l.objs[objID] = obj
}

func (l *txList) Reset(lifetime int64) {
	fmt.Println("l.discardCounter:", l.discardCounter, "l.Len()", l.Len())
	if l.discardCounter > l.Len()/4 {
		return
	}
	all := l.objs
	objs := make(TxObjects, len(all))
	for key, obj := range all {
		if time.Now().Unix()-obj.CreateTime() > lifetime {
			delete(all, key)
			continue
		}
		objs.Push(obj)
	}
	l.items, l.discardCounter = objs, 0
}

//GetObj
func (l *txList) GetObj(objID thor.Hash) *TxObject {
	return l.objs[objID]
}
