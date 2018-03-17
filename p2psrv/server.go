package p2psrv

import (
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/w8cache"
)

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	srv  *p2p.Server
	goes co.Goes
	done chan struct{}

	goodNodes       *w8cache.W8Cache
	discoveredNodes *w8cache.W8Cache
	busyNodes       busyNodes
	dialCh          chan *discover.Node

	sessionFeed event.Feed
	feedScope   event.SubscriptionScope
}

// New create a p2p server.
func New(opts *Options) *Server {

	v5nodes := make([]*discv5.Node, 0, len(opts.BootstrapNodes))
	for _, n := range opts.BootstrapNodes {
		v5nodes = append(v5nodes, discv5.NewNode(discv5.NodeID(n.ID), n.IP, n.UDP, n.TCP))
	}

	dialCh := make(chan *discover.Node, 8)
	goodNodes := w8cache.New(16, nil)
	for _, node := range opts.GoodNodes {
		select {
		case dialCh <- node:
		default:
		}
		goodNodes.Set(node.ID, node, 0)
	}

	return &Server{
		srv: &p2p.Server{
			Config: p2p.Config{
				Name:             opts.Name,
				PrivateKey:       opts.PrivateKey,
				MaxPeers:         opts.MaxSessions,
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
		discoveredNodes: w8cache.New(32, nil),
		dialCh:          dialCh,
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

// SubscribeSession subscribe session event.
// Call Session.Alive to check which envent is (join or leave).
func (s *Server) SubscribeSession(ch chan *Session) event.Subscription {
	return s.feedScope.Track(s.sessionFeed.Subscribe(ch))
}

func (s *Server) runProtocol(proto *Protocol) func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	return func(peer *p2p.Peer, rw p2p.MsgReadWriter) (err error) {
		log.Debug("p2p session established", "peer", peer.String())
		session := newSession(peer, proto)
		s.goes.Go(func() { s.sessionFeed.Send(session) })

		defer func() {
			if node := s.busyNodes.remove(peer.ID()); node != nil {
				s.goodNodes.Set(peer.ID(), node, session.stats.weight())
			}
			s.goes.Go(func() { s.sessionFeed.Send(session) })
			log.Debug("p2p session disconnected", "peer", peer.String(), "err", err)
		}()

		return session.serve(rw, proto.HandleRequest)
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
	gns := make([]*discover.Node, 0, s.goodNodes.Count())
	for _, entry := range s.goodNodes.All() {
		gns = append(gns, entry.Value.(*discover.Node))
	}
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
			newNode := discover.NewNode(discover.NodeID(v5node.ID), v5node.IP, v5node.UDP, v5node.TCP)
			log.Trace("p2p discovered", "peer", newNode)
			s.discoveredNodes.Set(newNode.ID, newNode, rand.Float64())
			if entry := s.discoveredNodes.PopWorst(); entry != nil {
				s.discoveredNodes.Set(entry.Key, entry.Value, rand.Float64())
				s.goes.Go(func() { s.dialCh <- newNode })
			}
		case <-s.done:
			close(setPeriod)
			return
		}
	}
}

func (s *Server) dialLoop() {
	for {
		select {
		case node := <-s.dialCh:

			if err := s.tryDial(node); err != nil {
				// TODO log
			}
		case <-s.done:
			return
		}
	}
}

func (s *Server) tryDial(node *discover.Node) (err error) {
	if s.srv.PeerCount() >= s.srv.MaxPeers {
		return
	}
	if s.busyNodes.contains(node.ID) {
		return
	}

	log.Trace("p2p try to dial", "peer", node)
	s.busyNodes.add(node)
	defer func() {
		if err != nil {
			log.Trace("p2p failed to dial", "peer", node, "err", err)
			s.busyNodes.remove(node.ID)
		}
	}()

	conn, err := s.srv.Dialer.Dial(node)
	if err != nil {
		return err
	}
	return s.srv.SetupConn(conn, 1, node)
}

type busyNodes struct {
	once  sync.Once
	nodes map[discover.NodeID]*discover.Node
	lock  sync.Mutex
}

func (bn *busyNodes) init() {
	bn.once.Do(func() {
		bn.nodes = make(map[discover.NodeID]*discover.Node)
	})
}

func (bn *busyNodes) add(node *discover.Node) {
	bn.init()
	bn.lock.Lock()
	defer bn.lock.Unlock()
	bn.nodes[node.ID] = node
}

func (bn *busyNodes) remove(id discover.NodeID) *discover.Node {
	bn.init()
	bn.lock.Lock()
	defer bn.lock.Unlock()
	if node, ok := bn.nodes[id]; ok {
		delete(bn.nodes, id)
		return node
	}
	return nil
}

func (bn *busyNodes) contains(id discover.NodeID) bool {
	bn.init()
	bn.lock.Lock()
	defer bn.lock.Unlock()
	return bn.nodes[id] != nil
}

// Nodes slice of discovered nodes.
// It's rlp encode/decodable
type Nodes []*discover.Node

// DecodeRLP implements rlp.Decoder.
func (ns *Nodes) DecodeRLP(s *rlp.Stream) error {
	_, err := s.List()
	if err != nil {
		return err
	}
	*ns = nil
	for {
		var n discover.Node
		if err := s.Decode(&n); err != nil {
			if err != rlp.EOL {
				return err
			}
			return nil
		}
		*ns = append(*ns, discover.NewNode(n.ID, n.IP, n.UDP, n.TCP))
	}
}
