package comm

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/comm/session"
	"github.com/vechain/thor/p2psrv"
)

func (c *Communicator) chooseSessionToSync(bestBlock *block.Block) (*session.Session, int) {
	slice := c.sessionSet.Slice()
	betters := slice.Filter(func(s *session.Session) bool {
		_, totalScore := s.TrunkHead()
		return totalScore >= bestBlock.Header().TotalScore()
	})

	if len(betters) > 0 {
		return betters[0], len(slice)
	}
	return nil, len(slice)
}

func (c *Communicator) sync(handler HandleBlockChunk) error {
	best := c.chain.BestBlock()
	s, nSessions := c.chooseSessionToSync(best)
	if s == nil {
		if nSessions >= 3 {
			return nil
		}
		return errors.New("no suitable session")
	}

	ancestor, err := c.findCommonAncestor(s.Peer(), best.Header().Number())
	if err != nil {
		return err
	}

	return c.download(s, ancestor+1, handler)
}

func (c *Communicator) download(session *session.Session, fromNum uint32, handler HandleBlockChunk) error {

	type blockWithSize struct {
		*block.Block
		size int
	}

	var goes co.Goes
	defer goes.Wait()

	errCh := make(chan error)
	// buffer between downloading and processing
	blockCh := make(chan *blockWithSize, 512)
	defer close(blockCh)

	goes.Go(func() {
		const maxChunkBlocks = 2048
		const maxChunkSize = 1024 * 1024
		var chunk []*block.Block
		var chunkSize int
		for {
			select {
			case <-c.ctx.Done():
				return
			case bs := <-blockCh:
				if bs == nil {
					if len(chunk) > 0 {
						if err := handler(c.ctx, chunk); err != nil {
							errCh <- err
							return
						}
					}
					return
				}

				chunkSize += bs.size
				chunk = append(chunk, bs.Block)

				if len(chunk) >= maxChunkBlocks || chunkSize >= maxChunkSize {
					if err := handler(c.ctx, chunk); err != nil {
						errCh <- err
						return
					}
					chunk = nil
					chunkSize = 0
				}
			}
		}
	})
	for {
		peer := session.Peer()
		req := proto.ReqGetBlocksFromNumber{Num: fromNum}
		resp, err := req.Do(c.ctx, peer)
		if err != nil {
			return err
		}
		if len(resp) == 0 {
			return nil
		}

		for _, raw := range resp {
			var blk block.Block
			if err := rlp.DecodeBytes(raw, &blk); err != nil {
				return errors.Wrap(err, "invalid block")
			}

			session.MarkBlock(blk.Header().ID())
			fromNum++

			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case blockCh <- &blockWithSize{&blk, len(raw)}:
			case err := <-errCh:
				return err
			}
		}
	}
}

func (c *Communicator) findCommonAncestor(peer *p2psrv.Peer, headNum uint32) (uint32, error) {
	if headNum == 0 {
		return headNum, nil
	}

	isOverlapped := func(num uint32) (bool, error) {
		req := proto.ReqGetBlockIDByNumber{Num: num}
		resp, err := req.Do(c.ctx, peer)
		if err != nil {
			return false, err
		}
		id, err := c.chain.GetBlockIDByNumber(num)
		if err != nil {
			return false, err
		}
		return id == resp.ID, nil
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
