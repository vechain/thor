package packer

import (
	"github.com/vechain/thor/tx"
)

type TxFeed interface {
	Next() *tx.Transaction
	MarkTxBad(tx *tx.Transaction)
}
