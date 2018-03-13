package p2psrv

import (
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/vechain/thor/co"
)

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	srv        *p2p.Server
	runner     co.Runner
	done       chan struct{}
	sessionSet *SessionSet
}

// New create a p2p server.
func New(opts *Options) *Server {

	v5nodes := make([]*discv5.Node, 0, len(opts.BootstrapNodes))
	for _, n := range opts.BootstrapNodes {
		v5nodes = append(v5nodes, discv5.NewNode(discv5.NodeID(n.ID), n.IP, n.UDP, n.TCP))
	}

	return &Server{
		srv: &p2p.Server{
			Config: p2p.Config{
				Name:             opts.Name,
				PrivateKey:       opts.PrivateKey,
				MaxPeers:         opts.MaxPeers,
				MaxPendingPeers:  opts.MaxPendingPeers,
				NoDiscovery:      true,
				DiscoveryV5:      !opts.NoDiscovery,
				ListenAddr:       opts.ListenAddr,
				BootstrapNodesV5: v5nodes,
				StaticNodes:      opts.StaticNodes,
				TrustedNodes:     opts.TrustedNodes,
				NetRestrict:      opts.NetRestrict,
				NAT:              opts.NAT,
				NoDial:           opts.NoDial,
			},
		},
		done:       make(chan struct{}),
		sessionSet: newSessionSet(),
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

// Start start the server.
func (s *Server) Start(discoTopic string, protocols []*Protocol) error {
	for _, p := range protocols {
		s.srv.Protocols = append(s.srv.Protocols, p2p.Protocol{
			Name:    p.Name,
			Version: uint(p.Version),
			Length:  p.Length,
			//			NodeInfo: p.NodeInfo,
			//			PeerInfo: p.PeerInfo,
			Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
				session := newSession(peer, p)
				s.sessionSet.add(session)
				defer s.sessionSet.remove(session)
				return session.serve(rw, p.HandleRequest)
			},
		})
	}
	if err := s.srv.Start(); err != nil {
		return err
	}
	s.startDiscoverLoop(discv5.Topic(discoTopic))
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	s.srv.Stop()
	close(s.done)
	s.runner.Wait()
}

// AddPeer connects to the given node and maintains the connection until the
// server is shut down. If the connection fails for any reason, the server will
// attempt to reconnect the peer.
func (s *Server) AddPeer(node *discover.Node) {
	s.srv.AddPeer(node)
}

// RemovePeer disconnects from the given node
func (s *Server) RemovePeer(node *discover.Node) {
	s.srv.RemovePeer(node)
}

// NodeInfo gathers and returns a collection of metadata known about the host.
func (s *Server) NodeInfo() *p2p.NodeInfo {
	return s.srv.NodeInfo()
}

// SessionSet returns a set of alive sessions.
func (s *Server) SessionSet() *SessionSet {
	return s.sessionSet
}

func (s *Server) startDiscoverLoop(topic discv5.Topic) {
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

	s.runner.Go(func() {
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
			case node := <-discNodes:
				s.srv.AddPeer(discover.NewNode(discover.NodeID(node.ID), node.IP, node.UDP, node.TCP))
			case <-s.done:
				close(setPeriod)
				return
			}
		}
	})
}
