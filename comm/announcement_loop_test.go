// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/comm/proto"
	"github.com/vechain/thor/v2/p2p"
	"github.com/vechain/thor/v2/p2p/discover"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// replyRW answers every outgoing call with a canned result payload.
type replyRW struct {
	read   chan p2p.Msg
	result rlp.RawValue // the block RLP the peer serves
}

func (rw *replyRW) ReadMsg() (p2p.Msg, error) {
	msg, ok := <-rw.read
	if !ok {
		return p2p.Msg{}, io.EOF
	}
	return msg, nil
}

func (rw *replyRW) WriteMsg(msg p2p.Msg) error {
	s := rlp.NewStream(msg.Payload, uint64(msg.Size))
	if _, err := s.List(); err != nil {
		return err
	}
	var (
		callID   uint32
		isResult bool
	)
	if err := s.Decode(&callID); err != nil {
		return err
	}
	if err := s.Decode(&isResult); err != nil {
		return err
	}
	enc, err := rlp.EncodeToBytes([]any{callID, true, []rlp.RawValue{rw.result}})
	if err != nil {
		return err
	}
	rw.read <- p2p.Msg{Code: msg.Code, Size: uint32(len(enc)), Payload: bytes.NewReader(enc)}
	return nil
}

func fetchBlockByIDResult(t *testing.T, served rlp.RawValue, want thor.Bytes32) *block.Block {
	t.Helper()
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 2, MaxLifetime: time.Minute}, &thor.SoloFork)
	defer pool.Close()

	c := New(chain.Repo(), pool)
	rw := &replyRW{read: make(chan p2p.Msg, 1), result: served}
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), rw)
	go peer.Serve(func(*p2p.Msg, func(any)) error { return nil }, proto.MaxMsgSize)

	events := make(chan *NewBlockEvent, 1)
	sub := c.SubscribeBlock(events)
	defer sub.Unsubscribe()

	c.fetchBlockByID(peer, want)

	select {
	case ev := <-events:
		return ev.Block
	default:
		return nil
	}
}

func TestFetchBlockByID(t *testing.T) {
	blk := new(block.Builder).ParentID(thor.Bytes32{1}).Build()
	raw, err := rlp.EncodeToBytes(blk)
	require.NoError(t, err)

	t.Run("matching ID is forwarded", func(t *testing.T) {
		got := fetchBlockByIDResult(t, raw, blk.Header().ID())
		require.NotNil(t, got)
		assert.Equal(t, blk.Header().ID(), got.Header().ID())
		assert.EqualValues(t, len(raw), got.Size())
	})

	t.Run("mismatched ID is dropped", func(t *testing.T) {
		assert.Nil(t, fetchBlockByIDResult(t, raw, thor.Bytes32{9, 9, 9}))
	})

	t.Run("undecodable block is dropped", func(t *testing.T) {
		assert.Nil(t, fetchBlockByIDResult(t, rlp.RawValue{0x83, 0x01, 0x02, 0x03}, blk.Header().ID()))
	})
}
