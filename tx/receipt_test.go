package tx_test

import (
	"fmt"
	"testing"

	. "github.com/vechain/thor/tx"
)

func TestReceipt(t *testing.T) {
	var rs Receipts
	fmt.Println(rs.RootHash())

	var txs Transactions
	fmt.Println(txs.RootHash())
}
