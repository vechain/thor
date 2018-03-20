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
	"github.com/vechain/thor/comm/session"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
)

func (c *Communicator) getAllStatus(timeout *time.Timer) chan *status {
	slice := c.sessionSet.Slice()
	cn := make(chan *status, len(slice))

	var wg sync.WaitGroup
	wg.Add(len(slice))
	done := make(chan int)
	go func() {
		wg.Wait()
		done <- 1
	}()

	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	for _, s := range slice {
		go func(s *session.Session) {
			defer wg.Done()
			respSt, err := proto.ReqStatus{}.Do(ctx, s.Peer())
			if err != nil {
				return
			}
			cn <- &status{peer: s.Peer(), st: respSt}
		}(s)
	}

	select {
	case <-done:
	case <-timeout.C:
	case <-c.ctx.Done():
	}

	return cn
}

type status struct {
	peer *p2psrv.Peer
	st   *proto.RespStatus
}

func (c *Communicator) bestSession(genesisBlock *block.Block) *status {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	cn := c.getAllStatus(timer)
	if len(cn) == 0 {
		return nil
	}

	bestSt := &status{st: &proto.RespStatus{}}
	for {
		select {
		case st, ok := <-cn:
			if ok {
				if st.st.GenesisBlockID == genesisBlock.Header().ID() {
					if st.st.TotalScore > bestSt.st.TotalScore {
						bestSt = st
					} else if st.st.TotalScore == bestSt.st.TotalScore {
						if bytes.Compare(st.st.BestBlockID[:], bestSt.st.BestBlockID[:]) < 0 {
							bestSt = st
						}
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
	genesisBlock, err := c.ch.GetBlockByNumber(0)
	if err != nil {
		return fmt.Errorf("[sync]: %v", err)
	}

	st := c.bestSession(genesisBlock)
	if st == nil || (st.st.BestBlockID == thor.Hash{}) {
		return errors.New("don't have remote peer")
	}

	localBestBlock, err := c.ch.GetBestBlock()
	if err != nil {
		return fmt.Errorf("[sync]: %v", err)
	}

	if !c.isBetterThanLocal(localBestBlock, st.st) {
		return nil
	}

	ancestor, err := c.findAncestor(st.peer, 0, localBestBlock.Header().Number(), 0)
	if err != nil {
		return err
	}

	return c.download(st.peer, ancestor, block.Number(st.st.BestBlockID)-ancestor)
}

func (c *Communicator) download(peer *p2psrv.Peer, ancestor uint32, target uint32) error {
	for syned := 0; uint32(syned) < target; {
		blks, err := proto.ReqGetBlocksByNumber{Num: ancestor}.Do(c.ctx, peer)
		if err != nil {
			return err
		}
		syned += len(blks)
		ancestor += uint32(syned)
		for _, blk := range blks {
			go func(blk *block.Block) {
				select {
				case c.blockCh <- blk:
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

func (c *Communicator) findAncestor(peer *p2psrv.Peer, start uint32, end uint32, ancestor uint32) (uint32, error) {
	if start == end {
		localID, remoteID, err := c.getLocalAndRemoteIDByNumber(peer, start)
		if err != nil {
			return 0, err
		}

		if bytes.Compare(localID[:], remoteID[:]) == 0 {
			return start, nil
		}
	} else {
		mid := (start + end) / 2
		midID, remoteID, err := c.getLocalAndRemoteIDByNumber(peer, mid)
		if err != nil {
			return 0, err
		}

		if bytes.Compare(midID[:], remoteID[:]) == 0 {
			return c.findAncestor(peer, mid+1, end, mid)
		}

		if bytes.Compare(midID[:], remoteID[:]) != 0 {
			if mid > start {
				return c.findAncestor(peer, start, mid-1, ancestor)
			}
		}
	}

	return ancestor, nil
}

func (c *Communicator) getLocalAndRemoteIDByNumber(peer *p2psrv.Peer, num uint32) (thor.Hash, thor.Hash, error) {
	blk, err := c.ch.GetBlockByNumber(num)
	if err != nil {
		return thor.Hash{}, thor.Hash{}, fmt.Errorf("[findAncestor]: %v", err)
	}

	respID, err := proto.ReqGetBlockIDByNumber{Num: num}.Do(c.ctx, peer)
	if err != nil {
		return thor.Hash{}, thor.Hash{}, err
	}

	return blk.Header().ID(), respID.ID, nil
}
