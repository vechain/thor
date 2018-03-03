package chain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/lvldb"
)

func TestTraverser(t *testing.T) {
	s, _ := lvldb.NewMem()
	c := chain.New(s)
	b0 := new(block.Builder).Build()
	c.WriteGenesis(b0)

	b1 := new(block.Builder).ParentID(b0.Header().ID()).Build()
	b2 := new(block.Builder).ParentID(b1.Header().ID()).Build()
	b3 := new(block.Builder).ParentID(b2.Header().ID()).Build()
	b2x := new(block.Builder).ParentID(b1.Header().ID()).Build()
	b3x := new(block.Builder).ParentID(b2x.Header().ID()).Build()

	c.AddBlock(b1, true)
	c.AddBlock(b2, true)
	c.AddBlock(b3, true)
	c.AddBlock(b2x, false)
	c.AddBlock(b3x, false)

	tr := c.NewTraverser(b3x.Header().ID())
	assert.Equal(t, tr.Get(3).ID(), b3x.Header().ID())
	assert.Equal(t, tr.Get(2).ID(), b2x.Header().ID())
	assert.Equal(t, tr.Get(1).ID(), b1.Header().ID())
	assert.Equal(t, tr.Get(0).ID(), b0.Header().ID())
	assert.Nil(t, nil, tr.Error())
}
