package comm

import (
	"bytes"
	"context"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
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

type Comm struct {
	ps          p2pServer
	knownBlocks map[discover.NodeID]*set.Set
	knownTxs    map[discover.NodeID]*set.Set
	ch          *chain.Chain
	txpl        *txpool.TxPool
}

func (c *Comm) markTransaction(peer discover.NodeID, id thor.Hash) {
	for c.knownBlocks[peer].Size() >= maxKnownBlocks {
		c.knownBlocks[peer].Pop()
	}
	c.knownBlocks[peer].Add(id)
}

func (c *Comm) markBlock(peer discover.NodeID, id thor.Hash) {
	for c.knownTxs[peer].Size() >= maxKnownTxs {
		c.knownTxs[peer].Pop()
	}
	c.knownTxs[peer].Add(id)
}

func (c *Comm) sessionsWithoutFilter(filter func(*p2psrv.Session) bool) []*p2psrv.Session {
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

func (c *Comm) getStatus() chan *status {
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

	for _, session := range sset.All() {
		go func(s *p2psrv.Session) {
			st := &status{}
			if err := s.Request(ctx, StatusMsg, localSt, st); err != nil {
				return
			}
			cn <- st
		}(session)
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	<-timer.C
	cancel()

	return cn
}

func (c *Comm) bestSession() *status {
	bestSt := &status{}
	cn := c.getStatus()

	for {
		select {
		case st, ok := <-cn:
			if ok {
				if st.totalScore > bestSt.totalScore {
					bestSt = st
				} else if st.totalScore == bestSt.totalScore {
					if bytes.Compare(st.bestBlockID[:], bestSt.bestBlockID[:]) > 0 {
						bestSt = st
					}
				}
			}
		default:
			return bestSt
		}
	}
}

func (c *Comm) sync() {
	st := c.bestSession()

	// Make sure the peer's TD is higher than our own
	bestBlock, err := c.ch.GetBestBlock()
	if err != nil {
		log.Fatalf("[sync]: %v\n", err)
	}

	if bestBlock.Header().TotalScore() > s.totalScore {
		return
	}

	if bestBlock.Header().TotalScore() == s.totalScore {
		bestBlockID := bestBlock.Header().ID()
		if bytes.Compare(bestBlockID[:], s.bestBlockID[:]) > 0 {
			return
		}
	}

	s.findAncestor(0, bestBlock.Header().Number(), 0, c.ch)
}

// BroadcastTx 广播新插入池的交易
func (c *Comm) BroadcastTx(tx *tx.Transaction) {
	filter := func(s *p2psrv.Session) bool {
		return !c.knownBlocks[s.Peer().ID()].Has(tx.ID())
	}

	for _, session := range c.sessionsWithoutFilter(filter) {
		//session.notifyTransaction(tx)
	}
}

// BroadcastBlk 广播新插入链的块
func (c *Comm) BroadcastBlk(blk *block.Block) {
	filter := func(s *p2psrv.Session) bool {
		return !c.knownTxs[s.Peer().ID()].Has(blk.Header().ID())
	}

	for _, session := range c.sessionsWithoutFilter(filter) {
		//session.notifyBlockID(blk.Header().ID())
	}
}
