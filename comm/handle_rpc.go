package comm

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/metric"
)

func (c *Communicator) handleRPC(peer *Peer, msg *p2p.Msg, w func(interface{})) (err error) {
	const maxResultSize = 2 * 1024 * 1024
	defer func() {
		if _, bad := err.(badCall); bad {
			peer.Disconnect(p2p.DiscSubprotocolError)
		}
	}()

	log := peer.logger.New("msg", proto.MsgName(msg.Code))
	log.Debug("received RPC msg")

	switch msg.Code {
	case proto.MsgStatus:
		best := c.chain.BestBlock().Header()
		w(&proto.StatusResult{
			GenesisBlockID: c.chain.GenesisBlock().Header().ID(),
			SysTimestamp:   uint64(time.Now().Unix()),
			TotalScore:     best.TotalScore(),
			BestBlockID:    best.ID(),
		})
	case proto.MsgNewBlock:
		var arg proto.NewBlock
		if err := msg.Decode(&arg); err != nil {
			return badCall{err}
		}
		if arg.Block == nil {
			return badCall{errors.New("nil block")}
		}

		peer.MarkBlock(arg.Block.Header().ID())
		peer.UpdateHead(arg.Block.Header().ID(), arg.Block.Header().TotalScore())

		c.goes.Go(func() {
			c.newBlockFeed.Send(&NewBlockEvent{Block: arg.Block})
		})
		w(&struct{}{})
	case proto.MsgNewBlockID:
		var arg proto.NewBlockID
		if err := msg.Decode(&arg); err != nil {
			return badCall{err}
		}
		peer.MarkBlock(arg.ID)
		c.goes.Go(func() { c.handleAnnounce(arg.ID, peer) })
		w(&struct{}{})
	case proto.MsgNewTx:
		var arg proto.NewTx
		if err := msg.Decode(&arg); err != nil {
			return badCall{err}
		}
		if arg.Tx == nil {
			return badCall{errors.New("nil tx")}
		}
		peer.MarkTransaction(arg.Tx.ID())
		c.txPool.Add(arg.Tx)
		w(&struct{}{})
	case proto.MsgGetBlockByID:
		var arg proto.GetBlockByID
		if err := msg.Decode(&arg); err != nil {
			return badCall{err}
		}
		blk, err := c.chain.GetBlock(arg.ID)
		if err != nil {
			if !c.chain.IsNotFound(err) {
				log.Error("failed to get block", "err", err)
			}
			return nil
		}
		peer.MarkBlock(arg.ID)
		w(&proto.GetBlockByIDResult{Block: blk})
	case proto.MsgGetBlockIDByNumber:
		var arg proto.GetBlockIDByNumber
		if err := msg.Decode(&arg); err != nil {
			return badCall{err}
		}
		id, err := c.chain.GetBlockIDByNumber(arg.Num)
		if err != nil {
			if !c.chain.IsNotFound(err) {
				log.Error("failed to get block id by number", "err", err)
			}
			return nil
		}
		w(&proto.GetBlockIDByNumberResult{ID: id})
	case proto.MsgGetBlocksFromNumber:
		var arg proto.GetBlocksFromNumber
		if err := msg.Decode(&arg); err != nil {
			return badCall{err}
		}

		const maxBlocks = 1024
		result := make(proto.GetBlocksFromNumberResult, 0, maxBlocks)
		num := arg.Num
		var size metric.StorageSize
		for size < maxResultSize && len(result) < maxBlocks {
			raw, err := c.chain.GetBlockRawByNumber(num)
			if err != nil {
				if c.chain.IsNotFound(err) {
					break
				}
				log.Error("failed to get block raw by number", "err", err)
				return nil
			}
			result = append(result, rlp.RawValue(raw))
			num++
			size += metric.StorageSize(len(raw))
		}
		w(result)
	case proto.MsgGetTxs:
		txs := c.txPool.Pending(false)
		result := make(proto.GetTxsResult, 0, len(txs))
		var size metric.StorageSize
		for _, tx := range txs {
			size += tx.Size()
			if size > maxResultSize {
				break
			}
			result = append(result, tx)
		}
		w(result)
	default:
		return badCall{fmt.Errorf("unexpected message code(%v)", msg.Code)}
	}
	return nil
}

type badCall struct {
	err error
}

func (b badCall) Error() string {
	return "bad call: " + b.err.Error()
}
