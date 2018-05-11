package comm

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
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

func (c *Communicator) download(peer *Peer, fromNum uint32, handler HandleBlockStream) (err error) {

	errCh := make(chan error, 1)
	defer func() {
		if err != nil {
			<-errCh
		} else {
			err = <-errCh
		}
	}()

	blockCh := make(chan *block.Block, 32)
	defer close(blockCh)

	go func() {
		errCh <- handler(c.ctx, blockCh)
	}()

	for {
		result, err := proto.GetBlocksFromNumber{Num: fromNum}.Call(c.ctx, peer)
		if err != nil {
			return err
		}
		if len(result) == 0 {
			return nil
		}

		for _, raw := range result {
			var blk block.Block
			if err := rlp.DecodeBytes(raw, &blk); err != nil {
				return errors.Wrap(err, "invalid block")
			}
			if _, err := blk.Header().Signer(); err != nil {
				return errors.Wrap(err, "invalid block")
			}
			if blk.Header().Number() != fromNum {
				return errors.New("broken sequence")
			}
			peer.MarkBlock(blk.Header().ID())
			fromNum++

			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case blockCh <- &blk:
			case err := <-errCh:
				errCh <- err
				return err
			}
		}
	}
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
