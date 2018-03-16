package comm

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
)

func handleStatus(ch *chain.Chain) (*proto.RespStatus, error) {
	bestBlock, err := ch.GetBestBlock()
	if err != nil {
		return nil, err
	}

	genesisBlock, err := ch.GetBlockByNumber(0)
	if err != nil {
		return nil, err
	}

	header := bestBlock.Header()
	return &proto.RespStatus{
		GenesisBlockID: genesisBlock.Header().ID(),
		TotalScore:     header.TotalScore(),
		BestBlockID:    header.ID(),
	}, nil
}

func handleNewTx(msg *p2p.Msg, c *Communicator, peerID discover.NodeID) (*struct{}, error) {
	var reqTx proto.ReqMsgNewTx
	if err := msg.Decode(&reqTx); err != nil {
		return nil, err
	}
	c.markTransaction(peerID, reqTx.Tx.ID())
	c.txpl.Add(reqTx.Tx)
	return &struct{}{}, nil
}

func handleNewBlock(msg *p2p.Msg, c *Communicator) (*struct{}, error) {
	var reqBlk proto.ReqNewBlock
	if err := msg.Decode(&reqBlk); err != nil {
		return nil, err
	}
	go func() {
		select {
		case c.blockCh <- reqBlk.Block:
		case <-c.ctx.Done():
		}
	}()
	return &struct{}{}, nil
}

func handleNewBlockID(msg *p2p.Msg, c *Communicator, session *p2psrv.Session) (*struct{}, error) {
	var reqID proto.ReqNewBlockID
	if err := msg.Decode(&reqID); err != nil {
		return nil, err
	}
	c.markBlock(session.Peer().ID(), reqID.ID)
	if _, err := c.ch.GetBlock(reqID.ID); err != nil {
		if c.ch.IsNotFound(err) {
			go func() {
				select {
				case c.unKnownBlockCh <- &unKnown{session: session, id: reqID.ID}:
				case <-c.ctx.Done():
				}
			}()
			return &struct{}{}, nil
		}
		return nil, err
	}
	return &struct{}{}, nil
}

func handleGetBlockIDByNumber(msg *p2p.Msg, ch *chain.Chain) (*proto.RespGetBlockIDByNumber, error) {
	var reqNum proto.ReqGetBlockIDByNumber
	if err := msg.Decode(&reqNum); err != nil {
		return nil, err
	}
	id, err := ch.GetBlockIDByNumber(reqNum.Num)
	if err != nil {
		return nil, err
	}
	return &proto.RespGetBlockIDByNumber{ID: id}, nil
}

func handleGetBlocksByNumber(msg *p2p.Msg, ch *chain.Chain) (proto.RespGetBlocksByNumber, error) {
	var reqNum proto.ReqGetBlocksByNumber
	if err := msg.Decode(&reqNum); err != nil {
		return nil, err
	}

	bestBlk, err := ch.GetBestBlock()
	if err != nil {
		return nil, err
	}

	blks := make(proto.RespGetBlocksByNumber, 0, 10)
	for i := 0; i < 10; i++ {
		reqNum.Num++
		if uint32(reqNum.Num) > bestBlk.Header().Number() {
			break
		}

		blk, err := ch.GetBlockByNumber(reqNum.Num)
		if err != nil {
			return nil, err
		}

		blks = append(blks, blk)
	}

	return blks, nil
}

func handleGetBlockByID(msg *p2p.Msg, ch *chain.Chain) (*proto.RespGetBlockByID, error) {
	var reqID proto.ReqGetBlockByID
	if err := msg.Decode(&reqID); err != nil {
		return nil, err
	}

	blk, err := ch.GetBlock(reqID.ID)
	if err != nil {
		return nil, err
	}

	return &proto.RespGetBlockByID{Block: blk}, nil
}
