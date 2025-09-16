// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

// stubMsgReadWriter is a no-op MsgReadWriter
type stubMsgReadWriter struct{}

func (stubMsgReadWriter) ReadMsg() (p2p.Msg, error) { return p2p.Msg{}, nil }
func (stubMsgReadWriter) WriteMsg(p2p.Msg) error    { return nil }

func TestNewPeerAndHead(t *testing.T) {
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
	assert.NotNil(t, peer)
	id, score := peer.Head()
	assert.Equal(t, thor.Bytes32{}, id)
	assert.Equal(t, uint64(0), score)
}

func TestUpdateHead(t *testing.T) {
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
	id := thor.Bytes32{1, 2, 3}
	peer.UpdateHead(id, 10)
	gotID, gotScore := peer.Head()
	assert.Equal(t, id, gotID)
	assert.Equal(t, uint64(10), gotScore)
	// Should not update if lower score
	peer.UpdateHead(thor.Bytes32{9, 9, 9}, 5)
	gotID, gotScore = peer.Head()
	assert.Equal(t, id, gotID)
	assert.Equal(t, uint64(10), gotScore)
}

func TestMarkTransactionAndIsTransactionKnown(t *testing.T) {
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
	hash := thor.Bytes32{1, 2, 3}
	peer.MarkTransaction(hash)
	// Should be known immediately after marking
	assert.True(t, peer.IsTransactionKnown(hash))
	// Simulate expiration by setting deadline in the past
	peer.knownTxs.Add(hash, mclock.Now()-100)
	assert.False(t, peer.IsTransactionKnown(hash))
}

func TestMarkBlockAndIsBlockKnown(t *testing.T) {
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
	id := thor.Bytes32{4, 5, 6}
	peer.MarkBlock(id)
	assert.True(t, peer.IsBlockKnown(id))
	// Remove from cache and check
	peer.knownBlocks.Remove(id)
	assert.False(t, peer.IsBlockKnown(id))
}

func TestDuration(t *testing.T) {
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
	// Simulate some time passing
	peer.createdTime = mclock.Now() - 100
	assert.GreaterOrEqual(t, peer.Duration(), mclock.AbsTime(100))
}

func TestPeersFilterAndFind(t *testing.T) {
	peer1 := newPeer(p2p.NewPeer(discover.NodeID{}, "test1", nil), stubMsgReadWriter{})
	peer2 := newPeer(p2p.NewPeer(discover.NodeID{}, "test2", nil), stubMsgReadWriter{})
	peers := Peers{peer1, peer2}
	filtered := peers.Filter(func(p *Peer) bool { return p == peer1 })
	assert.Equal(t, Peers{peer1}, filtered)
	found := peers.Find(func(p *Peer) bool { return p == peer2 })
	assert.Equal(t, peer2, found)
}

func TestPeerSetAddFindRemoveSliceLen(t *testing.T) {
	ps := newPeerSet()
	peer := newPeer(p2p.NewPeer(discover.NodeID{1}, "test", nil), stubMsgReadWriter{})
	ps.Add(peer)
	assert.Equal(t, 1, ps.Len())
	found := ps.Find(peer.ID())
	assert.Equal(t, peer, found)
	removed := ps.Remove(peer.ID())
	assert.Equal(t, peer, removed)
	assert.Equal(t, 0, ps.Len())
	// Add multiple and test Slice
	peer2 := newPeer(p2p.NewPeer(discover.NodeID{2}, "test2", nil), stubMsgReadWriter{})
	peer3 := newPeer(p2p.NewPeer(discover.NodeID{3}, "test3", nil), stubMsgReadWriter{})
	ps.Add(peer2)
	ps.Add(peer3)
	slice := ps.Slice()
	assert.Len(t, slice, 2)
}
