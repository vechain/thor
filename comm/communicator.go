package comm

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	set "gopkg.in/fatih/set.v0"
)

const (
	maxKnownTxs    = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 1024  // Maximum block hashes to keep in the known list (prevent DOS)
)

type Communicator struct {
	synced  bool              // Flag whether we're synchronised
	BlockCh chan *block.Block // 100 缓冲区

	ps          *p2psrv.Server
	knownBlocks map[discover.NodeID]*set.Set
	knownTxs    map[discover.NodeID]*set.Set
	ch          *chain.Chain
	txpl        *txpool.TxPool
}

func New() *Communicator {
	return &Communicator{
		synced: false,
	}
}

func (c *Communicator) IsSynced() bool {
	return c.synced == true
}

func (c *Communicator) Protocols() []*p2psrv.Protocol {
	return []*p2psrv.Protocol{
		&p2psrv.Protocol{
			Name:          proto.Name,
			Version:       proto.Version,
			Length:        proto.Length,
			MaxMsgSize:    proto.MaxMsgSize,
			HandleRequest: c.handleRequest,
		},
	}
}

func (c *Communicator) handleRequest(session *p2psrv.Session, msg *p2p.Msg) (resp interface{}, err error) {
	switch {
	case msg.Code == proto.MsgStatus:
		bestBlock, err := c.ch.GetBestBlock()
		if err != nil {
			return nil, err
		}
		header := bestBlock.Header()
		return &proto.RespStatus{
			TotalScore:  header.TotalScore(),
			BestBlockID: header.ID(),
		}, nil
	case msg.Code == proto.MsgNewTx:
		tx := &tx.Transaction{}
		if err := msg.Decode(tx); err != nil {
			return nil, err
		}
		c.markTransaction(session.Peer().ID(), tx.ID())
		c.txpl.Add(tx)
		return &struct{}{}, nil
	case msg.Code == proto.MsgNewBlock:

	case msg.Code == proto.MsgGetBlockIDByNumber:
		var num uint32
		if err := msg.Decode(&num); err != nil {
			return nil, err
		}
		id, err := c.ch.GetBlockIDByNumber(num)
		if err != nil {
			return nil, err
		}
		return &id, nil
	case msg.Code == proto.MsgNewBlockID:
		var id thor.Hash
		if err := msg.Decode(&id); err != nil {
			return nil, err
		}
		c.markBlock(session.Peer().ID(), id)
		if _, err := c.ch.GetBlock(id); err != nil {
			if c.ch.IsNotFound(err) {
				//pm.fetcher.Notify(p.id, block.Hash, block.Number, time.Now(), p.RequestOneHeader, p.RequestBodies)
			}
			return nil, err
		}
		return &struct{}{}, nil
	case msg.Code == proto.MsgGetBlocksByNumber:
		var num uint32
		if err := msg.Decode(&num); err != nil {
			return nil, err
		}

		bestBlk, err := c.ch.GetBestBlock()
		if err != nil {
			return nil, err
		}

		blks := make([]*block.Block, 0, 10)
		for i := 0; i < 10; i++ {
			num++
			if num > bestBlk.Header().Number() {
				break
			}

			blk, err := c.ch.GetBlockByNumber(num)
			if err != nil {
				return nil, err
			}

			blks[i] = blk
		}
		return blks, nil
	}
	return nil, nil
}

// 需要考虑线程同步的问题
func (c *Communicator) markTransaction(peer discover.NodeID, id thor.Hash) {
	for c.knownBlocks[peer].Size() >= maxKnownBlocks {
		c.knownBlocks[peer].Pop()
	}
	c.knownBlocks[peer].Add(id)
}

func (c *Communicator) markBlock(peer discover.NodeID, id thor.Hash) {
	for c.knownTxs[peer].Size() >= maxKnownTxs {
		c.knownTxs[peer].Pop()
	}
	c.knownTxs[peer].Add(id)
}

func (c *Communicator) Sync() {
	if err := c.sync(); err != nil {
		return
	}

	c.synced = true
}

// BroadcastTx 广播新插入池的交易
func (c *Communicator) BroadcastTx(tx *tx.Transaction) {
	cond := func(s *p2psrv.Session) bool {
		return !c.knownBlocks[s.Peer().ID()].Has(tx.ID())
	}

	ss := c.ps.Sessions().Filter(cond)
	_ = ss
}

// BroadcastBlk 广播新插入链的块
func (c *Communicator) BroadcastBlock(blk *block.Block) {
	cond := func(s *p2psrv.Session) bool {
		return !c.knownTxs[s.Peer().ID()].Has(blk.Header().ID())
	}

	ss := c.ps.Sessions().Filter(cond)
	_ = ss
}
