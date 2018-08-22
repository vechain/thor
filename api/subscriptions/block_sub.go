package subscriptions

import (
	"context"
	"errors"

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

func (bs *BlockSub) Ch() chan struct{} { return bs.ch }

func (bs *BlockSub) Chain() *chain.Chain { return bs.chain }

func (bs *BlockSub) FromBlock() thor.Bytes32 { return bs.fromBlock }

func (bs *BlockSub) Read(ctx context.Context) (BlockIter, BlockIter, error) {
	changes, removes, err := Read(ctx, bs)
	if err != nil {
		return nil, nil, err
	}

	return BlockIter(changes), BlockIter(removes), err
}

func (bs *BlockSub) UpdateFilter(bestID thor.Bytes32) {
	bs.fromBlock = bestID
}

// from open, to closed
func (bs *BlockSub) SliceChain(from thor.Bytes32, to thor.Bytes32) ([]interface{}, error) {
	if block.Number(to) <= block.Number(from) {
		return nil, errors.New("to must be greater than from")
	}

	length := int64(block.Number(to) - block.Number(from))
	blks := make([]interface{}, length)

	for i := length - 1; i >= 0; i-- {
		blk, err := bs.chain.GetBlock(to)
		if err != nil {
			return nil, err
		}
		blks[i] = blk
		to = blk.Header().ParentID()
	}

	return blks, nil
}

type BlockIter []interface{}

func (bi BlockIter) Iterator() <-chan *block.Block {
	c := make(chan *block.Block)
	go func() {
		for _, v := range bi {
			if blk, ok := v.(*block.Block); ok {
				c <- blk
			}
		}
		close(c)
	}()
	return c
}
