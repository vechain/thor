package comm

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
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

// type unKnown struct {
// 	session *session.Session
// 	id      thor.Hash
// }

type Communicator struct {
	genesisID  thor.Hash
	chain      *chain.Chain
	txPool     *txpool.TxPool
	synced     bool
	ctx        context.Context
	cancel     context.CancelFunc
	sessionSet *session.Set
	syncCh     chan struct{}
	blockFeed  event.Feed
	txFeed     event.Feed
	feedScope  event.SubscriptionScope
	goes       co.Goes
}

func mustGetGenesisID(chain *chain.Chain) thor.Hash {
	id, err := chain.GetBlockIDByNumber(0)
	if err != nil {
		panic(err)
	}
	return id
}

func New(chain *chain.Chain, txPool *txpool.TxPool) *Communicator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Communicator{
		genesisID:  mustGetGenesisID(chain),
		chain:      chain,
		txPool:     txPool,
		ctx:        ctx,
		cancel:     cancel,
		sessionSet: session.NewSet(),
		syncCh:     make(chan struct{}),
	}
}

func (c *Communicator) IsSynced() bool {
	return c.synced
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

func (c *Communicator) SubscribeBlock(ch chan *block.Block) event.Subscription {
	return c.feedScope.Track(c.blockFeed.Subscribe(ch))
}

func (c *Communicator) SubscribeTx(ch chan *tx.Transaction) event.Subscription {
	return c.feedScope.Track(c.txFeed.Subscribe(ch))
}

func (c *Communicator) Start(peerCh chan *p2psrv.Peer) {
	c.goes.Go(func() { c.sessionLoop(peerCh) })
	c.goes.Go(c.syncLoop)
	// c.goes.Go(func() {
	// 	for {
	// 		select {
	// 		case unKnown := <-c.unKnownBlockCh:
	// 			respBlk, err := proto.ReqGetBlockByID{ID: unKnown.id}.Do(c.ctx, unKnown.session.Peer())
	// 			if err == nil {
	// 				go func() {
	// 					select {
	// 					case c.blockCh <- respBlk.Block:
	// 					case <-c.ctx.Done():
	// 						return
	// 					}
	// 				}()
	// 			}
	// 		case <-c.ctx.Done():
	// 			return
	// 		}
	// 	}
	// })
}

func (c *Communicator) Stop() {
	c.cancel()
	c.feedScope.Close()
	c.goes.Wait()
}

func (c *Communicator) sessionLoop(peerCh chan *p2psrv.Peer) {
	// controls session lifecycle
	lifecycle := func(peer *p2psrv.Peer) {
		defer peer.Disconnect()

		// 5sec timeout for handshake
		ctx, cancel := context.WithTimeout(c.ctx, time.Second*5)
		defer cancel()
		resp, err := proto.ReqStatus{}.Do(ctx, peer)
		if err != nil {
			return
		}
		if resp.GenesisBlockID != c.genesisID {
			return
		}

		session := session.New(peer)
		session.UpdateTrunkHead(resp.BestBlockID, resp.TotalScore)

		c.sessionSet.Add(session)
		defer c.sessionSet.Remove(peer.ID())

		select {
		case <-peer.Done():
		case <-c.ctx.Done():
		}
	}

	for {
		select {
		case peer := <-peerCh:
			c.goes.Go(func() { lifecycle(peer) })
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Communicator) syncLoop() {
	wait := 10 * time.Second

	timer := time.NewTimer(wait)
	defer timer.Stop()

	sync := func() {
		log.Trace("synchronization start")
		if err := c.sync(); err != nil {
			log.Trace("synchronization failed", "err", err)
		} else {
			c.synced = true
			log.Trace("synchronization done")
		}
	}

	for {
		timer.Reset(wait)
		select {
		case <-timer.C:
			sync()
		case <-c.syncCh:
			sync()
		case <-c.ctx.Done():
			return
		}
	}
}

// RequestSync request sync operation.
func (c *Communicator) RequestSync() bool {
	select {
	case c.syncCh <- struct{}{}:
		return true
	default:
		return false
	}
}

func (c *Communicator) BroadcastTx(tx *tx.Transaction) {
	slice := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
		return !s.IsTransactionKnown(tx.ID())
	})

	for _, s := range slice {
		s.MarkTransaction(tx.ID())
		peer := s.Peer()
		c.goes.Go(func() {
			req := proto.ReqMsgNewTx{Tx: tx}
			if err := req.Do(c.ctx, peer); err != nil {
			}
		})
	}
}

func (c *Communicator) BroadcastBlock(blk *block.Block) {
	slice := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
		return !s.IsBlockKnown(blk.Header().ID())
	})

	for _, s := range slice {
		s.MarkBlock(blk.Header().ID())
		peer := s.Peer()
		c.goes.Go(func() {
			req := proto.ReqNewBlock{Block: blk}
			if err := req.Do(c.ctx, peer); err != nil {
			}
		})
	}
}
