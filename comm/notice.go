package comm

import (
	"github.com/vechain/thor/thor"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/tx"
)

func txMsg(msg p2p.Msg, s *session) error {
	var tx *tx.Transaction
	if err := msg.Decode(&tx); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}

	if tx == nil {
		return errResp(ErrDecode, "transaction is nil")
	}

	s.MarkTransaction(tx.ID())
	s.txpl.Add(tx)

	return nil
}

func blockIDMsg(msg p2p.Msg, s *session) error {
	var id thor.Hash
	if err := msg.Decode(&id); err != nil {
		return errResp(ErrDecode, "%v: %v", msg, err)
	}

	s.MarkBlock(id)
	if _, err := s.blockChain.GetBlock(id); err != nil {
		if s.blockChain.IsNotFound(err) {
			//pm.fetcher.Notify(p.id, block.Hash, block.Number, time.Now(), p.RequestOneHeader, p.RequestBodies)
		}
		return errResp(ErrDecode, "%v: %v", msg, err)
	}

	return nil
}
