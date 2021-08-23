// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"fmt"

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
	var (
		ctx, cancel = context.WithCancel(c.ctx)
		blockCh     = make(chan *block.Block, 2048)
		goes        co.Goes
		handlerErr  error
	)
	defer goes.Wait()
	defer close(blockCh)
	defer cancel()

	goes.Go(func() {
		defer cancel()
		handlerErr = handler(ctx, blockCh)
	})

	var blocks []*block.Block
	for {
		result, err := proto.GetBlocksFromNumber(ctx, peer, fromNum)
		if err != nil {
			if handlerErr != nil {
				return handlerErr
			}
			return err
		}
		if len(result) == 0 {
			return handlerErr
		}

		blocks = blocks[:0]
		for _, raw := range result {
			var blk block.Block
			if err := rlp.DecodeBytes(raw, &blk); err != nil {
				return errors.Wrap(err, "invalid block")
			}
			if blk.Header().Number() != fromNum {
				return errors.New("broken sequence")
			}
			fromNum++
			blocks = append(blocks, &blk)
		}

		select {
		case <-co.Parallel(func(queue chan<- func()) {
			for _, blk := range blocks {
				h := blk.Header()
				queue <- func() { h.ID() }
				for _, tx := range blk.Transactions() {
					tx := tx
					queue <- func() {
						tx.ID()
						tx.UnprovedWork()
						_, _ = tx.IntrinsicGas()
						_, _ = tx.Delegator()
					}
				}
			}
		}):
		case <-ctx.Done():
			if handlerErr != nil {
				return handlerErr
			}
			return ctx.Err()
		}

		for _, blk := range blocks {
			// when queued blocks count > 10% channel cap,
			// send nil block to throttle to reduce mem pressure.
			if len(blockCh)*10 > cap(blockCh) {
				const targetSize = 2048
				for i := 0; i < int(blk.Size())/targetSize-1; i++ {
					select {
					case blockCh <- nil:
					default:
					}
				}
			}
			select {
			case <-ctx.Done():
				if handlerErr != nil {
					return handlerErr
				}
				return ctx.Err()
			case blockCh <- blk:
			}
		}
	}
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
		id, err := c.repo.NewBestChain().GetBlockID(num)
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
	for i := 0; ; i++ {
		peer.logger.Debug(fmt.Sprintf("sync txs loop %v", i))
		result, err := proto.GetTxs(c.ctx, peer)
		if err != nil {
			peer.logger.Debug("failed to request txs", "err", err)
			return
		}

		// no more txs
		if len(result) == 0 {
			break
		}

		for _, tx := range result {
			peer.MarkTransaction(tx.Hash())
			_ = c.txPool.StrictlyAdd(tx)
			select {
			case <-c.ctx.Done():
				return
			default:
			}
		}

		if i >= 100 {
			peer.logger.Debug("too many loops to sync txs, break")
			return
		}
	}
	peer.logger.Debug("sync txs done")
}
