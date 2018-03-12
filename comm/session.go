package comm

import (
	"bytes"
	"log"
	"sync"

	"github.com/vechain/thor/chain"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type session struct {
	*peer
	rw p2p.MsgReadWriter

	blockHeaderMsgNum uint32
	blockHeaderMsg    map[uint32]chan *block.Header

	blockBodyMsgNum uint32
	blockBodyMsg    map[uint32]chan *block.Header

	msgQueue []p2p.Msg
	lock     sync.RWMutex
}

func (s *session) findAncestor(start uint32, end uint32, ancestor uint32, ch *chain.Chain) uint32 {
	if start == end {
		localID, remoteID := getLocalAndRemoteIDByNumber(start, ch)
		if bytes.Compare(localID[:], remoteID[:]) == 0 {
			return start
		}
	} else {
		mid := (start + end) / 2
		midID, remoteID := getLocalAndRemoteIDByNumber(mid, ch)

		if bytes.Compare(midID[:], remoteID[:]) == 0 {
			return s.findAncestor(mid+1, end, mid, ch)
		}

		if bytes.Compare(midID[:], remoteID[:]) != 0 {
			if mid > start {
				return s.findAncestor(start, mid-1, ancestor, ch)
			}
		}
	}

	return ancestor
}

func getLocalAndRemoteIDByNumber(num uint32, ch *chain.Chain) (thor.Hash, thor.Hash) {
	blk, err := ch.GetBlockByNumber(num)
	if err != nil {
		log.Fatalf("[findAncestor]: %v\n", err)
	}
	return blk.Header().ID(), requestBlockHashByNumber(num)
}

func (s *session) safeAppendMsg(msg p2p.Msg) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.msgQueue = append(s.msgQueue, msg)
}

func (s *session) remoteMsg() error {
	msg, err := s.rw.ReadMsg()
	if err != nil {
		return err
	}
	s.safeAppendMsg(msg)
	return nil
}

func (s *session) localMsg() error {
	msg, err := s.rw.ReadMsg()
	if err != nil {
		return err
	}
	s.safeAppendMsg(msg)
	return nil
}

// func (s *session) PeekMessage() msg {
// 	s.lock.Lock()
// 	defer s.lock.Unlock()

// }

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
	case msg.Code == GetBlockHeaderMsg:
		//return requestHeader(msg, s)
	case msg.Code == BlockHeaderMsg:
		var bh blockHeaderDate
		if err := msg.Decode(&bh); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		if ch, ok := s.blockHeaderMsg[bh.num]; ok {
			ch <- bh.header
		}
	case msg.Code == TxMsg:
		//return txMsg(msg, s)
	case msg.Code == BlockIDMsg:
		// return blockIDMsg(msg, s)
	}

	return nil
}

func (s *session) notifyTransaction(tx *tx.Transaction) error {
	s.MarkTransaction(tx.ID())
	return p2p.Send(s.rw, TxMsg, tx)
}

func (s *session) notifyBlockID(id thor.Hash) error {
	s.MarkBlock(id)
	return p2p.Send(s.rw, BlockIDMsg, id)
}

func (s *session) GetHeader(id thor.Hash) (*block.Header, error) {
	msgNum := s.blockHeaderMsgNum
	s.blockHeaderMsg[msgNum] = make(chan *block.Header)
	if err := p2p.Send(s.rw, GetBlockHeaderMsg, &getBlockHeaderDate{id: id, num: msgNum}); err != nil {
		return nil, err
	}

	header := <-s.blockHeaderMsg[msgNum]
	delete(s.blockHeaderMsg, msgNum)

	return header, nil
}

// func (s *session) getBody() error {
// 	//p2p.Send(s.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
// }

// func (s *session) getBlock() error {
// 	//p2p.Send(s.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
// }
