package thor_test

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/thor"
)

func BenchmarkProvedWorkToEnergy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		thor.ProvedWorkToEnergy(big.NewInt(100000), 18*30*3600*24/10)
	}
}
