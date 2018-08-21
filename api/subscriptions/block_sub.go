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
		ch:        make(chan struct{}, 2),
		chain:     chain,
		fromBlock: fromBlock,
	}
}

func (bs *BlockSub) Read(ctx context.Context) ([]*block.Header, []*block.Header, error) {
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
				blks, err := blockHeaders(ancestor, bestID, bs.chain)
				if err != nil {
					return nil, nil, err
				}
				bs.fromBlock = bestID
				return blks, nil, nil
			}

			sa, err := lookingForSameAncestor(bs.fromBlock, ancestor, bs.chain)
			if err != nil {
				return nil, nil, err
			}

			removes, err := blockHeaders(sa, bs.fromBlock, bs.chain)
			if err != nil {
				return nil, nil, err
			}

			blks, err := blockHeaders(sa, bestID, bs.chain)
			if err != nil {
				return nil, nil, err
			}

			bs.fromBlock = bestID
			return blks, removes, nil
		}
	}
}

func lookingForSameAncestor(src, tar thor.Bytes32, chain *chain.Chain) (thor.Bytes32, error) {
	if src == tar {
		return src, nil
	}

	srcHeader, err := chain.GetBlockHeader(src)
	if err != nil {
		return thor.Bytes32{}, err
	}

	tarHeader, err := chain.GetBlockHeader(tar)
	if err != nil {
		return thor.Bytes32{}, err
	}

	return lookingForSameAncestor(srcHeader.ParentID(), tarHeader.ParentID(), chain)
}

// 左开右闭
func blockHeaders(from thor.Bytes32, to thor.Bytes32, chain *chain.Chain) ([]*block.Header, error) {
	length := block.Number(to) - block.Number(from)
	blockHeaders := make([]*block.Header, length)

	for i := length - 1; i >= 0; i-- {
		blkHeader, err := chain.GetBlockHeader(to)
		if err != nil {
			return nil, err
		}
		blockHeaders[i] = blkHeader
		to = blkHeader.ParentID()
	}

	return blockHeaders, nil
}
