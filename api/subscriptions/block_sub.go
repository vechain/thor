package subscriptions

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type BlockSub struct {
	chain     *chain.Chain
	fromBlock thor.Bytes32
}

func NewBlockSub(chain *chain.Chain, fromBlock thor.Bytes32) *BlockSub {
	return &BlockSub{
		chain:     chain,
		fromBlock: fromBlock,
	}
}

func (bs *BlockSub) Read() ([]*block.Block, []*block.Block, error) {
	best := bs.chain.BestBlock()
	bestID := best.Header().ID()

	if bestID == bs.fromBlock {
		return nil, nil, nil
	}

	ancestor, err := bs.chain.GetAncestorBlockID(bestID, block.Number(bs.fromBlock))
	if err != nil {
		return nil, nil, err
	}

	if ancestor == bs.fromBlock {
		next, err := bs.nextBlock(ancestor)
		if err != nil {
			return nil, nil, err
		}
		return []*block.Block{next}, nil, nil
	}

	sa, err := bs.lookForSameAncestor(bs.fromBlock, ancestor)
	if err != nil {
		return nil, nil, err
	}

	removes, err := bs.sliceChain(sa, bs.fromBlock)
	if err != nil {
		return nil, nil, err
	}

	next, err := bs.nextBlock(sa)
	if err != nil {
		return nil, nil, err
	}

	return []*block.Block{next}, removes, nil
}

// from open, to closed
func (bs *BlockSub) sliceChain(from, to thor.Bytes32) ([]*block.Block, error) {
	if block.Number(to) <= block.Number(from) {
		return nil, errors.New("to must be greater than from")
	}

	length := int64(block.Number(to) - block.Number(from))
	blks := make([]*block.Block, length)

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

// src and tar must have the same num
func (bs *BlockSub) lookForSameAncestor(src, tar thor.Bytes32) (thor.Bytes32, error) {
	for {
		if src == tar {
			return src, nil
		}

		srcHeader, err := bs.chain.GetBlockHeader(src)
		if err != nil {
			return thor.Bytes32{}, err
		}
		src = srcHeader.ParentID()

		tarHeader, err := bs.chain.GetBlockHeader(tar)
		if err != nil {
			return thor.Bytes32{}, err
		}
		tar = tarHeader.ParentID()
	}
}

func (bs *BlockSub) nextBlock(from thor.Bytes32) (*block.Block, error) {
	fromBlk, err := bs.chain.GetBlock(from)
	if err != nil {
		return nil, err
	}

	nextBlk, err := bs.chain.GetTrunkBlock(fromBlk.Header().Number() + 1)
	if err != nil {
		return nil, err
	}

	bs.fromBlock = nextBlk.Header().ID()
	return nextBlk, nil
}
