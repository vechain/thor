package subscriptions

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type Sub interface {
	Ch() chan struct{} // When chain changed, this channel will be readable
	Chain() *chain.Chain
	FromBlock() thor.Bytes32
	SliceChain(thor.Bytes32, thor.Bytes32) ([]interface{}, error)
	UpdateFilter(thor.Bytes32)
}

func Read(ctx context.Context, sub Sub) ([]interface{}, []interface{}, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-sub.Ch():
			best := sub.Chain().BestBlock()
			bestID := best.Header().ID()

			if bestID == sub.FromBlock() {
				continue
			}

			ancestor, err := sub.Chain().GetAncestorBlockID(bestID, block.Number(sub.FromBlock()))
			if err != nil {
				return nil, nil, err
			}

			if ancestor == sub.FromBlock() {
				changes, err := sub.SliceChain(ancestor, bestID)
				if err != nil {
					return nil, nil, err
				}

				sub.UpdateFilter(bestID)
				return changes, nil, nil
			}

			sa, err := lookForSameAncestor(sub.FromBlock(), ancestor, sub.Chain())
			if err != nil {
				return nil, nil, err
			}

			removes, err := sub.SliceChain(sa, sub.FromBlock())
			if err != nil {
				return nil, nil, err
			}

			changes, err := sub.SliceChain(sa, bestID)
			if err != nil {
				return nil, nil, err
			}

			sub.UpdateFilter(bestID)
			return changes, removes, nil
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
