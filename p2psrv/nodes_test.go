// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"bytes"
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/p2p/discover"
)

func dummyNode(idbyte byte) *discover.Node {
	id := discover.NodeID{}
	id[0] = idbyte
	return &discover.Node{ID: id, IP: net.IPv4(127, 0, 0, idbyte), UDP: 1000 + uint16(idbyte), TCP: 2000 + uint16(idbyte)}
}

func TestNodeMapBasic(t *testing.T) {
	nm := newNodeMap()
	n1 := dummyNode(1)
	n2 := dummyNode(2)

	assert.Equal(t, 0, nm.Len())
	assert.False(t, nm.Contains(n1.ID))

	nm.Add(n1)
	assert.True(t, nm.Contains(n1.ID))
	assert.Equal(t, 1, nm.Len())

	nm.Add(n2)
	assert.True(t, nm.Contains(n2.ID))
	assert.Equal(t, 2, nm.Len())

	removed := nm.Remove(n1.ID)
	assert.Equal(t, n1, removed)
	assert.False(t, nm.Contains(n1.ID))
	assert.Equal(t, 1, nm.Len())

	removedNil := nm.Remove(n1.ID)
	assert.Nil(t, removedNil)
}

func TestNodesDecodeRLP(t *testing.T) {
	n1 := dummyNode(1)
	n2 := dummyNode(2)
	nodes := Nodes{n1, n2}
	var buf bytes.Buffer
	err := rlp.Encode(&buf, nodes)
	assert.NoError(t, err)

	var decoded Nodes
	err = rlp.DecodeBytes(buf.Bytes(), &decoded)
	assert.NoError(t, err)
	assert.Len(t, decoded, 2)
	assert.Equal(t, n1.ID, decoded[0].ID)
	assert.Equal(t, n2.ID, decoded[1].ID)
}

func TestNodesDecodeRLP_Error(t *testing.T) {
	// Not a valid RLP list
	var ns Nodes
	bad := []byte{0x01, 0x02, 0x03}
	err := rlp.DecodeBytes(bad, &ns)
	assert.Error(t, err)
}
