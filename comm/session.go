package comm

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type session struct {
	p          *peer
	rw         p2p.MsgReadWriter
	blockChain *chain.Chain
	txpl       *txpool.TxPool
}

func (s *session) DispatchMessage() error {
	msg, err := s.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	switch {
	case msg.Code == StatusMsg:
		// Status messages should never arrive after the handshake
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")
	case msg.Code == GetBlockHeadersMsg:
		return handleGetBlockHeadersMsg(msg, s)
	case msg.Code == TxMsg:
		return txMsg(msg, s)
	}

	return nil
}

// func (s *session) sendBlockHeaders(headers []*block.Header) error {
// 	return p2p.Send(s.rw, BlockHeadersMsg, headers)
// }

func (s *session) sendTransaction(tx *tx.Transaction) error {
	s.p.MarkTransaction(tx.ID())
	return p2p.Send(s.rw, TxMsg, tx)
}

func (s *session) sendBlockID(id thor.Hash) error {
	s.p.MarkBlock(id)
	return p2p.Send(s.rw, NewBlockIDMsg, id)
}

type sessions []*session

func (ss sessions) sessionsWithoutFilter(filter func(*session) bool) sessions {
	list := make(sessions, 0, len(ss))
	for _, s := range ss {
		if filter(s) {
			list = append(list, s)
		}
	}
	return list
}

// 该方法将以回调的方式设置到 txpool 中.
func (ss sessions) broadcastTx(tx *tx.Transaction) {
	filter := func(s *session) bool {
		return !s.p.knownTxs.Has(tx.ID())
	}

	for _, session := range ss.sessionsWithoutFilter(filter) {
		session.sendTransaction(tx)
	}
}

// 该方法将以回调的方式设置到 blockChain 中.
func (ss sessions) broadcastBlk(blk *block.Block) {
	filter := func(s *session) bool {
		return !s.p.knownBlocks.Has(blk.Header().ID())
	}

	for _, session := range ss.sessionsWithoutFilter(filter) {
		session.sendBlockID(blk.Header().ID())
	}
}
