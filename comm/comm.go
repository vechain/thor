package comm

import (
	"bytes"
	"log"
	"time"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

const forceSyncCycle = 10 * time.Second

type Comm struct {
	sessions []*session
	ch       *chain.Chain
	txpl     *txpool.TxPool
}

func (c Comm) sessionsWithoutFilter(filter func(*session) bool) []*session {
	list := make([]*session, 0, len(c.sessions))
	for _, s := range c.sessions {
		if filter(s) {
			list = append(list, s)
		}
	}
	return list
}

func (c Comm) bestSession() *session {
	var (
		bestSession *session
		bestTc      uint64
	)

	for _, s := range c.sessions {
		if s.totalScore > bestTc {
			bestTc = s.totalScore
			bestSession = s
		}
	}

	return bestSession
}

func (c Comm) sync() {
	s := c.bestSession()

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

func (c Comm) BroadcastTx(tx *tx.Transaction) {
	filter := func(s *session) bool {
		return !s.knownTxs.Has(tx.ID())
	}

	for _, session := range c.sessionsWithoutFilter(filter) {
		session.notifyTransaction(tx)
	}
}

func (c Comm) BroadcastBlk(blk *block.Block) {
	filter := func(s *session) bool {
		return !s.knownBlocks.Has(blk.Header().ID())
	}

	for _, session := range c.sessionsWithoutFilter(filter) {
		session.notifyBlockID(blk.Header().ID())
	}
}
