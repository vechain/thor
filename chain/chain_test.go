package chain_test

import (
	"fmt"
	"testing"

	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/chain"
	"github.com/vechain/thor/kv"
)

func TestChain(t *testing.T) {

	s, _ := kv.NewMem(kv.Options{})
	chain := New(s)
	chain.WriteGenesis(new(block.Builder).Build())

	for i := 0; i < 100; i++ {
		best, _ := chain.GetBestBlock()
		b := new(block.Builder).
			ParentHash(best.Hash()).
			Build()
		fmt.Println(b.Hash())
		if err := chain.AddBlock(b, true); err != nil {
			fmt.Println(err)
		}
	}
	best, _ := chain.GetBestBlock()
	fmt.Println(best.Number())
}
