package comm

import (
	"context"
	"sync"
	"time"

	"github.com/bluele/gcache"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

const (
	maxKnownTxs    = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 1024  // Maximum block hashes to keep in the known list (prevent DOS)
)

type known struct {
	sync.Mutex
	m map[discover.NodeID]gcache.Cache
}

type unKnown struct {
	session *p2psrv.Session
	id      thor.Hash
}

type Communicator struct {
	blockCh        chan *block.Block
	synced         bool // Flag whether we're synchronised
	sessions       sessionSet
	unKnownBlockCh chan *unKnown
	knownBlocks    known
	knownTxs       known
	ch             *chain.Chain
	txpl           *txpool.TxPool
	cancel         context.CancelFunc
	ctx            context.Context
}

func New(ch *chain.Chain, txpl *txpool.TxPool) *Communicator {
	c := &Communicator{
		blockCh:        make(chan *block.Block),
		unKnownBlockCh: make(chan *unKnown),
		synced:         false,
		sessions:       sessionSet{m: make(map[discover.NodeID]*p2psrv.Session)},
		knownBlocks:    known{m: make(map[discover.NodeID]gcache.Cache)},
		knownTxs:       known{m: make(map[discover.NodeID]gcache.Cache)},
		ch:             ch,
		txpl:           txpl,
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

func (c *Communicator) Start(genesisBlock *block.Block, sessionCh chan *p2psrv.Session) Stop {
	var goes co.Goes

	goes.Go(func() {
		for {
			select {
			case session := <-sessionCh:
				peer := session.Peer()
				if !session.Alive() {
					c.sessions.remove(peer.ID())
					break
				}

				respSt, err := proto.ReqStatus{}.Do(c.ctx, session)
				if err != nil {
					peer.Disconnect(p2p.DiscNetworkError)
					break
				}

				if respSt.GenesisBlockID != genesisBlock.Header().ID() {
					peer.Disconnect(p2p.DiscUnexpectedIdentity)
					break
				}

				c.sessions.add(peer.ID(), session)
			case <-c.ctx.Done():
				return
			}
		}
	})

	goes.Go(func() {
		for {
			select {
			case unKnown := <-c.unKnownBlockCh:
				respBlk, err := proto.ReqGetBlockByID{ID: unKnown.id}.Do(c.ctx, unKnown.session)
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

func (c *Communicator) handleRequest(session *p2psrv.Session, msg *p2p.Msg) (resp interface{}, err error) {
	switch {
	case msg.Code == proto.MsgStatus:
		return handleStatus(c.ch)
	case msg.Code == proto.MsgNewTx:
		return handleNewTx(msg, c, session.Peer().ID())
	case msg.Code == proto.MsgNewBlock:
		return handleNewBlock(msg, c)
	case msg.Code == proto.MsgNewBlockID:
		return handleNewBlockID(msg, c, session)
	case msg.Code == proto.MsgGetBlockByID:
		return handleGetBlockByID(msg, c.ch)
	case msg.Code == proto.MsgGetBlockIDByNumber:
		return handleGetBlockIDByNumber(msg, c.ch)
	case msg.Code == proto.MsgGetBlocksByNumber:
		return handleGetBlocksByNumber(msg, c.ch)
	}
	return nil, nil
}

func (c *Communicator) markTransaction(peer discover.NodeID, id thor.Hash) {
	c.knownBlocks.Lock()
	defer c.knownBlocks.Unlock()

	if _, ok := c.knownBlocks.m[peer]; !ok {
		c.knownBlocks.m[peer] = gcache.New(maxKnownBlocks).Build()
	}
	c.knownBlocks.m[peer].Set(id, struct{}{})
}

func (c *Communicator) markBlock(peer discover.NodeID, id thor.Hash) {
	c.knownTxs.Lock()
	defer c.knownTxs.Unlock()

	if _, ok := c.knownTxs.m[peer]; !ok {
		c.knownTxs.m[peer] = gcache.New(maxKnownTxs).Build()
	}
	c.knownTxs.m[peer].Set(id, struct{}{})
}

func (c *Communicator) Sync() {
	if err := c.sync(); err != nil {
		return
	}

	c.synced = true
}

func (c *Communicator) BroadcastTx(tx *tx.Transaction) {
	txID := tx.ID()
	cond := func(s *p2psrv.Session) bool {
		c.knownBlocks.Lock()
		defer c.knownBlocks.Unlock()

		lru, ok := c.knownBlocks.m[s.Peer().ID()]
		if !ok {
			return true
		}

		_, err := lru.Get(txID)
		return err != nil
	}

	ss := c.sessions.getSessions().filter(cond)
	for _, s := range ss {
		go func(s *p2psrv.Session) {
			if err := (proto.ReqMsgNewTx{Tx: tx}.Do(c.ctx, s)); err == nil {
				c.markTransaction(s.Peer().ID(), txID)
			}
		}(s)
	}
}

func (c *Communicator) BroadcastBlock(blk *block.Block) {
	blkID := blk.Header().ID()
	cond := func(s *p2psrv.Session) bool {
		c.knownTxs.Lock()
		defer c.knownTxs.Unlock()

		lru, ok := c.knownTxs.m[s.Peer().ID()]
		if !ok {
			return true
		}

		_, err := lru.Get(blkID)
		return err != nil
	}

	ss := c.sessions.getSessions().filter(cond)
	for _, s := range ss {
		go func(s *p2psrv.Session) {
			if err := (proto.ReqNewBlockID{ID: blkID}.Do(c.ctx, s)); err == nil {
				c.markTransaction(s.Peer().ID(), blkID)
			}
		}(s)
	}
}
