// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"context"
	"errors"
	"math"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/nat"

	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/co"
)

var log = log15.New("pkg", "p2psrv")

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	opts            Options
	srv             *p2p.Server
	discv5          *discv5.Network
	goes            co.Goes
	done            chan struct{}
	bootstrapNodes  []*discv5.Node
	knownNodes      *cache.PrioCache
	discoveredNodes *cache.RandCache
	dialingNodes    *nodeMap
}

// New create a p2p server.
func New(opts *Options) *Server {
	knownNodes := cache.NewPrioCache(5)
	discoveredNodes := cache.NewRandCache(128)
	for _, node := range opts.KnownNodes {
		knownNodes.Set(node.ID, node, 0)
		discoveredNodes.Set(node.ID, node)
	}

	return &Server{
		opts: *opts,
		srv: &p2p.Server{
			Config: p2p.Config{
				Name:        opts.Name,
				PrivateKey:  opts.PrivateKey,
				MaxPeers:    opts.MaxPeers,
				NoDiscovery: true,
				DiscoveryV5: false, // disable discovery inside p2p.Server instance
				ListenAddr:  opts.ListenAddr,
				NetRestrict: opts.NetRestrict,
				NAT:         opts.NAT,
				NoDial:      opts.NoDial,
				DialRatio:   int(math.Sqrt(float64(opts.MaxPeers))),
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
func (s *Server) Start(protocols []*p2p.Protocol, topic discv5.Topic) error {
	for _, proto := range protocols {
		cpy := *proto
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
	if !s.opts.NoDiscovery {
		if err := s.listenDiscV5(); err != nil {
			return err
		}
		log.Debug("registering topic", "topic", topic)
		s.goes.Go(func() {
			s.discv5.RegisterTopic(topic, s.done)
		})

		log.Debug("searching topic", "topic", topic)
		s.goes.Go(func() {
			s.discoverLoop(topic)
		})

		s.goes.Go(s.fetchBootstrap)
	}

	log.Debug("start up", "self", s.Self())

	s.goes.Go(s.dialLoop)
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	if s.discv5 != nil {
		s.discv5.Close()
	}
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

func (s *Server) listenDiscV5() (err error) {
	// borrowed from ethereum/p2p.Server.Start
	addr, err := net.ResolveUDPAddr("udp", s.opts.ListenAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	realaddr := conn.LocalAddr().(*net.UDPAddr)
	if s.opts.NAT != nil {
		if !realaddr.IP.IsLoopback() {
			s.goes.Go(func() { nat.Map(s.opts.NAT, s.done, "udp", realaddr.Port, realaddr.Port, "vechain discovery") })
		}
		// TODO: react to external IP changes over time.
		if ext, err := s.opts.NAT.ExternalIP(); err == nil {
			realaddr = &net.UDPAddr{IP: ext, Port: realaddr.Port}
		}
	}

	network, err := discv5.ListenUDP(s.opts.PrivateKey, conn, realaddr, "", s.opts.NetRestrict)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			network.Close()
		}
	}()

	for _, node := range s.opts.BootstrapNodes {
		s.bootstrapNodes = append(s.bootstrapNodes, discv5.NewNode(discv5.NodeID(node.ID), node.IP, node.UDP, node.TCP))
	}
	for _, node := range s.opts.KnownNodes {
		s.bootstrapNodes = append(s.bootstrapNodes, discv5.NewNode(discv5.NodeID(node.ID), node.IP, node.UDP, node.TCP))

	}

	if err := network.SetFallbackNodes(s.bootstrapNodes); err != nil {
		return err
	}
	s.discv5 = network
	return nil
}

func (s *Server) discoverLoop(topic discv5.Topic) {
	if s.discv5 == nil {
		return
	}

	setPeriod := make(chan time.Duration, 1)
	setPeriod <- time.Millisecond * 100
	discNodes := make(chan *discv5.Node, 100)
	discLookups := make(chan bool, 100)

	s.goes.Go(func() {
		s.discv5.SearchTopic(topic, setPeriod, discNodes, discLookups)
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

	dialCount := 0
	for {
		delay := fastDialDur
		if dialCount == 20 {
			delay = nonFastDialDur
		} else if dialCount > 20 {
			if s.srv.PeerCount() > s.srv.MaxPeers/2 {
				delay = stableDialDur
			} else {
				delay = nonFastDialDur
			}
		}

		select {
		case <-time.After(delay):
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

func (s *Server) fetchBootstrap() {
	if s.opts.RemoteBootstrap == "" {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.done
		cancel()
	}()

	f := func() error {
		remoteNodes, err := fetchRemoteBootstrapNodes(ctx, s.opts.RemoteBootstrap)
		if err != nil {
			return err
		}

		bootnodes := append([]*discv5.Node(nil), s.bootstrapNodes...)
		bootnodes = append(bootnodes, remoteNodes...)
		if err := s.discv5.SetFallbackNodes(bootnodes); err != nil {
			return err
		}
		return nil
	}

	for {
		if err := f(); err == nil || errors.Is(err, context.Canceled) {
			return
		} else {
			log.Warn("update bootstrap nodes from remote failed", "err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 10):
		}
	}
}
