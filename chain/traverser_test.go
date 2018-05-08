package chain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestTraverser(t *testing.T) {
	s, _ := lvldb.NewMem()
	b0, _, _ := new(genesis.Builder).Build(state.NewCreator(s))
	c, err := chain.New(s, b0)
	assert.Nil(t, err)

	b1 := new(block.Builder).ParentID(b0.Header().ID()).TotalScore(1).Build()
	b2 := new(block.Builder).ParentID(b1.Header().ID()).TotalScore(1).Build()
	b3 := new(block.Builder).ParentID(b2.Header().ID()).TotalScore(1).Build()
	b2x := new(block.Builder).ParentID(b1.Header().ID()).Build()
	b3x := new(block.Builder).ParentID(b2x.Header().ID()).Build()

	c.AddBlock(b1, nil)
	c.AddBlock(b2, nil)
	c.AddBlock(b3, nil)
	c.AddBlock(b2x, nil)
	c.AddBlock(b3x, nil)

	tr := c.NewTraverser(b3x.Header().ID())
	assert.Equal(t, tr.Get(3).ID(), b3x.Header().ID())
	assert.Equal(t, tr.Get(2).ID(), b2x.Header().ID())
	assert.Equal(t, tr.Get(1).ID(), b1.Header().ID())
	assert.Equal(t, tr.Get(0).ID(), b0.Header().ID())

	assert.Nil(t, nil, tr.Error())
}
