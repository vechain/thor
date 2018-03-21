package comm

import (
	"github.com/vechain/thor/comm/session"

	"github.com/vechain/thor/block"
)

func (c *Communicator) chooseSessionToSync(bestBlock *block.Block) *session.Session {
	betters := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
		_, totalScore := s.TrunkHead()
		return totalScore > bestBlock.Header().TotalScore()
	})

	if len(betters) > 0 {
		return betters[0]
	}
	return nil
}

func (c *Communicator) sync() error {

	// best, err := c.chain.GetBestBlock()
	// if err != nil {
	// 	return err
	// }

	// s := c.chooseSessionToSync(best)
	// if s == nil {
	// 	return nil
	// }

	// ancestor, err := c.findCommonAncestor(s.Peer(), best.Header().Number())
	// if err != nil {
	// 	return err
	// }

	// return c.download(st.peer, ancestor, block.Number(st.st.BestBlockID)-ancestor)
	return nil
}

// func (c *Communicator) download(peer *p2psrv.Peer, ancestor uint32, target uint32) error {
// 	for syned := 0; uint32(syned) < target; {
// 		blks, err := proto.ReqGetBlocksByNumber{Num: ancestor}.Do(c.ctx, peer)
// 		if err != nil {
// 			return err
// 		}
// 		syned += len(blks)
// 		ancestor += uint32(syned)
// 		for _, blk := range blks {
// 			go func(blk *block.Block) {
// 				select {
// 				case c.blockCh <- blk:
// 				case <-c.ctx.Done():
// 				}
// 			}(blk)
// 		}
// 	}
// 	return nil
// }

// func (c *Communicator) findCommonAncestor(peer *p2psrv.Peer, fromNum uint32) (uint32, error) {
// 	isOverlap := func(num uint32) (bool, error) {
// 		req := proto.ReqGetBlockIDByNumber{Num: fromNum}
// 		resp, err := req.Do(c.ctx, peer)
// 		if err != nil {
// 			return false, err
// 		}
// 		id, err := c.chain.GetBlockIDByNumber(fromNum)
// 		if err != nil {
// 			return false, err
// 		}
// 		return id == resp.ID, nil
// 	}

// 	for {
// 		for i := uint32(0); i < 10; i++ {
// 			overlapped, err := isOverlap(fromNum)
// 			if err != nil {
// 				return 0, err
// 			}
// 			if overlapped {
// 				if i == 0 {
// 					return fromNum, nil
// 				}
// 				break
// 			}

// 			step := uint32(1) << i
// 			if step < fromNum {
// 				fromNum -= step
// 			} else {
// 				fromNum = 0
// 				break
// 			}
// 		}

// 		for {

// 		}
// 	}
// }

// func (c *Communicator) findAncestor(peer *p2psrv.Peer, start uint32, end uint32, ancestor uint32) (uint32, error) {
// 	if start == end {
// 		localID, remoteID, err := c.getLocalAndRemoteIDByNumber(peer, start)
// 		if err != nil {
// 			return 0, err
// 		}

// 		if bytes.Compare(localID[:], remoteID[:]) == 0 {
// 			return start, nil
// 		}
// 	} else {
// 		mid := (start + end) / 2
// 		midID, remoteID, err := c.getLocalAndRemoteIDByNumber(peer, mid)
// 		if err != nil {
// 			return 0, err
// 		}

// 		if bytes.Compare(midID[:], remoteID[:]) == 0 {
// 			return c.findAncestor(peer, mid+1, end, mid)
// 		}

// 		if bytes.Compare(midID[:], remoteID[:]) != 0 {
// 			if mid > start {
// 				return c.findAncestor(peer, start, mid-1, ancestor)
// 			}
// 		}
// 	}

// 	return ancestor, nil
// }

// func (c *Communicator) getLocalAndRemoteIDByNumber(peer *p2psrv.Peer, num uint32) (thor.Hash, thor.Hash, error) {
// 	blk, err := c.ch.GetBlockByNumber(num)
// 	if err != nil {
// 		return thor.Hash{}, thor.Hash{}, fmt.Errorf("[findAncestor]: %v", err)
// 	}

// 	respID, err := proto.ReqGetBlockIDByNumber{Num: num}.Do(c.ctx, peer)
// 	if err != nil {
// 		return thor.Hash{}, thor.Hash{}, err
// 	}

// 	return blk.Header().ID(), respID.ID, nil
// }
