package receipt_test

import (
	"fmt"
	"testing"

	. "github.com/vechain/thor/tx/receipt"
)

func TestReceipt(t *testing.T) {
	var rs Receipts
	rs = append(rs, &Receipt{})

	fmt.Println(rs.RootHash().String())
}
