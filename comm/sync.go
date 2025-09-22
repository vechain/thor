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
	"github.com/vechain/thor/v2/co"
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

	// Three-stage pipeline for block synchronization:
	//
	// Stage 1: Raw Data Fetcher (Worker 1)
	//   - Fetches raw block data from peer in batches
	//   - Sends raw data to rawBatches channel
	//   - Closes rawBatches when done
	//
	// Stage 2: Decoder & Warm-up (Worker 2)
	//   - Receives raw data from rawBatches channel
	//   - Decodes raw data into block objects
	//   - Pre-warms block/transaction caches (ID, Beta, IntrinsicGas, etc.)
	//   - Sends decoded blocks to warmedUp channel
	//   - Closes warmedUp when done
	//
	// Stage 3: Block Handler (Worker 3)
	//   - Receives pre-warmed blocks from warmedUp channel
	//   - Processes blocks (validation, storage, etc.)
	//   - Runs until warmedUp channel is closed
	//
	// Channel Flow:
	//   rawBatches (chan []byte) -> warmedUp (chan *block.Block)
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
	var err error
	<-co.Parallel(func(queue chan<- func()) {
		for batch := range rawBatches {
			for i, raw := range batch.rawBlocks {
				var blk block.Block
				if err := rlp.DecodeBytes(raw, &blk); err != nil {
					err = errors.Wrap(err, "invalid block")
					return
				}

				expectedNum := batch.startNum + uint32(i)
				if blk.Header().Number() != expectedNum {
					err = errors.New("broken sequence")
					return
				}

				// warm up functions with cache, ignore error here
				_ = blk.Header().ID()
				_, _ = blk.Header().Beta()
				for _, tx := range blk.Transactions() {
					// pre warming functions with cache
					_ = tx.ID()
					_ = tx.UnprovedWork()
					_, _ = tx.IntrinsicGas()
					_, _ = tx.Delegator()
				}
				select {
				case <-ctx.Done():
					err = ctx.Err()
					return
				case warmedUp <- &blk:
					// when queued blocks count > 10% warmed up channel cap,
					// send nil block to throttle to reduce mem pressure.
					if len(warmedUp)*10 > cap(warmedUp) {
						const targetSize = 2048
						for range int(blk.Size())/targetSize - 1 {
							select {
							case warmedUp <- nil:
							default:
							}
						}
					}
				}
			}
		}
	})
	return err
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
