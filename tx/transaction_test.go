package tx_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/vechain/vecore/cry"
	"github.com/vechain/vecore/tx"
)

func TestTx(t *testing.T) {
	tb := tx.Builder{}

	tx1 := tb.Build()

	fmt.Println(tx1.Hash().String(), tx1.GasPrice())

	tx2 := tb.GasPrice(big.NewInt(1)).Build()
	fmt.Println(tx2.Hash().String(), tx2.GasPrice())

	txs := tx.Transactions{}
	fmt.Println(txs.RootHash().String())

	hw := sha3.NewKeccak256()
	hw.Write([]byte{1})
	var h cry.Hash

	hw.Sum(h[:])
	fmt.Println(h.String())
}
