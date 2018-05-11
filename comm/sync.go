package comm

import (
	"context"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
)

func (c *Communicator) choosePeerToSync(bestBlock *block.Block) *Peer {
	betters := c.peerSet.Slice().Filter(func(peer *Peer) bool {
		_, totalScore := peer.Head()
		return totalScore >= bestBlock.Header().TotalScore()
	})

	if len(betters) > 0 {
		return betters[0]
	}
	return nil
}

func (c *Communicator) sync(handler HandleBlockStream) error {
	localBest := c.chain.BestBlock()
	peer := c.choosePeerToSync(localBest)
	if peer == nil {
		if c.peerSet.Len() >= 3 {
			return nil
		}
		return errors.New("no suitable peer")
	}

	ancestor, err := c.findCommonAncestor(peer, localBest.Header().Number())
	if err != nil {
		return err
	}

	return c.download(peer, ancestor+1, handler)
}

func (c *Communicator) download(peer *Peer, fromNum uint32, handler HandleBlockStream) error {
	ctx, cancel := context.WithCancel(c.ctx)
	var errValue atomic.Value
	blockCh := make(chan *block.Block, 32)

	var goes co.Goes
	goes.Go(func() {
		defer cancel()
		if err := handler(ctx, blockCh); err != nil {
			errValue.Store(err)
		}
	})
	goes.Go(func() {
		defer close(blockCh)
		for {
			result, err := proto.GetBlocksFromNumber{Num: fromNum}.Call(ctx, peer)
			if err != nil {
				errValue.Store(err)
				return
			}
			if len(result) == 0 {
				return
			}

			for _, raw := range result {
				var blk block.Block
				if err := rlp.DecodeBytes(raw, &blk); err != nil {
					errValue.Store(errors.Wrap(err, "invalid block"))
					return
				}
				if _, err := blk.Header().Signer(); err != nil {
					errValue.Store(errors.Wrap(err, "invalid block"))
					return
				}
				if blk.Header().Number() != fromNum {
					errValue.Store(errors.New("broken sequence"))
					return
				}
				peer.MarkBlock(blk.Header().ID())
				fromNum++

				select {
				case <-ctx.Done():
					return
				case blockCh <- &blk:
				}
			}
		}
	})
	goes.Wait()
	if err := errValue.Load(); err != nil {
		return err.(error)
	}
	return nil
}

func (c *Communicator) findCommonAncestor(peer *Peer, headNum uint32) (uint32, error) {
	if headNum == 0 {
		return headNum, nil
	}

	isOverlapped := func(num uint32) (bool, error) {
		result, err := proto.GetBlockIDByNumber{Num: num}.Call(c.ctx, peer)
		if err != nil {
			return false, err
		}
		id, err := c.chain.GetBlockIDByNumber(num)
		if err != nil {
			return false, err
		}
		return id == result.ID, nil
	}

	var find func(start uint32, end uint32, ancestor uint32) (uint32, error)
	find = func(start uint32, end uint32, ancestor uint32) (uint32, error) {
		if start == end {
			overlapped, err := isOverlapped(start)
			if err != nil {
				return 0, err
			}
			if overlapped {
				return start, nil
			}
		} else {
			mid := (start + end) / 2
			overlapped, err := isOverlapped(mid)
			if err != nil {
				return 0, err
			}
			if overlapped {
				return find(mid+1, end, mid)
			}

			if mid > start {
				return find(start, mid-1, ancestor)
			}
		}
		return ancestor, nil
	}

	fastSeek := func() (uint32, error) {
		i := uint32(0)
		for {
			backward := uint32(4) << i
			i++
			if backward >= headNum {
				return 0, nil
			}

			overlapped, err := isOverlapped(headNum - backward)
			if err != nil {
				return 0, err
			}
			if overlapped {
				return headNum - backward, nil
			}
		}
	}

	seekedNum, err := fastSeek()
	if err != nil {
		return 0, err
	}

	return find(seekedNum, headNum, 0)
}
