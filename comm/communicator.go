package comm

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var log = log15.New("pkg", "comm")

// Communicator communicates with remote p2p peers to exchange blocks and txs, etc.
type Communicator struct {
	chain           *chain.Chain
	txPool          *txpool.TxPool
	ctx             context.Context
	cancel          context.CancelFunc
	peerSet         *PeerSet
	syncedCh        chan struct{}
	newBlockFeed    event.Feed
	feedScope       event.SubscriptionScope
	goes            co.Goes
	onceForSyncedCh sync.Once
}

// New create a new Communicator instance.
func New(chain *chain.Chain, txPool *txpool.TxPool) *Communicator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Communicator{
		chain:    chain,
		txPool:   txPool,
		ctx:      ctx,
		cancel:   cancel,
		peerSet:  newPeerSet(),
		syncedCh: make(chan struct{}),
	}
}

// Synced returns a channel indicates if synchronization process passed.
func (c *Communicator) Synced() <-chan struct{} {
	return c.syncedCh
}

func (c *Communicator) setSynced() {
	c.onceForSyncedCh.Do(func() {
		close(c.syncedCh)
	})
}

// Sync start synchronization process.
func (c *Communicator) Sync(handler HandleBlockStream) {
	c.goes.Go(func() {
		timer := time.NewTimer(0)
		defer timer.Stop()

		delay := 2 * time.Second

		sync := func() {
			log.Debug("synchronization start")
			if err := c.sync(handler); err != nil {
				log.Debug("synchronization failed", "err", err)
			} else {
				delay = 30 * time.Second
				c.setSynced()
				log.Debug("synchronization done")
			}
		}

		for {
			timer.Stop()
			timer = time.NewTimer(delay)
			select {
			case <-timer.C:
				sync()
			case <-c.ctx.Done():
				return
			}
		}
	})
}

// Protocols returns all supported protocols.
func (c *Communicator) Protocols() []*p2psrv.Protocol {
	genesisID := c.chain.GenesisBlock().Header().ID()
	return []*p2psrv.Protocol{
		&p2psrv.Protocol{
			Protocol: p2p.Protocol{
				Name:    proto.Name,
				Version: proto.Version,
				Length:  proto.Length,
				Run:     c.runPeer,
			},
			DiscTopic: fmt.Sprintf("%v%v@%x", proto.Name, proto.Version, genesisID[24:]),
		}}
}

func (c *Communicator) runPeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := newPeer(p, rw)
	c.goes.Go(func() {
		c.peerLifeCycle(peer)
	})
	return peer.Serve(func(msg *p2p.Msg, w func(interface{})) error {
		return c.handleRPC(peer, msg, w)
	}, proto.MaxMsgSize)
}

func (c *Communicator) peerLifeCycle(peer *Peer) {
	defer peer.Disconnect(p2p.DiscRequested)

	// 5sec timeout for handshake
	ctx, cancel := context.WithTimeout(c.ctx, time.Second*5)
	defer cancel()

	result, err := proto.Status{}.Call(ctx, peer)
	if err != nil {
		peer.logger.Debug("failed to request status", "err", err)
		return
	}
	if result.GenesisBlockID != c.chain.GenesisBlock().Header().ID() {
		peer.logger.Debug("failed to handshake", "err", "genesis id mismatch")
		return
	}
	now := uint64(time.Now().Unix())
	diff := now - result.SysTimestamp
	if now < result.SysTimestamp {
		diff = result.SysTimestamp
	}
	if diff > thor.BlockInterval {
		peer.logger.Debug("failed to handshake", "err", "sys time diff too large")
		return
	}

	peer.UpdateHead(result.BestBlockID, result.TotalScore)
	c.peerSet.Add(peer)
	peer.logger.Debug("peer added")

	defer func() {
		c.peerSet.Remove(peer.ID())
		peer.logger.Debug("peer removed")
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

func (c *Communicator) syncTxs(peer *Peer) {
	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()
	result, err := proto.GetTxs{}.Call(ctx, peer)
	if err != nil {
		peer.logger.Debug("failed to request txs", "err", err)
		return
	}
	for _, tx := range result {
		c.txPool.Add(tx)

		select {
		case <-ctx.Done():
			return
		case <-peer.Done():
			return
		default:
		}
	}
	peer.logger.Debug("tx synced")
}

// SubscribeBlock subscribe the event that new block received.
func (c *Communicator) SubscribeBlock(ch chan *NewBlockEvent) event.Subscription {
	return c.feedScope.Track(c.newBlockFeed.Subscribe(ch))
}

// Start start the communicator.
func (c *Communicator) Start() {
	c.goes.Go(c.txLoop)
}

// Stop stop the communicator.
func (c *Communicator) Stop() {
	c.cancel()
	c.feedScope.Close()
	c.goes.Wait()
}

func (c *Communicator) txLoop() {
	txCh := make(chan *tx.Transaction)
	sub := c.txPool.SubscribeNewTransaction(txCh)
	defer sub.Unsubscribe()

	for {
		select {
		case <-c.ctx.Done():
			return
		case tx := <-txCh:
			peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
				return !p.IsTransactionKnown(tx.ID())
			})

			for _, peer := range peers {
				peer.MarkTransaction(tx.ID())
				c.goes.Go(func() {
					if err := (proto.NewTx{Tx: tx}.Call(c.ctx, peer)); err != nil {
						peer.logger.Debug("failed to broadcast tx", "err", err)
					}
				})
			}
		}
	}
}

func (c *Communicator) handleAnnounce(blockID thor.Bytes32, src *Peer) {
	if _, err := c.chain.GetBlockHeader(blockID); err != nil {
		if !c.chain.IsNotFound(err) {
			log.Error("failed to get block header", "err", err)
		}
	} else {
		// already in chain
		return
	}

	target := c.peerSet.Slice().Find(func(p *Peer) bool {
		id, _ := p.Head()
		return bytes.Compare(id[:], blockID[:]) >= 0
	})
	if target == nil {
		target = src
	}

	result, err := proto.GetBlockByID{ID: blockID}.Call(c.ctx, target)
	if err != nil {
		target.logger.Debug("failed to get block by id", "err", err)
		return
	}

	if result.Block == nil {
		target.logger.Debug("get nil block by id")
		return
	}
	c.newBlockFeed.Send(&NewBlockEvent{
		Block: result.Block,
	})
}

// BroadcastBlock broadcast a block to remote peers.
func (c *Communicator) BroadcastBlock(blk *block.Block) {
	peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
		return !p.IsBlockKnown(blk.Header().ID())
	})

	p := int(math.Sqrt(float64(len(peers))))
	toPropagate := peers[:p]
	toAnnounce := peers[p:]

	for _, peer := range toPropagate {
		peer.MarkBlock(blk.Header().ID())
		c.goes.Go(func() {
			if err := (proto.NewBlock{Block: blk}.Call(c.ctx, peer)); err != nil {
				peer.logger.Debug("failed to broadcast new block", "err", err)
			}
		})
	}

	for _, peer := range toAnnounce {
		peer.MarkBlock(blk.Header().ID())
		c.goes.Go(func() {
			if err := (proto.NewBlockID{ID: blk.Header().ID()}.Call(c.ctx, peer)); err != nil {
				peer.logger.Debug("failed to broadcast new block id", "err", err)
			}
		})
	}
}

// PeersStats returns all peers' stats
func (c *Communicator) PeersStats() []*PeerStats {
	var stats []*PeerStats
	for _, peer := range c.peerSet.Slice() {
		bestID, totalScore := peer.Head()
		stats = append(stats, &PeerStats{
			BestBlockID: bestID,
			TotalScore:  totalScore,
			PeerID:      peer.ID().String(),
			NetAddr:     peer.RemoteAddr().String(),
			Inbound:     peer.Inbound(),
			Duration:    uint64(time.Duration(peer.Duration()) / time.Second),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].PeerID < stats[j].PeerID
	})
	return stats
}
