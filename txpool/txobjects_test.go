package txpool_test

import (
	"fmt"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	"math/big"
	"sort"
	"testing"
)

func TestObjs(t *testing.T) {
	objs := make(txpool.TxObjects, 0)
	for i := 0; i < 10; i++ {
		t := new(tx.Builder).Gas(1000 + uint64(i)).GasPrice(big.NewInt(1)).Build()
		obj := txpool.NewTxObject(t, big.NewInt(1000000), 1)
		objs.Push(obj)
	}
	sort.Sort(objs)
	for _, obj := range objs {
		fmt.Println(obj.Cost())
	}
}
