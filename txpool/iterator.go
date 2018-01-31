package txpool

import (
	"github.com/vechain/thor/tx"
)

//Iterator Iterator
type Iterator struct {
	data  TxObjects
	index int
}

//NewIterator constructor
func newIterator(data TxObjects) *Iterator {
	return &Iterator{
		data:  data,
		index: 0,
	}
}

//HasNext whether has next
func (i *Iterator) HasNext() bool {
	return i.index < i.data.Len()
}

//Next Next
func (i *Iterator) Next() *tx.Transaction {
	obj := i.data[i.index]
	i.index++
	return obj.Transaction()
}
