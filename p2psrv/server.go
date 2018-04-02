package p2psrv

import (
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/co"
)

var log = log15.New("pkg", "p2psrv")

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	srv  *p2p.Server
	goes co.Goes
	done chan struct{}

	goodNodes       *cache.PrioCache
	discoveredNodes *cache.RandCache
	busyNodes       *nodeMap
	dialCh          chan *discover.Node

	peerFeed  event.Feed
	feedScope event.SubscriptionScope
}

// New create a p2p server.
func New(opts *Options) *Server {

	v5nodes := make([]*discv5.Node, 0, len(opts.BootstrapNodes))
	for _, n := range opts.BootstrapNodes {
		v5nodes = append(v5nodes, discv5.NewNode(discv5.NodeID(n.ID), n.IP, n.UDP, n.TCP))
	}

	goodNodes := cache.NewPrioCache(16)
	discoveredNodes := cache.NewRandCache(128)
	for _, node := range opts.GoodNodes {
		goodNodes.Set(node.ID, node, 0)
		discoveredNodes.Set(node.ID, node)
	}

	return &Server{
		srv: &p2p.Server{
			Config: p2p.Config{
				Name:             opts.Name,
				PrivateKey:       opts.PrivateKey,
				MaxPeers:         opts.MaxPeers,
				NoDiscovery:      true,
				DiscoveryV5:      !opts.NoDiscovery,
				ListenAddr:       opts.ListenAddr,
				BootstrapNodesV5: v5nodes,
				NetRestrict:      opts.NetRestrict,
				NAT:              opts.NAT,
				NoDial:           opts.NoDial,
			},
		},
		done:            make(chan struct{}),
		goodNodes:       goodNodes,
		discoveredNodes: discoveredNodes,
		busyNodes:       newNodeMap(),
		dialCh:          make(chan *discover.Node),
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

// SubscribePeer subscribe new peer event.
func (s *Server) SubscribePeer(ch chan *Peer) event.Subscription {
	return s.feedScope.Track(s.peerFeed.Subscribe(ch))
}

func (s *Server) runProtocol(proto *Protocol) func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	return func(peer *p2p.Peer, rw p2p.MsgReadWriter) (err error) {
		log := log.New("peer", peer)
		log.Debug("peer connected")
		p := newPeer(peer, proto)
		s.goes.Go(func() { s.peerFeed.Send(p) })

		defer func() {
			if node := s.busyNodes.Remove(peer.ID()); node != nil {
				s.goodNodes.Set(peer.ID(), node, p.stats.weight())
			}
			log.Debug("peer disconnected", "reason", err)
		}()

		return p.serve(rw, proto.HandleRequest)
	}
}

// Start start the server.
func (s *Server) Start(discoTopic string, protocols []*Protocol) error {
	for _, proto := range protocols {
		s.srv.Protocols = append(s.srv.Protocols, p2p.Protocol{
			Name:    proto.Name,
			Version: uint(proto.Version),
			Length:  proto.Length,
			//			NodeInfo: p.NodeInfo,
			//			PeerInfo: p.PeerInfo,
			Run: s.runProtocol(proto),
		})
	}
	if err := s.srv.Start(); err != nil {
		return err
	}
	s.goes.Go(func() { s.discoverLoop(discv5.Topic(discoTopic)) })
	s.goes.Go(s.dialLoop)
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	s.srv.Stop()
	close(s.done)
	s.feedScope.Close()
	s.goes.Wait()
}

// GoodNodes returns good nodes.
func (s *Server) GoodNodes() Nodes {
	gns := make([]*discover.Node, 0, s.goodNodes.Len())
	s.goodNodes.ForEach(func(ent *cache.PrioEntry) bool {
		gns = append(gns, ent.Value.(*discover.Node))
		return true
	})
	return gns
}

// AddStatic connects to the given node and maintains the connection until the
// server is shut down. If the connection fails for any reason, the server will
// attempt to reconnect the peer.
func (s *Server) AddStatic(node *discover.Node) {
	s.srv.AddPeer(node)
}

// RemoveStatic disconnects from the given node
func (s *Server) RemoveStatic(node *discover.Node) {
	s.srv.RemovePeer(node)
}

// NodeInfo gathers and returns a collection of metadata known about the host.
func (s *Server) NodeInfo() *p2p.NodeInfo {
	return s.srv.NodeInfo()
}

func (s *Server) discoverLoop(topic discv5.Topic) {
	if s.srv.DiscV5 == nil {
		return
	}

	setPeriod := make(chan time.Duration, 1)
	setPeriod <- time.Millisecond * 100
	discNodes := make(chan *discv5.Node, 100)
	discLookups := make(chan bool, 100)

	s.goes.Go(func() {
		s.srv.DiscV5.RegisterTopic(topic, s.done)
	})

	s.goes.Go(func() {
		s.srv.DiscV5.SearchTopic(topic, setPeriod, discNodes, discLookups)
	})

	var (
		lookupCount  = 0
		fastDiscover = true
		convTime     mclock.AbsTime
	)
	// see go-ethereum serverpool.go
	for {
		select {
		case conv := <-discLookups:
			if conv {
				if lookupCount == 0 {
					convTime = mclock.Now()
				}
				lookupCount++
				if fastDiscover && (lookupCount == 50 || time.Duration(mclock.Now()-convTime) > time.Minute) {
					fastDiscover = false
					setPeriod <- time.Minute
				}
			}
		case v5node := <-discNodes:
			node := discover.NewNode(discover.NodeID(v5node.ID), v5node.IP, v5node.UDP, v5node.TCP)
			if _, found := s.discoveredNodes.Get(node.ID); !found {
				s.discoveredNodes.Set(node.ID, node)
				log.Debug("discovered node", "node", node)
			}
		case <-s.done:
			close(setPeriod)
			return
		}
	}
}

func (s *Server) dialLoop() {
	const nonFastDialDur = 10 * time.Second

	// fast dialing initially
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	dialCount := 0
	for {
		select {
		case <-ticker.C:
			entry := s.discoveredNodes.Pick()
			if entry == nil {
				continue
			}
			node := entry.Value.(*discover.Node)
			if s.srv.PeerCount() >= s.srv.MaxPeers {
				continue
			}
			if s.busyNodes.Contains(node.ID) {
				continue
			}

			log := log.New("node", node)
			log.Debug("try to dial node")
			s.busyNodes.Add(node)
			s.goes.Go(func() {
				if err := s.tryDial(node); err != nil {
					s.busyNodes.Remove(node.ID)
					log.Debug("failed to dial node", "err", err)
				}
			})
			dialCount++
			if dialCount == 20 {
				ticker = time.NewTicker(nonFastDialDur)
			}
		case <-s.done:
			return
		}
	}
}

func (s *Server) tryDial(node *discover.Node) error {
	conn, err := s.srv.Dialer.Dial(node)
	if err != nil {
		return err
	}
	return s.srv.SetupConn(conn, 1, node)
}
