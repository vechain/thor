// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
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
	srv             *p2p.Server
	goes            co.Goes
	done            chan struct{}
	knownNodes      *cache.PrioCache
	discoveredNodes *cache.RandCache
	dialingNodes    *nodeMap
}

// New create a p2p server.
func New(opts *Options) *Server {
	bootstrapsV5 := make([]*discv5.Node, 0, len(opts.BootstrapNodes))
	for _, node := range opts.BootstrapNodes {
		bootstrapsV5 = append(bootstrapsV5, discv5.NewNode(discv5.NodeID(node.ID), node.IP, node.UDP, node.TCP))
	}

	knownNodes := cache.NewPrioCache(5)
	discoveredNodes := cache.NewRandCache(128)
	for _, node := range opts.KnownNodes {
		knownNodes.Set(node.ID, node, 0)
		discoveredNodes.Set(node.ID, node)

		bootstrapsV5 = append(bootstrapsV5, discv5.NewNode(discv5.NodeID(node.ID), node.IP, node.UDP, node.TCP))
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
				BootstrapNodesV5: bootstrapsV5,
				NetRestrict:      opts.NetRestrict,
				NAT:              opts.NAT,
				NoDial:           opts.NoDial,
				DialRatio:        int(math.Sqrt(float64(opts.MaxPeers))),
			},
		},
		done:            make(chan struct{}),
		knownNodes:      knownNodes,
		discoveredNodes: discoveredNodes,
		dialingNodes:    newNodeMap(),
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

// Start start the server.
func (s *Server) Start(protocols []*Protocol) error {
	for _, proto := range protocols {
		cpy := proto.Protocol
		run := cpy.Run
		cpy.Run = func(peer *p2p.Peer, rw p2p.MsgReadWriter) (err error) {
			dir := "outbound"
			if peer.Inbound() {
				dir = "inbound"
			}
			log := log.New("peer", peer, "dir", dir)

			log.Debug("peer connected")
			startTime := mclock.Now()
			defer func() {
				log.Debug("peer disconnected", "reason", err)
				if node := s.dialingNodes.Remove(peer.ID()); node != nil {
					// we assume that good peer has longer connection duration.
					s.knownNodes.Set(peer.ID(), node, float64(mclock.Now()-startTime))
				}
			}()
			return run(peer, rw)
		}
		s.srv.Protocols = append(s.srv.Protocols, cpy)
	}

	if err := s.srv.Start(); err != nil {
		return err
	}
	log.Debug("start up", "self", s.Self())

	for _, proto := range protocols {
		topicToRegister := discv5.Topic(proto.DiscTopic)
		log.Debug("registering topic", "topic", topicToRegister)
		s.goes.Go(func() {
			s.srv.DiscV5.RegisterTopic(topicToRegister, s.done)
		})
	}

	if len(protocols) > 0 {
		topicToSearch := discv5.Topic(protocols[len(protocols)-1].DiscTopic)
		log.Debug("searching topic", "topic", topicToSearch)
		s.goes.Go(func() { s.discoverLoop(topicToSearch) })
		s.goes.Go(s.dialLoop)
	}
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	s.srv.Stop()
	close(s.done)
	s.goes.Wait()
}

// KnownNodes returns known nodes that can be saved for fast connecting next time.
func (s *Server) KnownNodes() Nodes {
	nodes := make([]*discover.Node, 0, s.knownNodes.Len())
	s.knownNodes.ForEach(func(ent *cache.PrioEntry) bool {
		nodes = append(nodes, ent.Value.(*discover.Node))
		return true
	})
	return nodes
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
	const fastDialDur = 500 * time.Millisecond
	const nonFastDialDur = 2 * time.Second
	const stableDialDur = 10 * time.Second

	// fast dialing initially
	ticker := time.NewTicker(fastDialDur)
	defer ticker.Stop()

	dialCount := 0
	for {
		select {
		case <-ticker.C:
			if s.srv.DialRatio < 1 {
				continue
			}

			if s.dialingNodes.Len() >= s.srv.MaxPeers/s.srv.DialRatio {
				continue
			}

			entry := s.discoveredNodes.Pick()
			if entry == nil {
				continue
			}

			node := entry.Value.(*discover.Node)
			if s.dialingNodes.Contains(node.ID) {
				continue
			}

			log := log.New("node", node)
			log.Debug("try to dial node")
			s.dialingNodes.Add(node)
			// don't use goes.Go, since the dial process can't be interrupted
			go func() {
				if err := s.tryDial(node); err != nil {
					s.dialingNodes.Remove(node.ID)
					log.Debug("failed to dial node", "err", err)
				}
			}()

			dialCount++
			if dialCount == 20 {
				ticker.Stop()
				ticker = time.NewTicker(nonFastDialDur)
			} else if dialCount > 20 {
				if s.srv.PeerCount() > s.srv.MaxPeers/2 {
					ticker.Stop()
					ticker = time.NewTicker(stableDialDur)
				} else {
					ticker.Stop()
					ticker = time.NewTicker(nonFastDialDur)
				}
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
