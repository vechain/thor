package p2psrv

import (
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/w8cache"
)

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	srv        *p2p.Server
	runner     co.Runner
	done       chan struct{}
	sessions   Sessions
	sessionsMu sync.Mutex

	goodNodes       *w8cache.W8Cache
	discoveredNodes *w8cache.W8Cache
	dialingNodes    map[discover.NodeID]*discover.Node
	dialingNodesMu  sync.Mutex
	dialCh          chan *discover.Node
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
		discoveredNodes: w8cache.New(32, nil),
		goodNodes:       goodNodes,
		dialingNodes:    make(map[discover.NodeID]*discover.Node),
		dialCh:          dialCh,
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

func (s *Server) runProtocol(proto *Protocol) func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	return func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
		session := newSession(peer, proto)

		s.addSession(session)
		defer s.removeSession(session)

		defer func() {
			s.dialingNodesMu.Lock()
			node, ok := s.dialingNodes[peer.ID()]
			delete(s.dialingNodes, peer.ID())
			s.dialingNodesMu.Unlock()
			if ok {
				s.goodNodes.Set(peer.ID(), node, session.stats.weight())
			}
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
	s.runner.Go(func() { s.discoverLoop(discv5.Topic(discoTopic)) })
	s.runner.Go(s.dialLoop)
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	s.srv.Stop()
	close(s.done)
	s.runner.Wait()
}

// GoodNodes returns good nodes.
func (s *Server) GoodNodes() []*discover.Node {
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

// Sessions returns slice of alive sessions.
func (s *Server) Sessions() Sessions {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	return append(Sessions(nil), s.sessions...)
}

func (s *Server) addSession(session *Session) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.sessions = append(s.sessions, session)
}

func (s *Server) removeSession(session *Session) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for i, ss := range s.sessions {
		if ss == session {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			break
		}
	}
}

func (s *Server) discoverLoop(topic discv5.Topic) {
	if s.srv.DiscV5 == nil {
		return
	}

	setPeriod := make(chan time.Duration, 1)
	discNodes := make(chan *discv5.Node, 100)
	discLookups := make(chan bool, 100)

	s.runner.Go(func() {
		s.srv.DiscV5.RegisterTopic(topic, s.done)
	})

	s.runner.Go(func() {
		s.srv.DiscV5.SearchTopic(topic, setPeriod, discNodes, discLookups)
	})

	setPeriod <- time.Millisecond * 100
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
			s.discoveredNodes.Set(newNode.ID, newNode, rand.Float64())
			if entry := s.discoveredNodes.PopWorst(); entry != nil {
				s.discoveredNodes.Set(entry.Key, entry.Value, rand.Float64())
				select {
				case s.dialCh <- newNode:
				default:
				}
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
			s.sessionsMu.Lock()
			sessionCnt := len(s.sessions)
			s.sessionsMu.Unlock()
			if sessionCnt >= s.srv.MaxPeers {
				continue
			}

			s.dialingNodesMu.Lock()
			_, isDialing := s.dialingNodes[node.ID]
			s.dialingNodesMu.Unlock()

			if isDialing {
				continue
			}

			conn, err := s.srv.Dialer.Dial(node)
			if err != nil {
				// TODO log
				continue
			}

			s.dialingNodesMu.Lock()
			s.dialingNodes[node.ID] = node
			s.dialingNodesMu.Unlock()

			if err := s.srv.SetupConn(conn, 1, node); err != nil {
				s.dialingNodesMu.Lock()
				delete(s.dialingNodes, node.ID)
				s.dialingNodesMu.Unlock()
			}
		case <-s.done:
			return
		}
	}
}

func (s *Server) tryDial(node *discover.Node) {
	s.sessionsMu.Lock()
	scnt := len(s.sessions)
	s.sessionsMu.Unlock()
	if scnt >= s.srv.MaxPeers {
		return
	}

	s.dialingNodesMu.Lock()
	_, isDialing := s.dialingNodes[node.ID]
	s.dialingNodesMu.Unlock()
	if isDialing {
		return
	}

	conn, err := s.srv.Dialer.Dial(node)
	if err != nil {
		// TODO log
		return
	}

	s.dialingNodesMu.Lock()
	s.dialingNodes[node.ID] = node
	s.dialingNodesMu.Unlock()

	if err := s.srv.SetupConn(conn, 1, node); err != nil {
		s.dialingNodesMu.Lock()
		delete(s.dialingNodes, node.ID)
		s.dialingNodesMu.Unlock()
	}
}
