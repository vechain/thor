// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Unable to generate private key: %v", err)
	}

	node := discover.MustParseNode("enode://1234cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@10.3.58.6:30303?discport=30301")
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
	assert.Equal(t, server.allowedOnlyNodes.Len(), 0)
	assert.True(t, server.discoveredNodes.Contains(node.ID))
	assert.True(t, server.knownNodes.Contains(node.ID))
	assert.False(t, server.opts.NoDial)
}

func TestNewServerConnectOnly(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Unable to generate private key: %v", err)
	}

	knownNode := discover.MustParseNode("enode://1234cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@10.3.58.6:30303?discport=30301")
	knownNode2 := discover.MustParseNode("enode://5555cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@10.3.58.6:30303?discport=30301")
	connectOnlyNode := discover.MustParseNode("enode://1094cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@10.3.58.6:30303?discport=30301")
	opts := &Options{
		Name:         "testNode",
		PrivateKey:   privateKey,
		MaxPeers:     10,
		ListenAddr:   ":30303",
		NetRestrict:  nil,
		NAT:          nil,
		NoDial:       false,
		KnownNodes:   Nodes{knownNode, knownNode2},
		AllowedPeers: Nodes{connectOnlyNode},
	}

	server := New(opts)

	assert.Equal(t, "testNode", server.opts.Name)
	assert.Equal(t, privateKey, server.opts.PrivateKey)
	assert.Equal(t, 10, server.opts.MaxPeers)
	assert.Equal(t, ":30303", server.opts.ListenAddr)
	assert.False(t, server.opts.NoDial)

	assert.Equal(t, server.discoveredNodes.Len(), 1)
	assert.Equal(t, server.knownNodes.Len(), 1)
	assert.Equal(t, server.allowedOnlyNodes.Len(), 1)
	assert.True(t, server.discoveredNodes.Contains(connectOnlyNode.ID))
	assert.True(t, server.knownNodes.Contains(connectOnlyNode.ID))
	assert.True(t, server.allowedOnlyNodes.Contains(connectOnlyNode.ID))
}
