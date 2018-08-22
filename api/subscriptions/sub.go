package subscriptions

import (
	"context"
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type Sub interface {
	FromBlock() thor.Bytes32
	SliceChain(thor.Bytes32, thor.Bytes32) ([]interface{}, error)
	UpdateFilter(thor.Bytes32)
}

func read(ctx context.Context, ch chan struct{}, chain *chain.Chain, sub Sub) ([]interface{}, []interface{}, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-ch:
			best := chain.BestBlock()
			bestID := best.Header().ID()

			if bestID == sub.FromBlock() {
				continue
			}

			ancestor, err := chain.GetAncestorBlockID(bestID, block.Number(sub.FromBlock()))
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

			sa, err := lookForSameAncestor(sub.FromBlock(), ancestor, chain)
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

// from open, to closed
func sliceChain(from, to thor.Bytes32, chain *chain.Chain, f func(*block.Block) (interface{}, error)) ([]interface{}, error) {
	if block.Number(to) <= block.Number(from) {
		return nil, errors.New("to must be greater than from")
	}

	length := int64(block.Number(to) - block.Number(from))
	slice := make([]interface{}, length)

	for i := length - 1; i >= 0; i-- {
		blk, err := chain.GetBlock(to)
		if err != nil {
			return nil, err
		}

		v, err := f(blk)
		if err != nil {
			return nil, err
		}

		slice[i] = v
		to = blk.Header().ParentID()
	}

	return slice, nil
}
