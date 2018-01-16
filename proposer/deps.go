package proposer

import (
	"github.com/vechain/thor/tx"
)

type TxFeed interface {
	Next() *tx.Transaction
}
