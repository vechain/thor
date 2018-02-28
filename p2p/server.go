package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"net"
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
	PrivateKey *ecdsa.PrivateKey `toml:"-"`

	// MaxPeers is the maximum number of peers that can be
	// connected. It must be greater than zero.
	MaxPeers int

	// MaxPendingPeers is the maximum number of peers that can be pending in the
	// handshake phase, counted separately for inbound and outbound connections.
	// Zero defaults to preset values.
	MaxPendingPeers int `toml:",omitempty"`

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
	BootstrapNodes []*discv5.Node `toml:",omitempty"`

	// Static nodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	StaticNodes []*discover.Node

	// Trusted nodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit.
	TrustedNodes []*discover.Node

	// Connectivity can be restricted to certain IP networks.
	// If this option is set to a non-nil value, only hosts which match one of the
	// IP networks contained in the list are considered.
	NetRestrict *netutil.Netlist `toml:",omitempty"`

	// If set to a non-nil value, the given NAT port mapper
	// is used to make the listening port available to the
	// Internet.
	NAT nat.Interface `toml:",omitempty"`

	// If NoDial is true, the server will not dial any peers.
	NoDial bool `toml:",omitempty"`

	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.
	Protocols []p2p.Protocol `toml:"-"`

	// Discovery v5 topic
	Topic string
}

// Server p2p server wraps ethereum's p2p.Server, and handles discovery v5 stuff.
type Server struct {
	Options
	runner co.Runner
	srv    *p2p.Server
	done   chan struct{}
}

// Self returns self enode url.
// Only available when server is running.
func (s *Server) Self() *discover.Node {
	if s.srv != nil {
		return s.srv.Self()
	}
	return &discover.Node{IP: net.ParseIP("0.0.0.0")}
}

// Start start the server.
func (s *Server) Start() error {
	addr, err := net.ResolveUDPAddr("udp", s.ListenAddr)
	if err != nil {
		return err
	}
	// discv5 use discport-1 as peer port
	discAddr := addr
	discAddr.Port++
	s.srv = &p2p.Server{
		Config: p2p.Config{
			Name:             s.Name,
			PrivateKey:       s.PrivateKey,
			MaxPeers:         s.MaxPeers,
			MaxPendingPeers:  s.MaxPendingPeers,
			NoDiscovery:      true,
			DiscoveryV5:      !s.NoDiscovery,
			DiscoveryV5Addr:  discAddr.String(),
			ListenAddr:       s.ListenAddr,
			BootstrapNodesV5: s.BootstrapNodes,
			StaticNodes:      s.StaticNodes,
			TrustedNodes:     s.TrustedNodes,
			NetRestrict:      s.NetRestrict,
			NAT:              s.NAT,
			NoDial:           s.NoDial,
			Protocols:        s.Protocols,
		},
	}

	if err := s.srv.Start(); err != nil {
		return err
	}
	s.done = make(chan struct{})

	s.startDiscoverLoop()
	return nil
}

// Stop stop the server.
func (s *Server) Stop() {
	if s.srv != nil {
		s.srv.Stop()
	}
	if s.done != nil {
		close(s.done)
	}
	s.runner.Wait()
	s.srv = nil
	s.done = nil
}

func (s *Server) startDiscoverLoop() {
	s.runner.Go(func() {
		s.srv.DiscV5.RegisterTopic(discv5.Topic(s.Topic), s.done)
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
				fmt.Println(node.ID, node.UDP, node.TCP)
				n, err := discover.ParseNode(node.String())
				if err != nil {
					continue
				}
				s.srv.AddPeer(n)
			case <-s.done:
				return
			}
		}
	})

	s.runner.Go(func() {
		s.srv.DiscV5.SearchTopic(discv5.Topic(s.Topic), setPeriod, discNodes, discLookups)
	})
}
