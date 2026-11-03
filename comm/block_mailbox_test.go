// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/p2p"
	"github.com/vechain/thor/v2/p2p/discover"
	"github.com/vechain/thor/v2/thor"
)

// blockWithNumber returns a block whose Header().Number() == num.
func blockWithNumber(num uint32) *block.Block {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:4], num-1)
	return new(block.Builder).ParentID(parentID).Build()
}

// testPeer returns a distinct *Peer usable as a blockMailbox map key.
func testPeer(id byte) *Peer {
	return newPeer(p2p.NewPeer(discover.NodeID{id}, "test", nil), stubMsgReadWriter{})
}

func TestBlockMailbox(t *testing.T) {
	t.Run("put overwrites and keeps the latest block", func(t *testing.T) {
		m := newBlockMailbox()
		p := testPeer(1)
		b1, b2 := blockWithNumber(1), blockWithNumber(2)

		assert.False(t, m.put(p, b1), "first put for a peer is not an overwrite")
		assert.True(t, m.put(p, b2), "second put for the same peer overwrites")

		batch := m.take()
		require.Len(t, batch, 1)
		assert.Equal(t, b2.Header().ID(), batch[p].Header().ID())
	})

	t.Run("each of several peers keeps only its last block", func(t *testing.T) {
		m := newBlockMailbox()
		peers := []*Peer{testPeer(1), testPeer(2), testPeer(3)}

		for _, p := range peers {
			for n := uint32(1); n <= 5; n++ {
				m.put(p, blockWithNumber(n))
			}
		}

		batch := m.take()
		require.Len(t, batch, len(peers))
		for _, p := range peers {
			blk, ok := batch[p]
			require.True(t, ok, "peer should have a pending block")
			assert.Equal(t, uint32(5), blk.Header().Number(), "only the last block per peer survives")
		}
	})

	t.Run("100 puts coalesce into at most one pending signal", func(t *testing.T) {
		m := newBlockMailbox()
		p := testPeer(1)
		for n := uint32(1); n <= 100; n++ {
			m.put(p, blockWithNumber(n))
		}
		assert.Len(t, m.signal, 1)
	})

	t.Run("remove excludes the peer from the next take", func(t *testing.T) {
		m := newBlockMailbox()
		p1, p2 := testPeer(1), testPeer(2)
		m.put(p1, blockWithNumber(1))
		m.put(p2, blockWithNumber(2))

		m.remove(p1)

		batch := m.take()
		_, ok := batch[p1]
		assert.False(t, ok)
		_, ok = batch[p2]
		assert.True(t, ok)
	})

	t.Run("put after take re-signals", func(t *testing.T) {
		m := newBlockMailbox()
		p := testPeer(1)

		m.put(p, blockWithNumber(1))
		<-m.signal // consumer drains signal before taking, per deliverBlocksLoop
		m.take()
		assert.Len(t, m.signal, 0)

		m.put(p, blockWithNumber(2))
		assert.Len(t, m.signal, 1)
	})
}
