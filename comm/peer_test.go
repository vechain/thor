// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func makePeerWithScore(score uint64) *Peer {
	return &Peer{
		head: struct {
			sync.Mutex
			id         thor.Bytes32
			totalScore uint64
		}{
			totalScore: score,
		},
	}
}

func TestWithBestScore(t *testing.T) {
	peer1 := makePeerWithScore(100)
	peer2 := makePeerWithScore(200)
	peer3 := makePeerWithScore(150)

	peerSet := &PeerSet{
		m: map[discover.NodeID]*Peer{
			discover.NodeID{1}: peer1,
			discover.NodeID{2}: peer2,
			discover.NodeID{3}: peer3,
		},
	}

	peer, score := peerSet.WithBestScore()
	assert.NotNil(t, peer)
	assert.Equal(t, peer2.head.id, peer.head.id)
	assert.Equal(t, peer2.head.totalScore, score)
}
