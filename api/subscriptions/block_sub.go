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

func (bs *BlockSub) Read(ctx context.Context) ([]*block.Block, []*block.Block, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-bs.ch:
			best := bs.chain.BestBlock()
			bestID := best.Header().ID()

			if bestID == bs.fromBlock {
				continue
			}

			ancestor, err := bs.chain.GetAncestorBlockID(bestID, block.Number(bs.fromBlock))
			if err != nil {
				return nil, nil, err
			}

			if ancestor == bs.fromBlock {
				blks, err := sliceChain(ancestor, bestID, bs.chain)
				if err != nil {
					return nil, nil, err
				}

				bs.fromBlock = bestID
				return blks, nil, nil
			}

			sa, err := lookForSameAncestor(bs.fromBlock, ancestor, bs.chain)
			if err != nil {
				return nil, nil, err
			}

			removes, err := sliceChain(sa, bs.fromBlock, bs.chain)
			if err != nil {
				return nil, nil, err
			}

			blks, err := sliceChain(sa, bestID, bs.chain)
			if err != nil {
				return nil, nil, err
			}

			bs.fromBlock = bestID
			return blks, removes, nil
		}
	}
}

// src and tar must have the same num
func lookForSameAncestor(src, tar thor.Bytes32, chain *chain.Chain) (thor.Bytes32, error) {
	for {
		if src == tar {
			return src, nil
		}

		srcHeader, err := chain.GetBlockHeader(src)
		if err != nil {
			return thor.Bytes32{}, err
		}
		src = srcHeader.ParentID()

		tarHeader, err := chain.GetBlockHeader(tar)
		if err != nil {
			return thor.Bytes32{}, err
		}
		tar = tarHeader.ParentID()
	}
}

// from open, to closed
func sliceChain(from thor.Bytes32, to thor.Bytes32, chain *chain.Chain) ([]*block.Block, error) {
	if block.Number(to) <= block.Number(from) {
		return nil, errors.New("to must be greater than from")
	}

	length := block.Number(to) - block.Number(from)
	blks := make([]*block.Block, length)

	for i := length - 1; i >= 0; i-- {
		blk, err := chain.GetBlock(to)
		if err != nil {
			return nil, err
		}
		blks[i] = blk
		to = blk.Header().ParentID()
	}

	return blks, nil
}
