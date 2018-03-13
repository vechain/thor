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
	ps          p2pServer
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

func (c *Communicator) sessionsWithoutFilter(filter func(*p2psrv.Session) bool) []*p2psrv.Session {
	sset := c.ps.SessionSet()

	list := make([]*p2psrv.Session, 0, sset.Len())
	for _, s := range sset.All() {
		if filter(s) {
			list = append(list, s)
		}
	}
	return list
}

type status struct {
	session     *p2psrv.Session
	totalScore  uint64
	bestBlockID thor.Hash
}

// timeout 移到参数, 内部加上 routine 执行完判断
func (c *Communicator) getAllStatus(timeout *time.Timer) chan *status {
	bestBlock, err := c.ch.GetBestBlock()
	if err != nil {
		log.Fatalf("[sync]: %v\n", err)
	}
	header := bestBlock.Header()
	localSt := &status{
		totalScore:  header.TotalScore(),
		bestBlockID: header.ID(),
	}

	sset := c.ps.SessionSet()
	ctx, cancel := context.WithCancel(context.Background())
	cn := make(chan *status, sset.Len())

	var wg sync.WaitGroup
	wg.Add(sset.Len())
	done := make(chan int)
	go func() {
		wg.Wait()
		done <- 1
	}()

	for _, session := range sset.All() {
		go func(s *p2psrv.Session) {
			defer wg.Done()
			st := &status{}
			if err := s.Request(ctx, StatusMsg, localSt, st); err != nil {
				return
			}
			cn <- st
		}(session)
	}

	select {
	case <-done:
	case <-timeout.C:
	}
	cancel()

	return cn
}

func (c *Communicator) bestSession() *status {
	bestSt := &status{}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	cn := c.getAllStatus(timer)

	for {
		select {
		case st, ok := <-cn:
			if ok {
				if st.totalScore > bestSt.totalScore {
					bestSt = st
				} else if st.totalScore == bestSt.totalScore {
					if bytes.Compare(st.bestBlockID[:], bestSt.bestBlockID[:]) < 0 {
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

	if bestBlock.Header().TotalScore() > st.totalScore {
		return
	}

	if bestBlock.Header().TotalScore() == st.totalScore {
		bestBlockID := bestBlock.Header().ID()
		if bytes.Compare(bestBlockID[:], st.bestBlockID[:]) < 0 {
			return
		}
	}

	c.findAncestor(0, bestBlock.Header().Number(), 0)
}

func (c *Communicator) findAncestor(start uint32, end uint32, ancestor uint32) uint32 {
	if start == end {
		localID, remoteID := c.getLocalAndRemoteIDByNumber(start)
		if bytes.Compare(localID[:], remoteID[:]) == 0 {
			return start
		}
	} else {
		mid := (start + end) / 2
		midID, remoteID := c.getLocalAndRemoteIDByNumber(mid)

		if bytes.Compare(midID[:], remoteID[:]) == 0 {
			return c.findAncestor(mid+1, end, mid)
		}

		if bytes.Compare(midID[:], remoteID[:]) != 0 {
			if mid > start {
				return c.findAncestor(start, mid-1, ancestor)
			}
		}
	}

	return ancestor
}

func (c *Communicator) getLocalAndRemoteIDByNumber(num uint32) (thor.Hash, thor.Hash) {
	blk, err := c.ch.GetBlockByNumber(num)
	if err != nil {
		log.Fatalf("[findAncestor]: %v\n", err)
	}
	return blk.Header().ID(), requestBlockHashByNumber(num)
}

// BroadcastTx 广播新插入池的交易
func (c *Communicator) BroadcastTx(tx *tx.Transaction) {
	filter := func(s *p2psrv.Session) bool {
		return !c.knownBlocks[s.Peer().ID()].Has(tx.ID())
	}

	for _, session := range c.sessionsWithoutFilter(filter) {
		//session.notifyTransaction(tx)
	}
}

// BroadcastBlk 广播新插入链的块
func (c *Communicator) BroadcastBlock(blk *block.Block) {
	filter := func(s *p2psrv.Session) bool {
		return !c.knownTxs[s.Peer().ID()].Has(blk.Header().ID())
	}

	for _, session := range c.sessionsWithoutFilter(filter) {
		//session.notifyBlockID(blk.Header().ID())
	}
}
