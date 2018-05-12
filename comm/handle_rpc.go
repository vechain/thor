package comm

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/metric"
)

// peer will be disconnected if error returned
func (c *Communicator) handleRPC(peer *Peer, msg *p2p.Msg, write func(interface{})) (err error) {
	const maxResultSize = 2 * 1024 * 1024

	log := peer.logger.New("msg", proto.MsgName(msg.Code))
	log.Debug("received RPC call")
	defer func() {
		if err != nil {
			log.Debug("failed to handle RPC call", "err", err)
		}
	}()

	switch msg.Code {
	case proto.MsgStatus:
		var arg proto.Status
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		best := c.chain.BestBlock().Header()
		write(&proto.StatusResult{
			GenesisBlockID: c.chain.GenesisBlock().Header().ID(),
			SysTimestamp:   uint64(time.Now().Unix()),
			TotalScore:     best.TotalScore(),
			BestBlockID:    best.ID(),
		})
	case proto.MsgNewBlock:
		var arg proto.NewBlock
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		peer.MarkBlock(arg.Block.Header().ID())
		peer.UpdateHead(arg.Block.Header().ID(), arg.Block.Header().TotalScore())
		c.newBlockFeed.Send(&NewBlockEvent{Block: arg.Block})
		write(&struct{}{})
	case proto.MsgNewBlockID:
		var arg proto.NewBlockID
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		peer.MarkBlock(arg.ID)
		select {
		case <-c.ctx.Done():
		case c.announcementCh <- &announcement{arg.ID, peer}:
		}
		write(&struct{}{})
	case proto.MsgNewTx:
		var arg proto.NewTx
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		peer.MarkTransaction(arg.Tx.ID())
		c.txPool.Add(arg.Tx)
		write(&struct{}{})
	case proto.MsgGetBlockByID:
		var arg proto.GetBlockByID
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		blk, err := c.chain.GetBlock(arg.ID)
		if err != nil {
			if !c.chain.IsNotFound(err) {
				log.Error("failed to get block", "err", err)
			}
			write(&proto.GetBlockByIDResult{})
		} else {
			write(&proto.GetBlockByIDResult{Block: blk})
		}
	case proto.MsgGetBlockIDByNumber:
		var arg proto.GetBlockIDByNumber
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		id, err := c.chain.GetBlockIDByNumber(arg.Num)
		if err != nil {
			if !c.chain.IsNotFound(err) {
				log.Error("failed to get block id by number", "err", err)
			}
			write(&proto.GetBlockIDByNumberResult{})
		} else {
			write(&proto.GetBlockIDByNumberResult{ID: id})
		}
	case proto.MsgGetBlocksFromNumber:
		var arg proto.GetBlocksFromNumber
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		const maxBlocks = 1024
		result := make(proto.GetBlocksFromNumberResult, 0, maxBlocks)
		num := arg.Num
		var size metric.StorageSize
		for size < maxResultSize && len(result) < maxBlocks {
			raw, err := c.chain.GetBlockRawByNumber(num)
			if err != nil {
				if !c.chain.IsNotFound(err) {
					log.Error("failed to get block raw by number", "err", err)
				}
				break
			}
			result = append(result, rlp.RawValue(raw))
			num++
			size += metric.StorageSize(len(raw))
		}
		write(result)
	case proto.MsgGetTxs:
		var arg proto.GetTxs
		if err := msg.Decode(&arg); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
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
		write(result)
	default:
		return fmt.Errorf("unknown message (%v)", msg.Code)
	}
	return nil
}
