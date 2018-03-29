package chain_test

import (
	"fmt"
	"testing"

	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/chain"
	"github.com/vechain/thor/lvldb"
)

func TestChain(t *testing.T) {

	s, _ := lvldb.NewMem()
	chain, _ := New(s, new(block.Builder).Build())

	for i := 0; i < 100; i++ {
		best, _ := chain.GetBestBlock()
		b := new(block.Builder).
			ParentID(best.Header().ID()).
			Build()
		fmt.Println(b.Header().ID())
		if _, err := chain.AddBlock(b, nil, true); err != nil {
			fmt.Println(err)
		}
	}
	best, _ := chain.GetBestBlock()
	fmt.Println(best.Header().Number())
}
