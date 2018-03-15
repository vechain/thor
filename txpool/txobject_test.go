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
	for i := 0; i < 10000; i++ {
		t := new(tx.Builder).Gas(100000 - uint64(i)).GasPriceCoef(uint8(i)).Build()
		obj := txpool.NewTxObject(t, 1)
		obj.SetOverallGP(big.NewInt(int64(i)))
		objs = append(objs, obj)
	}
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].OverallGP().Cmp(objs[j].OverallGP()) > 0
	})
	for _, obj := range objs {
		fmt.Println(obj.OverallGP())
	}
}
func BenchmarkSort(b *testing.B) {
	objs := make(txpool.TxObjects, 0)
	for i := 0; i < 10000; i++ {
		t := new(tx.Builder).Gas(100000 - uint64(i)).GasPriceCoef(uint8(i)).Build()
		obj := txpool.NewTxObject(t, 1)
		obj.SetOverallGP(big.NewInt(int64(i)))
		objs = append(objs, obj)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sort.Slice(objs, func(i, j int) bool {
			return objs[i].OverallGP().Cmp(objs[j].OverallGP()) > 0
		})
	}
}
