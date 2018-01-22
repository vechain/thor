package chain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestBlockIDGetter(t *testing.T) {
	s, _ := lvldb.NewMem()
	c := chain.New(s)
	b0 := new(block.Builder).Build()
	c.WriteGenesis(b0)

	b1 := new(block.Builder).ParentID(b0.ID()).Build()
	b2 := new(block.Builder).ParentID(b1.ID()).Build()
	b3 := new(block.Builder).ParentID(b2.ID()).Build()
	b2x := new(block.Builder).ParentID(b1.ID()).Timestamp(23132).Build()
	b3x := new(block.Builder).ParentID(b2x.ID()).Build()

	c.AddBlock(b1, true)
	c.AddBlock(b2, true)
	c.AddBlock(b3, true)
	c.AddBlock(b2x, false)
	c.AddBlock(b3x, false)

	hg := chain.NewBlockIDGetter(c, b3x.ID())
	assert.Equal(t, hg.GetID(4), thor.Hash{})
	assert.Equal(t, hg.GetID(3), b3x.ID())
	assert.Equal(t, hg.GetID(2), b2x.ID())
	assert.Equal(t, hg.GetID(1), b1.ID())
	assert.Equal(t, hg.GetID(0), b0.ID())
}
