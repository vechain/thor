package comm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
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

var log = log15.New("pkg", "comm")

// Communicator communicates with remote p2p peers to exchange blocks and txs, etc.
type Communicator struct {
	chain        *chain.Chain
	ctx          context.Context
	cancel       context.CancelFunc
	sessionSet   *session.Set
	syncedCh     chan struct{}
	syncCh       chan struct{}
	announceCh   chan *announce
	newBlockFeed event.Feed
	newTxFeed    event.Feed
	feedScope    event.SubscriptionScope
	goes         co.Goes
	txPool       *txpool.TxPool
}

// New create a new Communicator instance.
func New(chain *chain.Chain, txPool *txpool.TxPool) *Communicator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Communicator{
		chain:      chain,
		txPool:     txPool,
		ctx:        ctx,
		cancel:     cancel,
		sessionSet: session.NewSet(),
		syncedCh:   make(chan struct{}),
		syncCh:     make(chan struct{}),
		announceCh: make(chan *announce),
	}
}

// Synced returns a channel indicates if synchronization process passed.
func (c *Communicator) Synced() <-chan struct{} {
	return c.syncedCh
}

// Protocols returns all supported protocols.
func (c *Communicator) Protocols() []*p2psrv.Protocol {
	genesisID := c.chain.GenesisBlock().Header().ID()
	return []*p2psrv.Protocol{
		&p2psrv.Protocol{
			Name:       proto.Name,
			Version:    proto.Version,
			Length:     proto.Length,
			MaxMsgSize: proto.MaxMsgSize,
			DiscTopic:  fmt.Sprintf("%v%v@%x", proto.Name, proto.Version, genesisID[24:]),
			HandlePeer: c.handlePeer,
		}}
}

func (c *Communicator) handlePeer(peer *p2psrv.Peer) p2psrv.HandleRequest {
	// controls session lifecycle
	c.goes.Go(func() { c.sessionLoop(peer) })
	return c.handleRequest
}

func (c *Communicator) sessionLoop(peer *p2psrv.Peer) {
	defer peer.Disconnect()

	log := log.New("peer", peer)
	// 5sec timeout for handshake
	ctx, cancel := context.WithTimeout(c.ctx, time.Second*5)
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
	now := uint64(time.Now().Unix())
	diff := now - respStatus.SysTimestamp
	if now < respStatus.SysTimestamp {
		diff = respStatus.SysTimestamp
	}
	if diff > thor.BlockInterval {
		log.Debug("failed to handshake", "err", "sys time diff too large")
		return
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
	case <-c.syncedCh:
		c.syncTxs(peer)
		select {
		case <-peer.Done():
		case <-c.ctx.Done():
		}
	}
}

func (c *Communicator) syncTxs(peer *p2psrv.Peer) {
	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()
	respTxs, err := proto.ReqGetTxs{}.Do(ctx, peer)
	if err != nil {
		log.Debug("failed to request txs", "err", err)
		return
	}
	for _, tx := range respTxs {
		c.txPool.Add(tx)

		select {
		case <-ctx.Done():
			return
		case <-peer.Done():
			return
		default:
		}
	}
	log.Debug("tx synced")
}

// SubscribeBlock subscribe the event that new block received.
func (c *Communicator) SubscribeBlock(ch chan *NewBlockEvent) event.Subscription {
	return c.feedScope.Track(c.newBlockFeed.Subscribe(ch))
}

// SubscribeTransaction subscribe the event that new tx received.
func (c *Communicator) SubscribeTransaction(ch chan *NewTransactionEvent) event.Subscription {
	return c.feedScope.Track(c.newTxFeed.Subscribe(ch))
}

// Start start the communicator.
func (c *Communicator) Start(handler HandleBlockStream) {
	c.goes.Go(func() { c.syncLoop(handler) })
	c.goes.Go(c.announceLoop)
}

// Stop stop the communicator.
func (c *Communicator) Stop() {
	c.cancel()
	c.feedScope.Close()
	c.goes.Wait()
}

func (c *Communicator) syncLoop(handler HandleBlockStream) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	var once sync.Once
	delay := 2 * time.Second

	sync := func() {
		log.Debug("synchronization start")
		if err := c.sync(handler); err != nil {
			log.Debug("synchronization failed", "err", err)
		} else {
			once.Do(func() {
				close(c.syncedCh)
				delay = 30 * time.Second
			})
			log.Debug("synchronization done")
		}
	}

	for {
		timer.Stop()
		timer = time.NewTimer(delay)
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

		c.newBlockFeed.Send(&NewBlockEvent{
			Block: resp.Block,
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

// SessionsStats returns all sessions' stats
func (c *Communicator) SessionsStats() []*SessionStats {
	var stats []*SessionStats
	now := mclock.Now()
	for _, s := range c.sessionSet.Slice() {
		bestID, totalScore := s.TrunkHead()
		stats = append(stats, &SessionStats{
			BestBlockID: bestID,
			TotalScore:  totalScore,
			PeerID:      s.Peer().ID().String(),
			NetAddr:     s.Peer().RemoteAddr().String(),
			Inbound:     s.Peer().Inbound(),
			Duration:    uint64(time.Duration(now-s.CreatedTime()) / time.Second),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].PeerID < stats[j].PeerID
	})
	return stats
}
