package comm

import (
	"errors"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
)

type announce struct {
	blockID thor.Hash
	peer    *p2psrv.Peer
}

func (c *Communicator) handleRequest(peer *p2psrv.Peer, msg *p2p.Msg) (interface{}, error) {
	switch msg.Code {
	case proto.MsgStatus:
		bestBlock, err := c.chain.GetBestBlock()
		if err != nil {
			return nil, err
		}
		return &proto.RespStatus{
			GenesisBlockID: c.genesisID,
			TotalScore:     bestBlock.Header().TotalScore(),
			BestBlockID:    bestBlock.Header().ID(),
		}, nil
	case proto.MsgNewTx:
		var req proto.ReqMsgNewTx
		if err := msg.Decode(&req); err != nil {
			return nil, badRequest{err}
		}
		if req.Tx == nil {
			return nil, badRequest{errors.New("nil tx")}
		}
		if s := c.sessionSet.Find(peer.ID()); s != nil {
			s.MarkTransaction(req.Tx.ID())
		}
		c.goes.Go(func() { c.txFeed.Send(req.Tx) })
		return &struct{}{}, nil
	case proto.MsgNewBlock:
		var req proto.ReqNewBlock
		if err := msg.Decode(&req); err != nil {
			return nil, badRequest{err}
		}
		if req.Block == nil {
			return nil, badRequest{errors.New("nil block")}
		}
		if s := c.sessionSet.Find(peer.ID()); s != nil {
			s.MarkBlock(req.Block.Header().ID())
			s.UpdateTrunkHead(req.Block.Header().ID(), req.Block.Header().TotalScore())
		}
		c.goes.Go(func() { c.blockFeed.Send(req.Block) })
		return &struct{}{}, nil
	case proto.MsgNewBlockID:
		var req proto.ReqNewBlockID
		if err := msg.Decode(&req); err != nil {
			return nil, badRequest{err}
		}

		if s := c.sessionSet.Find(peer.ID()); s != nil {
			s.MarkBlock(req.ID)
		}
		c.goes.Go(func() { c.announceCh <- &announce{req.ID, peer} })
		return &struct{}{}, nil
	case proto.MsgGetBlockByID:
		var req proto.ReqGetBlockByID
		if err := msg.Decode(&req); err != nil {
			return nil, badRequest{err}
		}
		blk, err := c.chain.GetBlock(req.ID)
		if err != nil {
			return nil, err
		}
		if s := c.sessionSet.Find(peer.ID()); s != nil {
			s.MarkBlock(req.ID)
		}
		return &proto.RespGetBlockByID{Block: blk}, nil
	case proto.MsgGetBlockIDByNumber:
		var req proto.ReqGetBlockIDByNumber
		if err := msg.Decode(&req); err != nil {
			return nil, badRequest{err}
		}
		id, err := c.chain.GetBlockIDByNumber(req.Num)
		if err != nil {
			return nil, err
		}
		if s := c.sessionSet.Find(peer.ID()); s != nil {
			s.MarkBlock(id)
		}
		return &proto.RespGetBlockIDByNumber{ID: id}, nil
	case proto.MsgGetBlocksFromNumber:
		var req proto.ReqGetBlocksFromNumber
		if err := msg.Decode(&req); err != nil {
			return nil, badRequest{err}
		}

		const maxRespSize = 2 * 1024 * 1024
		const maxBlocks = 256
		resp := make(proto.RespGetBlocksFromNumber, 0, 100)
		num := req.Num
		var size metric.StorageSize
		for size < maxRespSize && len(resp) < maxBlocks {
			blk, err := c.chain.GetBlockByNumber(num)
			if err != nil {
				if c.chain.IsNotFound(err) {
					break
				}
				return nil, err
			}
			resp = append(resp, blk)
			num++
			size += blk.Size()
		}
		if s := c.sessionSet.Find(peer.ID()); s != nil {
			for _, blk := range resp {
				s.MarkBlock(blk.Header().ID())
			}
		}
		return resp, nil
	}
	return nil, errors.New("unexpected message")
}

type badRequest struct {
	err error
}

func (b badRequest) Error() string {
	return "bad request: " + b.err.Error()
}

func isPeerAlive(peer *p2psrv.Peer) bool {
	select {
	case <-peer.Done():
		return false
	default:
		return true
	}
}
