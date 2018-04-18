package comm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/comm/session"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var log = log15.New("pkg", "comm")

type NewBlockEvent struct {
	Block    *block.Block
	IsSynced bool
}

// Communicator communicates with remote p2p peers to exchange blocks and txs, etc.
type Communicator struct {
	chain      *chain.Chain
	synced     bool
	ctx        context.Context
	cancel     context.CancelFunc
	sessionSet *session.Set
	syncCh     chan struct{}
	announceCh chan *announce
	blockFeed  event.Feed
	txFeed     event.Feed
	feedScope  event.SubscriptionScope
	goes       co.Goes
	txpool     *txpool.TxPool
	syncReport func(int, metric.StorageSize, time.Duration)
}

// New create a new Communicator instance.
func New(chain *chain.Chain, txpool *txpool.TxPool) *Communicator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Communicator{
		chain:      chain,
		txpool:     txpool,
		ctx:        ctx,
		cancel:     cancel,
		sessionSet: session.NewSet(),
		syncCh:     make(chan struct{}),
		announceCh: make(chan *announce),
	}
}

// IsSynced returns if the synchronization process ever passed.
func (c *Communicator) IsSynced() bool {
	return c.synced
}

// Protocols returns all supported protocols.
func (c *Communicator) Protocols() []*p2psrv.Protocol {
	genesisID := c.chain.GenesisBlock().Header().ID()
	return []*p2psrv.Protocol{
		&p2psrv.Protocol{
			Name:          proto.Name,
			Version:       proto.Version,
			Length:        proto.Length,
			MaxMsgSize:    proto.MaxMsgSize,
			DiscTopic:     fmt.Sprintf("%v%v@%x", proto.Name, proto.Version, genesisID[24:]),
			HandleRequest: c.handleRequest,
		}}
}

// SubscribeBlock subscribe the event that new blocks received.
func (c *Communicator) SubscribeBlock(ch chan *NewBlockEvent) event.Subscription {
	return c.feedScope.Track(c.blockFeed.Subscribe(ch))
}

// SubscribeTx subscribe the event that new tx received.
func (c *Communicator) SubscribeTx(ch chan *tx.Transaction) event.Subscription {
	return c.feedScope.Track(c.txFeed.Subscribe(ch))
}

// Start start the communicator.
func (c *Communicator) Start(peerCh chan *p2psrv.Peer, syncReport func(int, metric.StorageSize, time.Duration)) {
	c.syncReport = syncReport
	c.goes.Go(func() { c.sessionLoop(peerCh) })
	c.goes.Go(c.syncLoop)
	c.goes.Go(c.announceLoop)
}

// Stop stop the communicator.
func (c *Communicator) Stop() {
	c.cancel()
	c.feedScope.Close()
	c.goes.Wait()
}

func (c *Communicator) sessionLoop(peerCh chan *p2psrv.Peer) {
	// controls session lifecycle
	lifecycle := func(peer *p2psrv.Peer) {
		defer peer.Disconnect()

		log := log.New("peer", peer)
		// 20sec timeout for handshake and txs transfer
		ctx, cancel := context.WithTimeout(c.ctx, time.Second*20)
		defer cancel()
		respStatus, err := proto.ReqStatus{}.Do(ctx, peer)
		if err != nil {
			log.Debug("failed to request status", "err", err)
			return
		}
		if respStatus.GenesisBlockID != c.chain.GenesisBlock().Header().ID() {
			log.Debug("failed to handshake", "err", "genesis id mismatch")
			return
		}

		respTxs, err := proto.ReqGetTxs{}.Do(ctx, peer)
		if err != nil {
			log.Debug("failed to request txs", "err", err)
			return
		}
		for _, raw := range respTxs {
			c.txpool.Add(raw)
		}

		session := session.New(peer)
		session.UpdateTrunkHead(respStatus.BestBlockID, respStatus.TotalScore)

		log.Debug("session created")
		c.sessionSet.Add(session)
		defer func() {
			c.sessionSet.Remove(peer.ID())
			log.Debug("session destroyed")
		}()

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
		log.Debug("synchronization start")
		if err := c.sync(); err != nil {
			log.Debug("synchronization failed", "err", err)
		} else {
			c.synced = true
			log.Debug("synchronization done")
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

func (c *Communicator) announceLoop() {
	fetch := func(blockID thor.Bytes32, peer *p2psrv.Peer) error {
		if _, err := c.chain.GetBlockHeader(blockID); err != nil {
			if !c.chain.IsNotFound(err) {
				return err
			}
		} else {
			// already in chain
			return nil
		}

		if !isPeerAlive(peer) {
			slice := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
				id, _ := s.TrunkHead()
				return bytes.Compare(id[:], blockID[:]) >= 0
			})
			if len(slice) > 0 {
				peer = slice[0].Peer()
			}
		}
		if peer == nil {
			return errors.New("no peer to fetch block by id")
		}

		req := proto.ReqGetBlockByID{ID: blockID}
		resp, err := req.Do(c.ctx, peer)
		if err != nil {
			return err
		}
		if resp.Block == nil {
			return errors.New("nil block")
		}

		c.blockFeed.Send(&NewBlockEvent{
			Block:    resp.Block,
			IsSynced: false,
		})
		return nil
	}

	for {
		select {
		case ann := <-c.announceCh:
			if err := fetch(ann.blockID, ann.peer); err != nil {
				log.Debug("failed to fetch block by ID", "peer", ann.peer, "err", err)
			}
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

// BroadcastTx broadcast new tx to remote peers.
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
				log.Debug("failed to broadcast tx", "peer", peer, "err", err)
			}
		})
	}
}

// BroadcastBlock broadcast a block to remote peers.
func (c *Communicator) BroadcastBlock(blk *block.Block) {
	slice := c.sessionSet.Slice().Filter(func(s *session.Session) bool {
		return !s.IsBlockKnown(blk.Header().ID())
	})

	p := int(math.Sqrt(float64(len(slice))))
	toPropagate := slice[:p]
	toAnnounce := slice[p:]

	for _, s := range toPropagate {
		s.MarkBlock(blk.Header().ID())
		peer := s.Peer()
		c.goes.Go(func() {
			req := proto.ReqNewBlock{Block: blk}
			if err := req.Do(c.ctx, peer); err != nil {
				log.Debug("failed to broadcast new block", "peer", peer, "err", err)
			}
		})
	}

	for _, s := range toAnnounce {
		s.MarkBlock(blk.Header().ID())
		peer := s.Peer()
		c.goes.Go(func() {
			req := proto.ReqNewBlockID{ID: blk.Header().ID()}
			if err := req.Do(c.ctx, peer); err != nil {
				log.Debug("failed to broadcast new block id", "peer", peer, "err", err)
			}
		})
	}
}

// SessionCount returns count of sessions.
func (c *Communicator) SessionCount() int {
	return c.sessionSet.Len()
}
