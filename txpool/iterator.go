package txpool

import (
	"github.com/vechain/thor/tx"
)

//Iterator Iterator
type Iterator struct {
	data  *txList
	index int
}

//NewIterator constructor
func newIterator(data *txList) *Iterator {
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
	v := i.data.Index(i.index)
	i.index++
	return v.Transaction()
}
