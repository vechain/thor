// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/comm/proto"
)

type rawBlockBatch struct {
	rawBlocks []rlp.RawValue
	startNum  uint32
}

func download(_ctx context.Context, repo *chain.Repository, peer *Peer, headNum uint32, handler HandleBlockStream) error {
	ancestor, err := findCommonAncestor(_ctx, repo, peer, headNum)
	if err != nil {
		return errors.WithMessage(err, "find common ancestor")
	}

	var (
		ctx, cancel = context.WithCancel(_ctx)
		rawBatches  = make(chan rawBlockBatch, 10)
		warmedUp    = make(chan *block.Block, 2048)
	)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(rawBatches)
		return fetchRawBlockBatches(ctx, peer, ancestor+1, rawBatches)
	})

	g.Go(func() error {
		defer close(warmedUp)
		return decodeAndWarmupBatches(ctx, rawBatches, warmedUp)
	})

	g.Go(func() error {
		return handler(ctx, warmedUp)
	})

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func fetchRawBlockBatches(ctx context.Context, peer *Peer, fromBlockNum uint32, rawBatches chan<- rawBlockBatch) error {
	for {
		result, err := proto.GetBlocksFromNumber(ctx, peer, fromBlockNum)
		if err != nil {
			return err
		}
		if len(result) == 0 {
			return nil
		}

		batch := rawBlockBatch{
			rawBlocks: result,
			startNum:  fromBlockNum,
		}

		select {
		case rawBatches <- batch:
		case <-ctx.Done():
			return ctx.Err()
		}

		fromBlockNum += uint32(len(result))
	}
}

func decodeAndWarmupBatches(ctx context.Context, rawBatches <-chan rawBlockBatch, warmedUp chan<- *block.Block) error {
	g, ctx := errgroup.WithContext(ctx)

	for batch := range rawBatches {
		g.Go(func() error {
			return decodeAndWarmupBatch(ctx, batch, warmedUp)
		})
	}

	return g.Wait()
}

func decodeAndWarmupBatch(ctx context.Context, batch rawBlockBatch, warmedUp chan<- *block.Block) error {
	blocks := make([]*block.Block, 0, len(batch.rawBlocks))

	for i, raw := range batch.rawBlocks {
		var blk block.Block
		if err := rlp.DecodeBytes(raw, &blk); err != nil {
			return errors.Wrap(err, "invalid block")
		}

		expectedNum := batch.startNum + uint32(i)
		if blk.Header().Number() != expectedNum {
			return errors.New("broken sequence")
		}

		blocks = append(blocks, &blk)
	}

	for _, blk := range blocks {
		blk.Header().ID()
		blk.Header().Beta()

		for _, tx := range blk.Transactions() {
			// pre warming functions with cache
			tx.ID()
			tx.UnprovedWork()
			_, _ = tx.IntrinsicGas()
			_, _ = tx.Delegator()
		}

		select {
		case <-ctx.Done():
			return nil
		case warmedUp <- blk:
		}
	}

	return nil
}

func findCommonAncestor(ctx context.Context, repo *chain.Repository, peer *Peer, headNum uint32) (uint32, error) {
	if headNum == 0 {
		return headNum, nil
	}

	isOverlapped := func(num uint32) (bool, error) {
		result, err := proto.GetBlockIDByNumber(ctx, peer, num)
		if err != nil {
			return false, err
		}
		id, err := repo.NewBestChain().GetBlockID(num)
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
		peer.logger.Trace(fmt.Sprintf("sync txs loop %v", i))
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
