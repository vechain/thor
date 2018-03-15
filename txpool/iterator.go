package txpool

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Iterator Iterator
type Iterator struct {
	objs  TxObjects
	pool  *TxPool
	index int
}

//NewIterator constructor
func newIterator(objs TxObjects, pool *TxPool) *Iterator {
	return &Iterator{
		objs:  objs,
		pool:  pool,
		index: 0,
	}
}

//HasNext HasNext
func (i *Iterator) HasNext() bool {
	return i.index < len(i.objs)
}

//Next Next
func (i *Iterator) Next() *tx.Transaction {
	obj := i.objs[i.index]
	i.index++
	return obj.Transaction()
}

//OnProcessed OnProcessed
func (i *Iterator) OnProcessed(txID thor.Hash, err error) {
	i.pool.Remove(txID)
}
