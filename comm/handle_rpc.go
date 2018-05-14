package comm

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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
	case proto.MsgGetStatus:
		if err := msg.Decode(&struct{}{}); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		best := c.chain.BestBlock().Header()
		write(&proto.Status{
			GenesisBlockID: c.chain.GenesisBlock().Header().ID(),
			SysTimestamp:   uint64(time.Now().Unix()),
			TotalScore:     best.TotalScore(),
			BestBlockID:    best.ID(),
		})
	case proto.MsgNewBlock:
		var newBlock *block.Block
		if err := msg.Decode(&newBlock); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		peer.MarkBlock(newBlock.Header().ID())
		peer.UpdateHead(newBlock.Header().ID(), newBlock.Header().TotalScore())
		c.newBlockFeed.Send(&NewBlockEvent{Block: newBlock})
		write(&struct{}{})
	case proto.MsgNewBlockID:
		var newBlockID thor.Bytes32
		if err := msg.Decode(&newBlockID); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		peer.MarkBlock(newBlockID)
		select {
		case <-c.ctx.Done():
		case c.announcementCh <- &announcement{newBlockID, peer}:
		}
		write(&struct{}{})
	case proto.MsgNewTx:
		var newTx *tx.Transaction
		if err := msg.Decode(&newTx); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		peer.MarkTransaction(newTx.ID())
		c.txPool.Add(newTx)
		write(&struct{}{})
	case proto.MsgGetBlockByID:
		var blockID thor.Bytes32
		if err := msg.Decode(&blockID); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		blk, err := c.chain.GetBlock(blockID)
		if err != nil {
			if !c.chain.IsNotFound(err) {
				log.Error("failed to get block", "err", err)
			}
			write(&struct{}{})
		} else {
			write(blk)
		}
	case proto.MsgGetBlockIDByNumber:
		var num uint32
		if err := msg.Decode(&num); err != nil {
			return errors.WithMessage(err, "decode msg")
		}
		id, err := c.chain.GetBlockIDByNumber(num)
		if err != nil {
			if !c.chain.IsNotFound(err) {
				log.Error("failed to get block id by number", "err", err)
			}
			write(thor.Bytes32{})
		} else {
			write(id)
		}
	case proto.MsgGetBlocksFromNumber:
		var num uint32
		if err := msg.Decode(&num); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		const maxBlocks = 1024
		result := make([]rlp.RawValue, 0, maxBlocks)
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
		if err := msg.Decode(&struct{}{}); err != nil {
			return errors.WithMessage(err, "decode msg")
		}

		txs := c.txPool.Pending(false)
		result := make([]*tx.Transaction, 0, len(txs))
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
