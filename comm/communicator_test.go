// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"bytes"
	"context"
	"encoding/binary"
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
	"github.com/vechain/thor/v2/p2psrv/rpc"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// TestNewBlockDeliveryDoesNotBlockReadPath pins the regression: a housekeeping
// subscriber that never drains newBlockFeed must not freeze the peer's RPC
// read loop. handleRPC is called directly, standing in for the read loop.
func TestNewBlockDeliveryDoesNotBlockReadPath(t *testing.T) {
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 2, MaxLifetime: time.Minute}, &thor.SoloFork)
	defer pool.Close()

	c := New(chain.Repo(), pool)

	// Subscriber that never reads, simulating housekeeping stuck in processBlock.
	events := make(chan *NewBlockEvent)
	sub := c.SubscribeBlock(events)
	defer sub.Unsubscribe()

	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
	txsToSync := &txsToSync{}

	const n = 50
	for i := range n {
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:4], uint32(i))
		raw, err := rlp.EncodeToBytes(new(block.Builder).ParentID(parentID).Build())
		require.NoError(t, err)
		msg := &p2p.Msg{Code: proto.MsgNewBlock, Size: uint32(len(raw)), Payload: bytes.NewReader(raw)}

		callDone := make(chan error, 1)
		go func() {
			callDone <- c.handleRPC(peer, msg, func(any) {}, txsToSync)
		}()

		select {
		case err := <-callDone:
			require.NoError(t, err)
		case <-time.After(time.Second):
			// Don't block the test on a frozen read loop; fail instead of hanging.
			t.Fatalf("handleRPC call %d blocked past the per-call budget; peer read loop would be frozen", i)
		}
	}
}

// TestDeliverBlocksLoopOrdersByNumber checks that deliverBlocksLoop sorts a batch
// taken from the mailbox by block number before forwarding it to subscribers.
func TestDeliverBlocksLoopOrdersByNumber(t *testing.T) {
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 2, MaxLifetime: time.Minute}, &thor.SoloFork)
	defer pool.Close()

	c := New(chain.Repo(), pool)
	events := make(chan *NewBlockEvent, 3)
	sub := c.SubscribeBlock(events)
	defer sub.Unsubscribe()

	// Fill the mailbox before starting the loop so it is taken as one batch.
	peers := []*Peer{testPeer(1), testPeer(2), testPeer(3)}
	numbers := []uint32{30, 10, 20} // deliberately out of order
	for i, p := range peers {
		c.blockMailbox.put(p, blockWithNumber(numbers[i]))
	}

	c.Start()
	defer c.Stop()

	var got []uint32
	for i := range numbers {
		select {
		case ev := <-events:
			got = append(got, ev.Block.Header().Number())
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for delivered block %d", i)
		}
	}
	assert.Equal(t, []uint32{10, 20, 30}, got)
}

// TestStopStopsDeliverBlocksLoop checks that Stop terminates deliverBlocksLoop so
// goes.Wait returns, instead of hanging forever.
func TestStopStopsDeliverBlocksLoop(t *testing.T) {
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 2, MaxLifetime: time.Minute}, &thor.SoloFork)
	defer pool.Close()

	c := New(chain.Repo(), pool)
	c.Start()

	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s; deliverBlocksLoop may not have exited")
	}
}

// bufferedRW hands payloads to the reader as *bytes.Reader, as the devp2p
// transport does. rpc.Serve relies on it: rlp.Stream would otherwise buffer
// past the call header and starve the handler's decode.
type bufferedRW struct{ *p2p.MsgPipeRW }

func (rw bufferedRW) ReadMsg() (p2p.Msg, error) {
	msg, err := rw.MsgPipeRW.ReadMsg()
	if err != nil {
		return msg, err
	}
	data, err := io.ReadAll(msg.Payload)
	if err != nil {
		return msg, err
	}
	msg.Payload = bytes.NewReader(data)
	return msg, nil
}

// TestOutboundCallNotStalledByBlockPush checks that while a stuck consumer
// holds a block pushed by a peer, our own calls to that peer still get answered,
// since their results arrive on the same read loop.
func TestOutboundCallNotStalledByBlockPush(t *testing.T) {
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 2, MaxLifetime: time.Minute}, &thor.SoloFork)
	defer pool.Close()

	c := New(chain.Repo(), pool)
	events := make(chan *NewBlockEvent) // never drained
	sub := c.SubscribeBlock(events)
	defer sub.Unsubscribe()

	c.Start()
	defer c.Stop()

	localEnd, remoteEnd := p2p.MsgPipe()
	defer localEnd.Close()

	local := newPeer(p2p.NewPeer(discover.NodeID{1}, "remote", nil), bufferedRW{localEnd})
	var txsToSync txsToSync
	go local.Serve(func(msg *p2p.Msg, w func(any)) error {
		return c.handleRPC(local, msg, w, &txsToSync)
	}, proto.MaxMsgSize)

	remote := rpc.New(p2p.NewPeer(discover.NodeID{2}, "local", nil), bufferedRW{remoteEnd})
	go remote.Serve(func(msg *p2p.Msg, w func(any)) error {
		if msg.Code == proto.MsgGetStatus {
			w(&proto.Status{})
		}
		return nil
	}, proto.MaxMsgSize)

	// Two pushes: the first occupies the consumer, the second must still be read.
	pushed := make(chan error, 1)
	go func() {
		for num := uint32(1); num <= 2; num++ {
			if err := proto.NotifyNewBlock(context.Background(), remote, blockWithNumber(num)); err != nil {
				pushed <- err
				return
			}
		}
		pushed <- nil
	}()
	select {
	case err := <-pushed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("block push not consumed; peer read loop is frozen")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = proto.GetStatus(ctx, local)
	require.NoError(t, err, "outbound call to the pushing peer was not answered")
}
