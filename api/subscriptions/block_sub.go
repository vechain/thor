package subscriptions

import (
	"context"
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type BlockSub struct {
	chain        *chain.Chain
	fromBlock    thor.Bytes32
	headerWaiter func() <-chan bool
}

func NewBlockSub(chain *chain.Chain, fromBlock thor.Bytes32) *BlockSub {
	return &BlockSub{
		chain:        chain,
		fromBlock:    fromBlock,
		headerWaiter: chain.HeadWaiter(),
	}
}

func (bs *BlockSub) Read(ctx context.Context) ([]*block.Block, []*block.Block, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-bs.headerWaiter():
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
				changes, err := bs.sliceChain(ancestor, bestID)
				if err != nil {
					return nil, nil, err
				}

				bs.fromBlock = bestID
				return changes, nil, nil
			}

			sa, err := bs.lookForSameAncestor(bs.fromBlock, ancestor)
			if err != nil {
				return nil, nil, err
			}

			removes, err := bs.sliceChain(sa, bs.fromBlock)
			if err != nil {
				return nil, nil, err
			}

			changes, err := bs.sliceChain(sa, bestID)
			if err != nil {
				return nil, nil, err
			}

			bs.fromBlock = bestID
			return changes, removes, nil
		}
	}
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
