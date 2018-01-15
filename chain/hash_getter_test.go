package chain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestHashGetter(t *testing.T) {
	s, _ := lvldb.NewMem()
	c := chain.New(s)
	b0 := new(block.Builder).Build()
	c.WriteGenesis(b0)

	b1 := new(block.Builder).ParentHash(b0.Hash()).Build()
	b2 := new(block.Builder).ParentHash(b1.Hash()).Build()
	b3 := new(block.Builder).ParentHash(b2.Hash()).Build()
	b2x := new(block.Builder).ParentHash(b1.Hash()).Timestamp(23132).Build()
	b3x := new(block.Builder).ParentHash(b2x.Hash()).Build()

	c.AddBlock(b1, true)
	c.AddBlock(b2, true)
	c.AddBlock(b3, true)
	c.AddBlock(b2x, false)
	c.AddBlock(b3x, false)

	hg := chain.NewHashGetter(c, b3x.Hash())
	assert.Equal(t, hg.GetHash(4), thor.Hash{})
	assert.Equal(t, hg.GetHash(3), b3x.Hash())
	assert.Equal(t, hg.GetHash(2), b2x.Hash())
	assert.Equal(t, hg.GetHash(1), b1.Hash())
	assert.Equal(t, hg.GetHash(0), b0.Hash())
}
