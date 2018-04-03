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
	b1 := new(block.Builder).Build()
	c, err := chain.New(s, b1)
	assert.Nil(t, err)

	b2 := new(block.Builder).ParentID(b1.Header().ID()).Build()
	b3 := new(block.Builder).ParentID(b2.Header().ID()).Build()
	b2x := new(block.Builder).ParentID(b1.Header().ID()).Build()
	b3x := new(block.Builder).ParentID(b2x.Header().ID()).Build()

	c.AddBlock(b2, nil, true)
	c.AddBlock(b3, nil, true)
	c.AddBlock(b2x, nil, false)
	c.AddBlock(b3x, nil, false)

	tr := c.NewTraverser(b3x.Header().ID())
	assert.Equal(t, tr.Get(3).ID(), b3x.Header().ID())
	assert.Equal(t, tr.Get(2).ID(), b2x.Header().ID())
	assert.Equal(t, tr.Get(1).ID(), b1.Header().ID())

	assert.Nil(t, nil, tr.Error())
}
