package block_test

import (
	"fmt"
	"math/big"
	"testing"

	. "github.com/vechain/vecore/block"
)

func TestBlock(t *testing.T) {

	builder := Builder{}
	block := builder.GasUsed(big.NewInt(1000)).Build()
	h := block.Header()
	fmt.Println(h.Hash())
}
