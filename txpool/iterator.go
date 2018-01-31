package txpool

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Iterator Iterator
type Iterator struct {
	pool  *TxPool
	data  TxObjects
	index int
}

//NewIterator constructor
func newIterator(data TxObjects, pool *TxPool) *Iterator {
	return &Iterator{
		pool:  pool,
		data:  data,
		index: 0,
	}
}

//HasNext HasNext
func (i *Iterator) HasNext() bool {
	return i.index < i.data.Len()
}

//Next Next
func (i *Iterator) Next() *tx.Transaction {
	obj := i.data[i.index]
	i.index++
	return obj.Transaction()
}

//OnProcessed OnProcessed
func (i *Iterator) OnProcessed(txID thor.Hash, err error) {
	i.pool.m.Lock()
	defer i.pool.m.Unlock()
	if err != nil {
		if _, ok := i.pool.all[txID]; ok {
			delete(i.pool.all, txID)
		}
	}
}
