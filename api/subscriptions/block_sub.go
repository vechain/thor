package subscriptions

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type BlockSub struct {
	ch        chan struct{} // When chain changed, this channel will be readable
	chain     *chain.Chain
	fromBlock thor.Bytes32
}

func NewBlockSub(ch chan struct{}, chain *chain.Chain, fromBlock thor.Bytes32) *BlockSub {
	return &BlockSub{
		ch:        ch,
		chain:     chain,
		fromBlock: fromBlock,
	}
}

func (bs *BlockSub) FromBlock() thor.Bytes32 { return bs.fromBlock }

func (bs *BlockSub) UpdateFilter(bestID thor.Bytes32) {
	bs.fromBlock = bestID
}

// from open, to closed
func (bs *BlockSub) SliceChain(from, to thor.Bytes32) ([]interface{}, error) {
	analyseF := func(chain *chain.Chain, blk *block.Block) (interface{}, error) {
		return blk, nil
	}
	return sliceChain(from, to, bs.chain, analyseF)
}

func (bs *BlockSub) Read(ctx context.Context) ([]*block.Block, []*block.Block, error) {
	changes, removes, err := read(ctx, bs.ch, bs.chain, bs)
	if err != nil {
		return nil, nil, err
	}

	convertBlock := func(slice []interface{}) []*block.Block {
		result := make([]*block.Block, len(slice))
		for i, v := range slice {
			if blk, ok := v.(*block.Block); ok {
				result[i] = blk
			}
		}
		return result
	}

	return convertBlock(changes), convertBlock(removes), err
}
