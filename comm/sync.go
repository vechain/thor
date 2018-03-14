package comm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
)

func (c *Communicator) getAllStatus(timeout *time.Timer) chan *proto.RespStatus {
	ss := c.ps.Sessions()
	cn := make(chan *proto.RespStatus, len(ss))

	var wg sync.WaitGroup
	wg.Add(len(ss))
	done := make(chan int)
	go func() {
		wg.Wait()
		done <- 1
	}()

	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	for _, session := range ss {
		go func(s *p2psrv.Session) {
			defer wg.Done()
			respSt, err := proto.ReqStatus{}.Do(ctx, s)
			if err != nil {
				return
			}
			respSt.Session = s
			cn <- respSt
		}(session)
	}

	select {
	case <-done:
	case <-timeout.C:
	case <-c.ctx.Done():
	}

	return cn
}

func (c *Communicator) bestSession() *proto.RespStatus {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	cn := c.getAllStatus(timer)
	if len(cn) == 0 {
		return nil
	}

	bestSt := &proto.RespStatus{}
	for {
		select {
		case st, ok := <-cn:
			if ok {
				if st.TotalScore > bestSt.TotalScore {
					bestSt = st
				} else if st.TotalScore == bestSt.TotalScore {
					if bytes.Compare(st.BestBlockID[:], bestSt.BestBlockID[:]) < 0 {
						bestSt = st
					}
				}
			}
		case <-c.ctx.Done():
			return nil
		default:
			return bestSt
		}
	}
}

func (c *Communicator) sync() error {
	st := c.bestSession()
	if st == nil {
		return errors.New("don't have remote peer")
	}

	localBestBlock, err := c.ch.GetBestBlock()
	if err != nil {
		return fmt.Errorf("[sync]: %v", err)
	}

	if !c.isBetterThanLocal(localBestBlock, st) {
		return nil
	}

	ancestor, err := c.findAncestor(st.Session, 0, localBestBlock.Header().Number(), 0)
	if err != nil {
		return err
	}

	return c.download(st.Session, ancestor, block.Number(st.BestBlockID)-ancestor)
}

func (c *Communicator) download(remote *p2psrv.Session, ancestor uint32, target uint32) error {
	for syned := 0; uint32(syned) < target; {
		blks, err := proto.ReqGetBlocksByNumber{Num: ancestor}.Do(c.ctx, remote)
		if err != nil {
			return err
		}
		syned += len(blks)
		ancestor += uint32(syned)
		for _, blk := range blks {
			go func(blk *block.Block) {
				select {
				case c.BlockCh <- blk:
				case <-c.ctx.Done():
				}
			}(blk)
		}
	}
	return nil
}

func (c *Communicator) isBetterThanLocal(localBestBlock *block.Block, st *proto.RespStatus) bool {
	if localBestBlock.Header().TotalScore() > st.TotalScore {
		return false
	}

	if localBestBlock.Header().TotalScore() == st.TotalScore {
		bestBlockID := localBestBlock.Header().ID()
		if bytes.Compare(bestBlockID[:], st.BestBlockID[:]) < 0 {
			return false
		}
	}

	return true
}

func (c *Communicator) findAncestor(s *p2psrv.Session, start uint32, end uint32, ancestor uint32) (uint32, error) {
	if start == end {
		localID, remoteID, err := c.getLocalAndRemoteIDByNumber(s, start)
		if err != nil {
			return 0, err
		}

		if bytes.Compare(localID[:], remoteID[:]) == 0 {
			return start, nil
		}
	} else {
		mid := (start + end) / 2
		midID, remoteID, err := c.getLocalAndRemoteIDByNumber(s, mid)
		if err != nil {
			return 0, err
		}

		if bytes.Compare(midID[:], remoteID[:]) == 0 {
			return c.findAncestor(s, mid+1, end, mid)
		}

		if bytes.Compare(midID[:], remoteID[:]) != 0 {
			if mid > start {
				return c.findAncestor(s, start, mid-1, ancestor)
			}
		}
	}

	return ancestor, nil
}

func (c *Communicator) getLocalAndRemoteIDByNumber(s *p2psrv.Session, num uint32) (thor.Hash, thor.Hash, error) {
	blk, err := c.ch.GetBlockByNumber(num)
	if err != nil {
		return thor.Hash{}, thor.Hash{}, fmt.Errorf("[findAncestor]: %v", err)
	}
	respID, err := proto.ReqGetBlockIDByNumber{Num: num}.Do(c.ctx, s)
	if err != nil {
		return thor.Hash{}, thor.Hash{}, err
	}

	return blk.Header().ID(), respID.ID, nil
}
