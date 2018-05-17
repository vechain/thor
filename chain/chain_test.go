// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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
		best := chain.BestBlock()
		b := new(block.Builder).
			ParentID(best.Header().ID()).
			Build()
		fmt.Println(b.Header().ID())
		if _, err := chain.AddBlock(b, nil); err != nil {
			fmt.Println(err)
		}
	}
	fmt.Println(chain.BestBlock().Header().Number())
}
