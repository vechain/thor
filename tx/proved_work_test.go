package tx_test

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/tx"
)

func BenchmarkProvedWorkToEnergy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tx.ProvedWorkToEnergy(big.NewInt(100000), 18*30*3600*24/10, 10)
	}
}
