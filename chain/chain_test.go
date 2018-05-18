// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func initChain() *chain.Chain {
	kv, _ := lvldb.NewMem()
	g, _ := genesis.NewDevnet()
	b0, _, _ := g.Build(state.NewCreator(kv))

	chain, err := chain.New(kv, b0)
	if err != nil {
		panic(err)
	}
	return chain
}

var privateKey, _ = crypto.GenerateKey()

func newBlock(parent *block.Block, score uint64) *block.Block {
	b := new(block.Builder).ParentID(parent.Header().ID()).TotalScore(parent.Header().TotalScore() + score).Build()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
	return b.WithSignature(sig)
}

func TestAdd(t *testing.T) {
	ch := initChain()
	b0 := ch.GenesisBlock()
	b1 := newBlock(b0, 1)
	b2 := newBlock(b1, 1)
	b3 := newBlock(b2, 1)
	b4 := newBlock(b3, 1)
	b4x := newBlock(b3, 2)

	tests := []struct {
		newBlock *block.Block
		fork     *chain.Fork
		best     *block.Block
	}{
		{b1, &chain.Fork{Ancestor: b0, Trunk: []*block.Block{b1}}, b1},
		{b2, &chain.Fork{Ancestor: b1, Trunk: []*block.Block{b2}}, b2},
		{b3, &chain.Fork{Ancestor: b2, Trunk: []*block.Block{b3}}, b3},
		{b4, &chain.Fork{Ancestor: b3, Trunk: []*block.Block{b4}}, b4},
		{b4x, &chain.Fork{Ancestor: b3, Trunk: []*block.Block{b4x}, Branch: []*block.Block{b4}}, b4x},
	}

	for _, tt := range tests {
		fork, err := ch.AddBlock(tt.newBlock, nil)
		assert.Nil(t, err)
		assert.Equal(t, tt.best.Header().ID(), ch.BestBlock().Header().ID())

		assert.Equal(t, tt.fork.Ancestor.Header().ID(), fork.Ancestor.Header().ID())
		assert.Equal(t, len(tt.fork.Branch), len(fork.Branch))
		assert.Equal(t, len(tt.fork.Trunk), len(fork.Trunk))
		for i, b := range fork.Branch {
			assert.Equal(t, tt.fork.Branch[i].Header().ID(), b.Header().ID())
		}
		for i, b := range fork.Trunk {
			assert.Equal(t, tt.fork.Trunk[i].Header().ID(), b.Header().ID())
		}
	}
}
