package comm

import (
	"bytes"
	"context"
	"log"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	set "gopkg.in/fatih/set.v0"
)

const (
	maxKnownTxs    = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 1024  // Maximum block hashes to keep in the known list (prevent DOS)
)

type Communicator struct {
	ps          *p2psrv.Server
	knownBlocks map[discover.NodeID]*set.Set
	knownTxs    map[discover.NodeID]*set.Set
	ch          *chain.Chain
	txpl        *txpool.TxPool
}

func New() *Communicator {
	return &Communicator{}
}

func (c *Communicator) Protocols() []*p2psrv.Protocol {
	return []*p2psrv.Protocol{
		&p2psrv.Protocol{
			Name:          proto.Name,
			Version:       proto.Version,
			Length:        proto.Length,
			MaxMsgSize:    proto.MaxMsgSize,
			HandleRequest: c.handleRequest,
		},
	}
}

func (c *Communicator) handleRequest(session *p2psrv.Session, msg *p2p.Msg) (resp interface{}, err error) {
	switch {
	case msg.Code == proto.MsgStatus:
		bestBlock, err := c.ch.GetBestBlock()
		if err != nil {
			return nil, err
		}
		header := bestBlock.Header()
		return &proto.RespStatus{
			TotalScore:  header.TotalScore(),
			BestBlockID: header.ID(),
		}, nil
	case msg.Code == proto.MsgNewTx:
		tx := &tx.Transaction{}
		if err := msg.Decode(tx); err != nil {
			return nil, err
		}
		c.markTransaction(session.Peer().ID(), tx.ID())
		c.txpl.Add(tx)
		return &struct{}{}, nil
	case msg.Code == proto.MsgNewBlockID:
		var id thor.Hash
		if err := msg.Decode(&id); err != nil {
			return nil, err
		}
		c.markBlock(session.Peer().ID(), id)
		if _, err := c.ch.GetBlock(id); err != nil {
			if c.ch.IsNotFound(err) {
				//pm.fetcher.Notify(p.id, block.Hash, block.Number, time.Now(), p.RequestOneHeader, p.RequestBodies)
			}
			return nil, err
		}
		return &struct{}{}, nil
	}
	return nil, nil
}

// 需要考虑线程同步的问题
func (c *Communicator) markTransaction(peer discover.NodeID, id thor.Hash) {
	for c.knownBlocks[peer].Size() >= maxKnownBlocks {
		c.knownBlocks[peer].Pop()
	}
	c.knownBlocks[peer].Add(id)
}

func (c *Communicator) markBlock(peer discover.NodeID, id thor.Hash) {
	for c.knownTxs[peer].Size() >= maxKnownTxs {
		c.knownTxs[peer].Pop()
	}
	c.knownTxs[peer].Add(id)
}

// timeout 移到参数, 内部加上 routine 执行完判断
func (c *Communicator) getAllStatus(timeout *time.Timer) chan *proto.RespStatus {
	ss := c.ps.Sessions()
	ctx, cancel := context.WithCancel(context.Background())
	cn := make(chan *proto.RespStatus, len(ss))

	var wg sync.WaitGroup
	wg.Add(len(ss))
	done := make(chan int)
	go func() {
		wg.Wait()
		done <- 1
	}()

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
	}
	cancel()

	return cn
}

func (c *Communicator) bestSession() *proto.RespStatus {
	bestSt := &proto.RespStatus{}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	cn := c.getAllStatus(timer)

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
		default:
			return bestSt
		}
	}
}

func (c *Communicator) sync() {
	st := c.bestSession()

	// Make sure the peer's TD is higher than our own
	bestBlock, err := c.ch.GetBestBlock()
	if err != nil {
		log.Fatalf("[sync]: %v\n", err)
	}

	if bestBlock.Header().TotalScore() > st.TotalScore {
		return
	}

	if bestBlock.Header().TotalScore() == st.TotalScore {
		bestBlockID := bestBlock.Header().ID()
		if bytes.Compare(bestBlockID[:], st.BestBlockID[:]) < 0 {
			return
		}
	}

	ancestor, err := c.findAncestor(st.Session, 0, bestBlock.Header().Number(), 0)
	if err != nil {
		return
	}

	_ = ancestor
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
		log.Fatalf("[findAncestor]: %v\n", err)
	}
	remoteID, err := proto.ReqGetBlockIDByNumber(num).Do(context.Background(), s)
	if err != nil {
		return thor.Hash{}, thor.Hash{}, err
	}

	return blk.Header().ID(), thor.Hash(*remoteID), nil
}

// BroadcastTx 广播新插入池的交易
func (c *Communicator) BroadcastTx(tx *tx.Transaction) {
	cond := func(s *p2psrv.Session) bool {
		return !c.knownBlocks[s.Peer().ID()].Has(tx.ID())
	}

	ss := c.ps.Sessions().Filter(cond)
	_ = ss
}

// BroadcastBlk 广播新插入链的块
func (c *Communicator) BroadcastBlock(blk *block.Block) {
	cond := func(s *p2psrv.Session) bool {
		return !c.knownTxs[s.Peer().ID()].Has(blk.Header().ID())
	}

	ss := c.ps.Sessions().Filter(cond)
	_ = ss
}
