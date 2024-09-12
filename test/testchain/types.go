package testchain

import "github.com/vechain/thor/v2/tx"

type TxAndRcpt struct {
	Transaction *tx.Transaction
	ReceiptFunc func(tx.Receipts)
}
