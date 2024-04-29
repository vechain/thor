package p2p

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/v2/p2psrv"
	"github.com/vechain/thor/v2/test/datagen"

	"github.com/stretchr/testify/assert"
)

func TestNewThorP2P(t *testing.T) {
	// Generate a private key for testing
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Setup test cases
	tests := []struct {
		name               string
		maxPeers           int
		listenAddr         string
		allowedPeers       []*discover.Node
		cachedPeers        []*discover.Node
		bootstrapNodes     []*discover.Node
		expectedKnownNodes p2psrv.Nodes
	}{
		{
			name:               "Basic instance with no default settings",
			maxPeers:           datagen.RandInt(),
			listenAddr:         datagen.RandHostPort(),
			allowedPeers:       nil,
			cachedPeers:        nil,
			bootstrapNodes:     nil,
			expectedKnownNodes: fallbackBootstrapNodes,
		},
		{
			name:               "Instance with allowed peers only",
			maxPeers:           datagen.RandInt(),
			listenAddr:         datagen.RandHostPort(),
			allowedPeers:       []*discover.Node{{ID: discover.NodeID{1}}, {ID: discover.NodeID{2}}},
			cachedPeers:        []*discover.Node{{ID: discover.NodeID{100}}},
			bootstrapNodes:     []*discover.Node{{ID: discover.NodeID{200}}},
			expectedKnownNodes: p2psrv.Nodes{{ID: discover.NodeID{1}}, {ID: discover.NodeID{2}}},
		},
		{
			name:               "Cached peers append with default fallback nodes",
			maxPeers:           datagen.RandInt(),
			listenAddr:         datagen.RandHostPort(),
			cachedPeers:        []*discover.Node{{ID: discover.NodeID{2}}, {ID: discover.NodeID{3}}, {ID: discover.NodeID{4}}},
			bootstrapNodes:     nil,
			expectedKnownNodes: append(fallbackBootstrapNodes, p2psrv.Nodes{{ID: discover.NodeID{2}}, {ID: discover.NodeID{3}}, {ID: discover.NodeID{4}}}...),
		},
		{
			name:               "Cached and bootstrap nodes flag are appended",
			maxPeers:           datagen.RandInt(),
			listenAddr:         datagen.RandHostPort(),
			cachedPeers:        []*discover.Node{{ID: discover.NodeID{2}}, {ID: discover.NodeID{22}}, {ID: discover.NodeID{222}}},
			bootstrapNodes:     []*discover.Node{{ID: discover.NodeID{3}}},
			expectedKnownNodes: p2psrv.Nodes{{ID: discover.NodeID{3}}, {ID: discover.NodeID{2}}, {ID: discover.NodeID{22}}, {ID: discover.NodeID{222}}},
		},
		{
			name:               "Duplicated nodes are removed (cached and bootstrap nodes) ",
			maxPeers:           datagen.RandInt(),
			listenAddr:         datagen.RandHostPort(),
			cachedPeers:        []*discover.Node{{ID: discover.NodeID{2}}, {ID: discover.NodeID{5}}, {ID: discover.NodeID{2}}, {ID: discover.NodeID{2}}, {ID: discover.NodeID{5}}},
			bootstrapNodes:     []*discover.Node{{ID: discover.NodeID{3}}, {ID: discover.NodeID{33}}, {ID: discover.NodeID{33}}, {ID: discover.NodeID{3}}},
			expectedKnownNodes: p2psrv.Nodes{{ID: discover.NodeID{3}}, {ID: discover.NodeID{33}}, {ID: discover.NodeID{2}}, {ID: discover.NodeID{5}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Instantiate ThorP2P
			thor := New(
				nil,
				privateKey,
				"/tmp/thor-instance",
				nil,
				"1.0",
				tc.maxPeers,
				123,
				tc.listenAddr,
				tc.allowedPeers,
				tc.cachedPeers,
				tc.bootstrapNodes,
			)

			assert.Equal(t, thor.p2pSrv.Options().KnownNodes, tc.expectedKnownNodes)
			assert.NotNil(t, thor, "ThorP2P instance should not be nil")
			assert.Equal(t, thor.p2pSrv.Options().MaxPeers, tc.maxPeers)
			assert.Equal(t, thor.p2pSrv.Options().ListenAddr, tc.listenAddr)
		})
	}
}
