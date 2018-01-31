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

func (i *Iterator) hasNext() bool {
	return i.index < i.data.Len()
}

func (i *Iterator) next() *tx.Transaction {
	obj := i.data[i.index]
	i.index++
	return obj.Transaction()
}
