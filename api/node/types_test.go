// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node_test

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/thor"
)

func TestConvertPeersStats(t *testing.T) {
	// Test case 1: Empty input slice
	ss := []*comm.PeerStats{}
	expected := []*node.PeerStats(nil)
	assert.Equal(t, expected, node.ConvertPeersStats(ss))

	// Test case 2: Non-empty input slice
	bestBlock1 := randomBytes32()
	bestBlock2 := randomBytes32()
	ss = []*comm.PeerStats{
		{
			Name:        "peer1",
			BestBlockID: bestBlock1,
			TotalScore:  100,
			PeerID:      "peerID1",
			NetAddr:     "netAddr1",
			Inbound:     true,
			Duration:    10,
		},
		{
			Name:        "peer2",
			BestBlockID: bestBlock2,
			TotalScore:  200,
			PeerID:      "peerID2",
			NetAddr:     "netAddr2",
			Inbound:     false,
			Duration:    20,
		},
	}
	expected = []*node.PeerStats{
		{
			Name:        "peer1",
			BestBlockID: bestBlock1,
			TotalScore:  100,
			PeerID:      "peerID1",
			NetAddr:     "netAddr1",
			Inbound:     true,
			Duration:    10,
		},
		{
			Name:        "peer2",
			BestBlockID: bestBlock2,
			TotalScore:  200,
			PeerID:      "peerID2",
			NetAddr:     "netAddr2",
			Inbound:     false,
			Duration:    20,
		},
	}
	assert.Equal(t, expected, node.ConvertPeersStats(ss))
}

func randomBytes32() thor.Bytes32 {
	var b32 thor.Bytes32

	rand.Read(b32[:])
	return b32
}
