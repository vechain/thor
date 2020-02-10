// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
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
	chain          *chain.Chain
	txPool         *txpool.TxPool
	ctx            context.Context
	cancel         context.CancelFunc
	peerSet        *PeerSet
	syncedCh       chan struct{}
	newBlockFeed   event.Feed
	announcementCh chan *announcement
	feedScope      event.SubscriptionScope
	goes           co.Goes
	onceSynced     sync.Once

	newBlockSummaryFeed event.Feed
	newEndorsementFeed  event.Feed
	newTxSetFeed        event.Feed
	newBlockHeaderFeed  event.Feed
}

// New create a new Communicator instance.
func New(chain *chain.Chain, txPool *txpool.TxPool) *Communicator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Communicator{
		chain:          chain,
		txPool:         txPool,
		ctx:            ctx,
		cancel:         cancel,
		peerSet:        newPeerSet(),
		syncedCh:       make(chan struct{}),
		announcementCh: make(chan *announcement),
	}
}

// Synced returns a channel indicates if synchronization process passed.
func (c *Communicator) Synced() <-chan struct{} {
	return c.syncedCh
}

// Sync start synchronization process.
func (c *Communicator) Sync(handler HandleBlockStream) {
	const initSyncInterval = 2 * time.Second
	const syncInterval = 30 * time.Second

	c.goes.Go(func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		delay := initSyncInterval
		syncCount := 0

		shouldSynced := func() bool {
			bestBlockTime := c.chain.BestBlock().Header().Timestamp()
			now := uint64(time.Now().Unix())
			if bestBlockTime+thor.BlockInterval >= now {
				return true
			}
			if syncCount > 2 {
				return true
			}
			return false
		}

		for {
			timer.Stop()
			timer = time.NewTimer(delay)
			select {
			case <-c.ctx.Done():
				return
			case <-timer.C:
				log.Debug("synchronization start")

				best := c.chain.BestBlock().Header()
				// choose peer which has the head block with higher total score
				peer := c.peerSet.Slice().Find(func(peer *Peer) bool {
					_, totalScore := peer.Head()
					return totalScore >= best.TotalScore()
				})
				if peer == nil {
					if c.peerSet.Len() < 3 {
						log.Debug("no suitable peer to sync")
						break
					}
					// if more than 3 peers connected, we are assumed to be the best
					log.Debug("synchronization done, best assumed")
				} else {
					if err := c.sync(peer, best.Number(), handler); err != nil {
						peer.logger.Debug("synchronization failed", "err", err)
						break
					}
					peer.logger.Debug("synchronization done")
				}
				syncCount++

				if shouldSynced() {
					delay = syncInterval
					c.onceSynced.Do(func() {
						close(c.syncedCh)
					})
				}
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
				Run:     c.servePeer,
			},
			DiscTopic: fmt.Sprintf("%v%v@%x", proto.Name, proto.Version, genesisID[24:]),
		}}
}

// Start start the communicator.
func (c *Communicator) Start() {
	c.goes.Go(c.txsLoop)
	c.goes.Go(c.announcementLoop)
}

// Stop stop the communicator.
func (c *Communicator) Stop() {
	c.cancel()
	c.feedScope.Close()
	c.goes.Wait()
}

type txsToSync struct {
	txs    tx.Transactions
	synced bool
}

func (c *Communicator) servePeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := newPeer(p, rw)
	c.goes.Go(func() {
		c.runPeer(peer)
	})

	var txsToSync txsToSync

	return peer.Serve(func(msg *p2p.Msg, w func(interface{})) error {
		return c.handleRPC(peer, msg, w, &txsToSync)
	}, proto.MaxMsgSize)
}

func (c *Communicator) runPeer(peer *Peer) {
	defer peer.Disconnect(p2p.DiscRequested)

	// 5sec timeout for handshake
	ctx, cancel := context.WithTimeout(c.ctx, time.Second*5)
	defer cancel()

	status, err := proto.GetStatus(ctx, peer)
	if err != nil {
		peer.logger.Debug("failed to get status", "err", err)
		return
	}
	if status.GenesisBlockID != c.chain.GenesisBlock().Header().ID() {
		peer.logger.Debug("failed to handshake", "err", "genesis id mismatch")
		return
	}
	localClock := uint64(time.Now().Unix())
	remoteClock := status.SysTimestamp

	diff := localClock - remoteClock
	if localClock < remoteClock {
		diff = remoteClock - localClock
	}
	if diff > thor.BlockInterval*2 {
		peer.logger.Debug("failed to handshake", "err", "sys time diff too large")
		return
	}

	peer.UpdateHead(status.BestBlockID, status.TotalScore)
	c.peerSet.Add(peer)
	peer.logger.Debug(fmt.Sprintf("peer added (%v)", c.peerSet.Len()))

	defer func() {
		c.peerSet.Remove(peer.ID())
		peer.logger.Debug(fmt.Sprintf("peer removed (%v)", c.peerSet.Len()))
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

// SubscribeBlockSummary subscribes the event of the arrival of a new block summary
func (c *Communicator) SubscribeBlockSummary(ch chan *NewBlockSummaryEvent) event.Subscription {
	return c.feedScope.Track(c.newBlockSummaryFeed.Subscribe(ch))
}

// SubscribeEndorsement subscribes the event of the arrival of a new endorsement
func (c *Communicator) SubscribeEndorsement(ch chan *NewEndorsementEvent) event.Subscription {
	return c.feedScope.Track(c.newEndorsementFeed.Subscribe(ch))
}

// SubscribeTxSet subscribes the event of the arrival of a new tx set
func (c *Communicator) SubscribeTxSet(ch chan *NewTxSetEvent) event.Subscription {
	return c.feedScope.Track(c.newTxSetFeed.Subscribe(ch))
}

// SubscribeBlockHeader ...
func (c *Communicator) SubscribeBlockHeader(ch chan *NewBlockHeaderEvent) event.Subscription {
	return c.feedScope.Track(c.newBlockHeaderFeed.Subscribe(ch))
}

// SubscribeBlock subscribe the event that new block received.
func (c *Communicator) SubscribeBlock(ch chan *NewBlockEvent) event.Subscription {
	return c.feedScope.Track(c.newBlockFeed.Subscribe(ch))
}

// BroadcastBlockSummary broadcasts a block summary to remote peers
func (c *Communicator) BroadcastBlockSummary(bs *block.Summary) {
	peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
		return !p.IsBlockSummaryKnown(bs.ID())
	})

	for _, peer := range peers {
		peer.MarkBlockSummary(bs.ID())
		c.goes.Go(func() {
			if err := proto.NotifyNewBlockSummary(c.ctx, peer, bs); err != nil {
				peer.logger.Debug("failed to broadcast new block summary", "err", err)
			}
		})
	}
}

// BroadcastTxSet broadcasts a tx set to remote peers
func (c *Communicator) BroadcastTxSet(ts *block.TxSet) {
	peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
		return !p.IsTxSetKnown(ts.ID())
	})

	for _, peer := range peers {
		peer.MarkTxSet(ts.ID())
		c.goes.Go(func() {
			if err := proto.NotifyNewTxSet(c.ctx, peer, ts); err != nil {
				peer.logger.Debug("failed to broadcast new tx set", "err", err)
			}
		})
	}
}

// BroadcastBlockHeader broadcasts a block header to remote peers
func (c *Communicator) BroadcastBlockHeader(header *block.Header) {
	peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
		return !p.IsBlockHeaderKnown(header.ID())
	})

	for _, peer := range peers {
		peer.MarkBlockHeader(header.ID())
		c.goes.Go(func() {
			if err := proto.NotifyNewBlockHeader(c.ctx, peer, header); err != nil {
				peer.logger.Debug("failed to broadcast new block header", "err", err)
			}
		})
	}
}

// BroadcastEndorsement broadcasts an endorsement to remote peers
func (c *Communicator) BroadcastEndorsement(ed *block.Endorsement) {
	peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
		return !p.IsEndorsementKnown(ed.ID())
	})

	for _, peer := range peers {
		peer := peer
		peer.MarkEndorsement(ed.ID())
		c.goes.Go(func() {
			if err := proto.NotifyNewEndorsement(c.ctx, peer, ed); err != nil {
				peer.logger.Debug("failed to broadcast new endorsement", "err", err)
			}
		})
	}
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
		log.Info("sending block", "peer", peer.ID())
		peer := peer
		peer.MarkBlock(blk.Header().ID())
		c.goes.Go(func() {
			if err := proto.NotifyNewBlock(c.ctx, peer, blk); err != nil {
				peer.logger.Debug("failed to broadcast new block", "err", err)
			}
		})
	}

	for _, peer := range toAnnounce {
		peer := peer
		peer.MarkBlock(blk.Header().ID())
		c.goes.Go(func() {

			if err := proto.NotifyNewBlockID(c.ctx, peer, blk.Header().ID()); err != nil {
				peer.logger.Debug("failed to broadcast new block id", "err", err)
			}
		})
	}
}

// BroadcastBlockID ...
func (c *Communicator) BroadcastBlockID(id thor.Bytes32) {
	peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
		return !p.IsBlockKnown(id)
	})

	for _, peer := range peers {
		peer := peer
		// peer.MarkBlock(id)
		c.goes.Go(func() {
			if err := proto.NotifyNewBlockID(c.ctx, peer, id); err != nil {
				peer.logger.Debug("failed to broadcast new block id", "err", err)
			}
		})
	}
}

// PeerCount returns count of peers.
func (c *Communicator) PeerCount() int {
	return c.peerSet.Len()
}

// PeersStats returns all peers' stats
func (c *Communicator) PeersStats() []*PeerStats {
	var stats []*PeerStats
	for _, peer := range c.peerSet.Slice() {
		bestID, totalScore := peer.Head()
		stats = append(stats, &PeerStats{
			Name:        peer.Name(),
			BestBlockID: bestID,
			TotalScore:  totalScore,
			PeerID:      peer.ID().String(),
			NetAddr:     peer.RemoteAddr().String(),
			Inbound:     peer.Inbound(),
			Duration:    uint64(time.Duration(peer.Duration()) / time.Second),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Duration < stats[j].Duration
	})
	return stats
}
