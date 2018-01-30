package txpool

import (
	"github.com/vechain/thor/tx"
)

type transaction struct {
	addTime    int64
	conversion uint64
	tx         *tx.Transaction
}

func newTransaction(tx *tx.Transaction, conversion uint64, addTime int64) *transaction {
	return &transaction{
		addTime,
		conversion,
		tx,
	}
}
