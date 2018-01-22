package chain

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// BlockIDGetter helper class to support vm.Context.GetHash.
type BlockIDGetter struct {
	chain *Chain
	ref   thor.Hash
	err   error
}

// NewBlockIDGetter create a new BlockIDGetter object.
func NewBlockIDGetter(chain *Chain, ref thor.Hash) *BlockIDGetter {
	return &BlockIDGetter{
		chain,
		ref,
		nil,
	}
}

// Error returns error occurred during GetHash calls.
func (g *BlockIDGetter) Error() error {
	return g.err
}

func (g *BlockIDGetter) setError(err error) {
	if g.err == nil {
		g.err = err
	}
}

// GetID is compliant with vm.Context.GetHash.
func (g *BlockIDGetter) GetID(num uint32) thor.Hash {

	ref := g.ref

	for {
		refNum := block.Number(ref)
		if num > refNum {
			return thor.Hash{}
		}
		if num == refNum {
			return ref
		}
		header, err := g.chain.GetBlockHeader(ref)
		if err != nil {
			if !g.chain.IsNotFound(err) {
				g.setError(err)
			}
			return thor.Hash{}
		}
		ref = header.ParentID()
	}
}
