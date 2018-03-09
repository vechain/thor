package comm

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/tx"
)

type Comm []*session

func (c Comm) commWithoutFilter(filter func(*session) bool) Comm {
	list := make(Comm, 0, len(c))
	for _, s := range c {
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

	for _, s := range c {
		if s.totalScore > bestTc {
			bestTc = s.totalScore
			bestSession = s
		}
	}

	return bestSession
}

// 该方法将以回调的方式设置到 txpool 中.
func (c Comm) BroadcastTx(tx *tx.Transaction) {
	filter := func(s *session) bool {
		return !s.knownTxs.Has(tx.ID())
	}

	for _, session := range c.commWithoutFilter(filter) {
		session.notifyTransaction(tx)
	}
}

// 该方法将以回调的方式设置到 blockChain 中.
func (c Comm) BroadcastBlk(blk *block.Block) {
	filter := func(s *session) bool {
		return !s.knownBlocks.Has(blk.Header().ID())
	}

	for _, session := range c.commWithoutFilter(filter) {
		session.notifyBlockID(blk.Header().ID())
	}
}
