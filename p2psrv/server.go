// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"context"
	"errors"
	"math"
	"net"
	"slices"
	"time"

	"github.com/vechain/thor/v2/common/mclock"
	"github.com/vechain/thor/v2/p2p"
	"github.com/vechain/thor/v2/p2p/discover"
	discv5 "github.com/vechain/thor/v2/p2p/discv5"
	discv5discover "github.com/vechain/thor/v2/p2p/discv5/discover"
	"github.com/vechain/thor/v2/p2p/discv5/enode"
	"github.com/vechain/thor/v2/p2p/enr"
	"github.com/vechain/thor/v2/p2p/nat"
	"github.com/vechain/thor/v2/p2p/tempdiscv5"

	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/log"
)

var logger = log.WithContext("pkg", "p2psrv")

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	opts            *Options
	srv             *p2p.Server
	tempdiscv5      *tempdiscv5.Network
	discv5          *discv5discover.UDPv5
	goes            co.Goes
	done            chan struct{}
	bootstrapNodes  []*tempdiscv5.Node
	knownNodes      *cache.PrioCache
	discoveredNodes *cache.RandCache
	dialingNodes    *nodeMap
	discv5NewNodes  chan *enode.Node

	// Filter discv5 nodes based on the filter
	filterNodes func(*enode.Node) bool
}

// New create a p2p server.
func New(opts *Options, filterNodes func(node *enode.Node) bool) *Server {
	knownNodes := cache.NewPrioCache(5)
	discoveredNodes := cache.NewRandCache(128)
	for _, node := range opts.KnownNodes {
		knownNodes.Set(node.ID, node, 0)
		discoveredNodes.Set(node.ID, node)
	}

	return &Server{
		opts: opts,
		srv: &p2p.Server{
			Config: p2p.Config{
				Name:        opts.Name,
				PrivateKey:  opts.PrivateKey,
				MaxPeers:    opts.MaxPeers,
				NoDiscovery: true,  // disable discovery inside p2p.Server instance(we use our own)
				DiscoveryV5: false, // disable discovery inside p2p.Server instance(we use our own)
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
		filterNodes:     filterNodes,
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

// Start start the server.
func (s *Server) Start(protocols []*p2p.Protocol, topic *tempdiscv5.Topic) error {
	for _, proto := range protocols {
		cpy := *proto
		run := cpy.Run
		cpy.Run = func(peer *p2p.Peer, rw p2p.MsgReadWriter) (err error) {
			dir := "outbound"
			if peer.Inbound() {
				dir = "inbound"
			}
			log := logger.New("peer", peer, "dir", dir)

			log.Trace("peer connected")
			metricConnectedPeers().Add(1)

			startTime := mclock.Now()
			defer func() {
				log.Debug("peer disconnected", "reason", err)
				if node := s.dialingNodes.Remove(peer.ID()); node != nil {
					// we assume that good peer has longer connection duration.
					s.knownNodes.Set(peer.ID(), node, float64(mclock.Now()-startTime))
				}
				metricConnectedPeers().Add(-1)
			}()
			return run(peer, rw)
		}
		s.srv.Protocols = append(s.srv.Protocols, cpy)
	}

	for _, node := range s.opts.DiscoveryNodes {
		s.bootstrapNodes = append(s.bootstrapNodes, tempdiscv5.NewNode(tempdiscv5.NodeID(node.ID), node.IP, node.UDP, node.TCP))
	}
	// known nodes are also acting as bootstrap servers
	for _, node := range s.opts.KnownNodes {
		s.bootstrapNodes = append(s.bootstrapNodes, tempdiscv5.NewNode(tempdiscv5.NodeID(node.ID), node.IP, node.UDP, node.TCP))
	}

	var (
		conn      *net.UDPConn
		unhandled chan discv5discover.ReadPacket
	)

	// borrowed from ethereum/p2p.Server.Start
	addr, err := net.ResolveUDPAddr("udp", s.opts.ListenAddr)
	if err != nil {
		return err
	}
	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	if err := s.srv.Start(); err != nil {
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

	if s.opts.DiscV5 {
		unhandled = make(chan discv5discover.ReadPacket)
		var lconn discv5discover.UDPConn
		if s.opts.TempDiscV5 {
			conn := discv5.NewSharedUDPConn(conn, unhandled)
			lconn = &conn
		} else {
			lconn = conn
		}

		s.discv5NewNodes = make(chan *enode.Node, 1)
		laddr := conn.LocalAddr().(*net.UDPAddr)
		if topic != nil {
			stringTopic := string(*topic)
			if err := s.listenDiscV5(lconn, unhandled, laddr.Port, realaddr, &stringTopic); err != nil {
				return err
			}
		} else {
			if err := s.listenDiscV5(lconn, unhandled, laddr.Port, realaddr, nil); err != nil {
				return err
			}
		}

	}

	if s.opts.TempDiscV5 {
		if err := s.listenTempDiscV5(conn, realaddr, unhandled); err != nil {
			return err
		}
		if topic != nil {
			logger.Debug("registering topic", "topic", topic)
			s.goes.Go(func() {
				s.tempdiscv5.RegisterTopic(*topic, s.done)
			})
		}

		logger.Debug("searching topic", "topic", topic)

		s.goes.Go(s.fetchBootstrap)
	}

	s.goes.Go(func() {
		s.discoverLoop(topic)
	})

	logger.Debug("start up", "self", s.Self())

	if !s.opts.NoDial {
		s.goes.Go(s.dialLoop)
	}
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	if s.tempdiscv5 != nil {
		s.tempdiscv5.Close()
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

// Options returns the options.
func (s *Server) Options() *Options {
	return s.opts
}

// TryDial tries to establish a connection with the  the given node.
func (s *Server) TryDial(node *discover.Node) error {
	if s.dialingNodes.Contains(node.ID) {
		return nil
	}

	// Record the manual dialing node for future dial ratio calculation.
	// But the dial ratio limit is not applied to manual dialing.
	s.dialingNodes.Add(node)
	err := s.tryDial(node)
	if err != nil {
		s.dialingNodes.Remove(node.ID)
	}

	return err
}

func (s *Server) listenTempDiscV5(conn *net.UDPConn, realAddr *net.UDPAddr, unhandled chan discv5discover.ReadPacket) (err error) {
	network, err := tempdiscv5.ListenUDP(s.opts.PrivateKey, conn, realAddr, "", s.opts.NetRestrict, unhandled)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			network.Close()
		}
	}()

	if err := network.SetFallbackNodes(s.bootstrapNodes); err != nil {
		return err
	}
	s.tempdiscv5 = network
	return nil
}

func (s *Server) listenDiscV5(conn discv5discover.UDPConn, unhandled chan discv5discover.ReadPacket, port int, realAddr *net.UDPAddr, topic *string) (err error) {
	db, err := enode.OpenDB("")
	if err != nil {
		return err
	}
	localNode := enode.NewLocalNode(db, s.opts.PrivateKey)
	localNode.SetStaticIP(realAddr.IP)
	localNode.SetFallbackUDP(port)
	localNode.Set(enr.TCP(port))
	if topic != nil {
		localNode.Set(enr.WithEntry("eth", *topic))
	}
	bootnodes := make([]*enode.Node, len(s.bootstrapNodes))
	for index, node := range s.bootstrapNodes {
		pubkey, err := node.ID.Pubkey()
		if err != nil {
			return err
		}
		bootnodes[index] = enode.NewV4(pubkey, node.IP, int(node.TCP), int(node.UDP))
	}
	network, err := discv5discover.ListenV5(conn, localNode, discv5discover.Config{
		PrivateKey:              s.opts.PrivateKey,
		NetRestrict:             s.opts.NetRestrict,
		Unhandled:               unhandled,
		V5RespTimeout:           700 * time.Millisecond,
		Bootnodes:               bootnodes,
		PingInterval:            3 * time.Second,
		RefreshInterval:         30 * time.Minute,
		NoFindnodeLivenessCheck: false,
		V5ProtocolID:            nil, // if nil will use default
		ValidSchemes:            enode.ValidSchemes,
		Clock:                   mclock.System{},
	}, s.discv5NewNodes, s.filterNodes)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			network.Close()
		}
	}()

	s.discv5 = network
	return nil
}

func (s *Server) discoverLoop(topic *tempdiscv5.Topic) {
	setPeriod := make(chan time.Duration, 1)
	setPeriod <- time.Millisecond * 100
	discNodes := make(chan *tempdiscv5.Node, 100)
	discLookups := make(chan bool, 100)

	if s.tempdiscv5 != nil {
		if topic != nil {
			s.goes.Go(func() {
				s.tempdiscv5.SearchTopic(*topic, setPeriod, discNodes, discLookups)
			})
		}
	} else if s.discv5 == nil {
		return
	}

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
				metricDiscoveredNodes().Add(1)
				s.discoveredNodes.Set(node.ID, node)
				logger.Trace("discovered node", "node", node)
			}
		case v5node := <-s.discv5NewNodes:
			node := discover.NewNode(discover.NodeID(v5node.ID()), v5node.IP(), uint16(v5node.UDP()), uint16(v5node.TCP()))
			if _, found := s.discoveredNodes.Get(node.ID); !found {
				metricDiscoveredNodes().Add(1)
				s.discoveredNodes.Set(node.ID, node)
				logger.Trace("discovered node", "IP", node.IP, "UDP", node.UDP, "TCP", node.TCP, "node", v5node)
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

			log := logger.New("node", node)
			s.dialingNodes.Add(node)
			// don't use goes.Go, since the dial process can't be interrupted
			go func() {
				if err := s.tryDial(node); err != nil {
					s.dialingNodes.Remove(node.ID)
					log.Debug("failed to dial node", "err", err)
				}

				s.discoveredNodes.Remove(node.ID)
			}()

			dialCount++
		case <-s.done:
			return
		}
	}
}

func (s *Server) tryDial(node *discover.Node) error {
	metricDialingNewNode().Add(1)
	defer metricDialingNewNode().Add(-1)

	conn, err := s.srv.Dialer.Dial(node)
	if err != nil {
		return err
	}
	return s.srv.SetupConn(conn, 1, node)
}

func (s *Server) fetchBootstrap() {
	if s.opts.RemoteDiscoveryList == "" {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.done
		cancel()
	}()

	f := func() error {
		remoteNodes, err := fetchRemoteBootstrapNodes(ctx, s.opts.RemoteDiscoveryList)
		if err != nil {
			return err
		}

		bootnodes := slices.Clone(s.bootstrapNodes)
		bootnodes = append(bootnodes, remoteNodes...)
		if err := s.tempdiscv5.SetFallbackNodes(bootnodes); err != nil {
			return err
		}
		return nil
	}

	for {
		err := f()
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		logger.Warn("update bootstrap nodes from remote failed", "err", err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 10):
		}
	}
}
