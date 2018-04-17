package chain_test

import (
	"fmt"
	"testing"

	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestChain(t *testing.T) {

	s, _ := lvldb.NewMem()
	g, _ := genesis.NewDevnet()
	blk, _, _ := g.Build(state.NewCreator(s))
	chain, _ := New(s, blk)

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
