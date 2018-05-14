package comm

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
)

func (c *Communicator) sync(peer *Peer, headNum uint32, handler HandleBlockStream) error {
	ancestor, err := c.findCommonAncestor(peer, headNum)
	if err != nil {
		return errors.WithMessage(err, "find common ancestor")
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
			result, err := proto.GetBlocksFromNumber(ctx, peer, fromNum)
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
		result, err := proto.GetBlockIDByNumber(c.ctx, peer, num)
		if err != nil {
			return false, err
		}
		id, err := c.chain.GetBlockIDByNumber(num)
		if err != nil {
			return false, err
		}
		return id == result, nil
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
		var backward uint32
		for {
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
			if backward == 0 {
				backward = 1
			} else {
				backward <<= 1
			}
		}
	}

	seekNum, err := fastSeek()
	if err != nil {
		return 0, err
	}
	if seekNum == headNum {
		return headNum, nil
	}
	return find(seekNum, headNum, 0)
}

func (c *Communicator) syncTxs(peer *Peer) {
	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()
	result, err := proto.GetTxs(ctx, peer)
	if err != nil {
		peer.logger.Debug("failed to request txs", "err", err)
		return
	}
	for _, tx := range result {
		c.txPool.Add(tx)

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	peer.logger.Debug("tx synced")
}
