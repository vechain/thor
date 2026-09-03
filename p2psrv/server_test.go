// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/p2p/discover"
)

func TestNewServer(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Unable to generate private key: %v", err)
	}

	node := discover.MustParseNode(
		"enode://1234cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@10.3.58.6:30303?discport=30301",
	)
	opts := &Options{
		Name:        "testNode",
		PrivateKey:  privateKey,
		MaxPeers:    10,
		ListenAddr:  ":30303",
		NetRestrict: nil,
		NAT:         nil,
		NoDial:      false,
		KnownNodes:  Nodes{node},
	}

	server := New(opts)

	assert.Equal(t, "testNode", server.opts.Name)
	assert.Equal(t, privateKey, server.opts.PrivateKey)
	assert.Equal(t, 10, server.opts.MaxPeers)
	assert.Equal(t, ":30303", server.opts.ListenAddr)
	assert.Equal(t, server.discoveredNodes.Len(), 1)
	assert.Equal(t, server.knownNodes.Len(), 1)
	assert.True(t, server.discoveredNodes.Contains(node.ID))
	assert.True(t, server.knownNodes.Contains(node.ID))
	assert.False(t, server.opts.NoDial)
}

func TestNewServerConnectOnly(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Unable to generate private key: %v", err)
	}

	knownNode := discover.MustParseNode(
		"enode://1234cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@10.3.58.6:30303?discport=30301",
	)
	opts := &Options{
		Name:        "testNode",
		PrivateKey:  privateKey,
		MaxPeers:    10,
		ListenAddr:  ":30303",
		NetRestrict: nil,
		NAT:         nil,
		NoDial:      false,
		KnownNodes:  Nodes{knownNode},
	}

	server := New(opts)

	assert.Equal(t, "testNode", server.opts.Name)
	assert.Equal(t, privateKey, server.opts.PrivateKey)
	assert.Equal(t, 10, server.opts.MaxPeers)
	assert.Equal(t, ":30303", server.opts.ListenAddr)
	assert.False(t, server.opts.NoDial)

	assert.Equal(t, server.discoveredNodes.Len(), 1)
	assert.Equal(t, server.knownNodes.Len(), 1)
	assert.True(t, server.discoveredNodes.Contains(knownNode.ID))
	assert.True(t, server.knownNodes.Contains(knownNode.ID))
}

// int(sqrt(MaxPeers)) rounds down to 1 for MaxPeers 1..3, which would give outbound
// every slot and leave the inbound cap at 0. MaxPeers >= 4 keeps the plain sqrt.
// The {25, 5} row is p2psrv's side of the 5 slots p2p reserves (asserted by
// p2p.TestMaxInboundAndDialedConns).
func TestDialRatioFloor(t *testing.T) {
	tests := []struct {
		maxPeers  int
		wantRatio int
	}{
		{0, 2},
		{1, 2},
		{2, 2},
		{3, 2},
		{4, 2},
		{9, 3},
		{25, 5},
	}
	for _, tt := range tests {
		s := New(&Options{MaxPeers: tt.maxPeers})
		assert.Equal(t, tt.wantRatio, s.srv.DialRatio, "MaxPeers=%d", tt.maxPeers)
	}
}

// dialLoop's cap must agree with the slots p2p keeps free. MaxPeers 1 is the case
// that broke: DialRatio is floored at 2, so 1/2 truncated to 0 and it never dialed.
func TestOutboundQuota(t *testing.T) {
	tests := []struct {
		maxPeers  int
		wantQuota int
	}{
		{0, 0},
		{1, 1},
		{2, 1},
		{3, 1},
		{4, 2},
		{9, 3},
		{25, 5},
	}
	for _, tt := range tests {
		s := New(&Options{MaxPeers: tt.maxPeers})
		assert.Equal(t, tt.wantQuota, s.outboundQuota(), "MaxPeers=%d", tt.maxPeers)
	}
}
