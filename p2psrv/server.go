package p2psrv

import (
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/p2p/netutil"
	"github.com/vechain/thor/co"
)

// Options options for creating p2p server.
// Partially copied from ethereum p2p.Config.
type Options struct {
	// Name sets the node name of this server.
	// Use common.MakeName to create a name that follows existing conventions.
	Name string

	// This field must be set to a valid secp256k1 private key.
	PrivateKey *ecdsa.PrivateKey

	// MaxPeers is the maximum number of peers that can be
	// connected. It must be greater than zero.
	MaxPeers int

	// MaxPendingPeers is the maximum number of peers that can be pending in the
	// handshake phase, counted separately for inbound and outbound connections.
	// Zero defaults to preset values.
	MaxPendingPeers int

	// NoDiscovery can be used to disable the peer discovery mechanism.
	// Disabling is useful for protocol debugging (manual topology).
	NoDiscovery bool

	// If ListenAddr is set to a non-nil address, the server
	// will listen for incoming connections.
	//
	// If the port is zero, the operating system will pick a port. The
	// ListenAddr field will be updated with the actual address when
	// the server is started.
	ListenAddr string

	// BootstrapNodes are used to establish connectivity
	// with the rest of the network using the V5 discovery
	// protocol.
	BootstrapNodes []*discover.Node

	// Static nodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	StaticNodes []*discover.Node

	// Trusted nodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit.
	TrustedNodes []*discover.Node

	// Connectivity can be restricted to certain IP networks.
	// If this option is set to a non-nil value, only hosts which match one of the
	// IP networks contained in the list are considered.
	NetRestrict *netutil.Netlist

	// If set to a non-nil value, the given NAT port mapper
	// is used to make the listening port available to the
	// Internet.
	NAT nat.Interface

	// If NoDial is true, the server will not dial any peers.
	NoDial bool

	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.
	Protocols []p2p.Protocol

	// Discovery v5 topic
	Topic string
}

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	srv    *p2p.Server
	topic  discv5.Topic
	runner co.Runner
	done   chan struct{}
}

// New create a p2p server.
func New(opts Options) *Server {

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
				Protocols:        opts.Protocols,
			},
		},
		topic: discv5.Topic(opts.Topic),
		done:  make(chan struct{}),
	}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	return s.srv.Self()
}

// Start start the server.
func (s *Server) Start() error {
	if err := s.srv.Start(); err != nil {
		return err
	}
	s.startDiscoverLoop()
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

// PeerCount returns the number of connected peers.
func (s *Server) PeerCount() int {
	return s.srv.PeerCount()
}

// PeersInfo returns an array of metadata objects describing connected peers.
func (s *Server) PeersInfo() []*p2p.PeerInfo {
	return s.srv.PeersInfo()
}

func (s *Server) startDiscoverLoop() {
	if s.srv.DiscV5 == nil {
		return
	}

	s.runner.Go(func() {
		s.srv.DiscV5.RegisterTopic(s.topic, s.done)
	})

	var (
		setPeriod   = make(chan time.Duration, 1)
		discNodes   = make(chan *discv5.Node, 100)
		discLookups = make(chan bool, 100)

		lookupCount  = 0
		fastDiscover = true
		convTime     mclock.AbsTime
	)
	setPeriod <- time.Millisecond * 100

	s.runner.Go(func() {
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
				return
			}
		}
	})

	s.runner.Go(func() {
		s.srv.DiscV5.SearchTopic(s.topic, setPeriod, discNodes, discLookups)
	})
}
