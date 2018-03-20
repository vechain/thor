package comm

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/comm/session"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type unKnown struct {
	session *session.Session
	id      thor.Hash
}

type Communicator struct {
	blockCh        chan *block.Block
	synced         bool // Flag whether we're synchronised
	unKnownBlockCh chan *unKnown
	ch             *chain.Chain
	txpl           *txpool.TxPool
	cancel         context.CancelFunc
	ctx            context.Context
	sessionSet     *session.Set
}

func New(ch *chain.Chain, txpl *txpool.TxPool) *Communicator {
	c := &Communicator{
		blockCh:        make(chan *block.Block),
		unKnownBlockCh: make(chan *unKnown),
		synced:         false,
		ch:             ch,
		txpl:           txpl,
		sessionSet:     session.NewSet(),
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	return c
}

func (c *Communicator) BlockCh() chan *block.Block {
	return c.blockCh
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
		}}
}

type Stop func()

func (c *Communicator) Start(genesisBlock *block.Block, peerCh chan *p2psrv.Peer) Stop {
	var goes co.Goes

	goes.Go(func() {
		for {
			select {
			case peer := <-peerCh:
				if !peer.Alive() {
					c.sessionSet.Remove(peer.ID())
					break
				}

				respSt, err := proto.ReqStatus{}.Do(c.ctx, peer)
				if err != nil {
					peer.Disconnect(true)
					break
				}

				if respSt.GenesisBlockID != genesisBlock.Header().ID() {
					peer.Disconnect(true)
					break
				}

				c.sessionSet.Add(session.New(peer))
			case <-c.ctx.Done():
				return
			}
		}
	})

	goes.Go(func() {
		for {
			select {
			case unKnown := <-c.unKnownBlockCh:
				respBlk, err := proto.ReqGetBlockByID{ID: unKnown.id}.Do(c.ctx, unKnown.session.Peer())
				if err == nil {
					go func() {
						select {
						case c.blockCh <- respBlk.Block:
						case <-c.ctx.Done():
							return
						}
					}()
				}
			case <-c.ctx.Done():
				return
			}
		}
	})

	goes.Go(func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				c.Sync()
			case <-c.ctx.Done():
				ticker.Stop()
				return
			}
		}
	})

	return func() {
		c.cancel()
		goes.Wait()
	}
}

func (c *Communicator) handleRequest(peer *p2psrv.Peer, msg *p2p.Msg) (resp interface{}, err error) {
	switch msg.Code {
	case proto.MsgStatus:
		return handleStatus(c.ch)
	case proto.MsgNewTx:
		return handleNewTx(msg, c, peer.ID())
	case proto.MsgNewBlock:
		return handleNewBlock(msg, c)
	case proto.MsgNewBlockID:
		return handleNewBlockID(msg, c, peer)
	case proto.MsgGetBlockByID:
		return handleGetBlockByID(msg, c.ch)
	case proto.MsgGetBlockIDByNumber:
		return handleGetBlockIDByNumber(msg, c.ch)
	case proto.MsgGetBlocksByNumber:
		return handleGetBlocksByNumber(msg, c.ch)
	}
	return nil, nil
}

func (c *Communicator) Sync() {
	if err := c.sync(); err != nil {
		return
	}

	c.synced = true
}

func (c *Communicator) BroadcastTx(tx *tx.Transaction) {
	txID := tx.ID()

	slice := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
		return !s.IsBlockKnown(txID)
	})
	for _, s := range slice {
		go func(s *session.Session) {
			if err := (proto.ReqMsgNewTx{Tx: tx}.Do(c.ctx, s.Peer())); err == nil {
				s.MarkTransaction(txID)
			}
		}(s)
	}
}

func (c *Communicator) BroadcastBlock(blk *block.Block) {
	blkID := blk.Header().ID()
	slice := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
		return !s.IsBlockKnown(blkID)
	})
	for _, s := range slice {
		go func(s *session.Session) {
			if err := (proto.ReqNewBlockID{ID: blkID}.Do(c.ctx, s.Peer())); err == nil {
				s.MarkBlock(blkID)
			}
		}(s)
	}
}
