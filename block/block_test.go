package block_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/vechain/vecore/block"
)

func TestA(t *testing.T) {
	bb := block.Builder{}
	bb.GasUsed(big.NewInt(1))
	block := bb.Build()
	fmt.Println(block.HashForSigning().String())
}
