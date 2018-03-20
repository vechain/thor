package p2psrv

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/co"
)

func init() {
	// required when generate request id
	rand.Seed(time.Now().UnixNano())
}

var (
	errPeerDisconnected = errors.New("peer disconnected")
	errMsgTooLarge      = errors.New("msg too large")
)

// HandleRequest handles incoming request message, acts like a server.
type HandleRequest func(peer *Peer, msg *p2p.Msg) (resp interface{}, err error)

// Peer p2p peer which conforms request-response manner.
type Peer struct {
	peer    *p2p.Peer
	proto   *Protocol
	opCh    chan interface{}
	opAckCh chan struct{}
	doneCh  chan struct{}
	stats   peerStats
}

func newPeer(peer *p2p.Peer, proto *Protocol) *Peer {
	return &Peer{
		peer:    peer,
		proto:   proto,
		opCh:    make(chan interface{}),
		opAckCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Protocol returns protocol.
func (p *Peer) Protocol() *Protocol {
	return p.proto
}

func (p *Peer) String() string {
	id := p.peer.ID()
	return fmt.Sprintf("%x %v", id[:8], p.peer.RemoteAddr())
}

// RemoteAddr returns the remote address of the network connection.
func (p *Peer) RemoteAddr() net.Addr {
	return p.peer.RemoteAddr()
}

// ID returns peer node id.
func (p *Peer) ID() discover.NodeID {
	return p.peer.ID()
}

// Inbound returns if the peer is incoming connection.
func (p *Peer) Inbound() bool {
	return p.peer.Inbound()
}

// Disconnect disconnect from remote peer.
func (p *Peer) Disconnect(markAsBadPeer bool) {
	p.stats.badPeer = markAsBadPeer
	p.peer.Disconnect(p2p.DiscSelf)

}

// Done returns the done channel that indicates disconnection.
func (p *Peer) Done() <-chan struct{} {
	return p.doneCh
}

// serve handles p2p message.
func (p *Peer) serve(rw p2p.MsgReadWriter, handleRequest HandleRequest) error {
	startTime := mclock.Now()
	defer func() {
		p.stats.duration = time.Duration((mclock.Now() - startTime))
	}()

	var goes co.Goes
	defer goes.Wait()
	defer close(p.doneCh)

	goes.Go(func() { p.opLoop(rw, handleRequest) })

	// msg read loop
	for {
		if err := p.handleMsg(rw); err != nil {
			return err
		}
	}
}

func (p *Peer) handleMsg(rw p2p.MsgReadWriter) error {
	msg, err := rw.ReadMsg()
	if err != nil {
		return err
	}
	// ensure msg.Payload consumed
	defer msg.Discard()

	if msg.Size > p.proto.MaxMsgSize {
		return errMsgTooLarge
	}

	// parse firt two elements, which are reqID and isResponse
	stream := rlp.NewStream(msg.Payload, uint64(msg.Size))
	if _, err := stream.List(); err != nil {
		return err
	}
	var (
		reqID      uint32
		isResponse bool
	)
	if err := stream.Decode(&reqID); err != nil {
		return err
	}
	if err := stream.Decode(&isResponse); err != nil {
		return err
	}
	if isResponse {
		p.opCh <- &remoteResponse{reqID, &msg}
	} else {
		p.opCh <- &remoteRequest{reqID, &msg}
	}
	<-p.opAckCh
	return nil
}

func (p *Peer) opLoop(rw p2p.MsgReadWriter, handleRequest HandleRequest) {
	pendingReqs := make(map[uint32]*localRequest)

	genID := func() uint32 {
		for {
			id := rand.Uint32()
			if _, ok := pendingReqs[id]; !ok {
				return id
			}
		}
	}
	process := func(val interface{}) {
		switch val := val.(type) {
		case *localRequest:
			id := genID()
			if err := p2p.Send(rw, val.msgCode, &msgData{id, false, val.payload}); err != nil {
				// TODO log
				val.err = err
				break
			}
			pendingReqs[id] = val
			val.id = id
		case *endRequest:
			delete(pendingReqs, val.id)
		case *remoteRequest:
			p.stats.nRequest++
			resp, err := handleRequest(p, val.msg)
			if err != nil {
				p.stats.nBadRequest++
				// TODO log
				break
			}
			if err := p2p.Send(rw, val.msg.Code, &msgData{val.id, true, resp}); err != nil {
				// TODO log
				break
			}
		case *remoteResponse:
			p.stats.nResponse++
			req, ok := pendingReqs[val.id]
			if !ok {
				break
			}
			if val.msg.Code != req.msgCode {
				p.stats.nBadResponse++
				break
			}
			delete(pendingReqs, val.id)
			if req.handleResponse(val.msg) != nil {
				p.stats.nBadResponse++
			}
		}
	}

	// op loop
	for {
		select {
		case <-p.doneCh:
			return
		case val := <-p.opCh:
			process(val)
			p.opAckCh <- struct{}{}
		}
	}
}

// Request send request to remote peer and wait for response.
// reqPayload must be rlp encodable
// respPayload must be rlp decodable
func (p *Peer) Request(ctx context.Context, msgCode uint64, reqPayload interface{}, respPayload interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	respCh := make(chan error, 1)
	req := localRequest{
		msgCode: msgCode,
		payload: reqPayload,
		handleResponse: func(msg *p2p.Msg) error {
			// should consume msg here, or msg will be discarded
			err := msg.Decode(respPayload)
			respCh <- err
			return err
		},
	}
	var reqID uint32
	// send request
	select {
	case <-p.doneCh:
		return errPeerDisconnected
	case <-ctx.Done():
		return ctx.Err()
	case p.opCh <- &req:
		<-p.opAckCh
		if req.err != nil {
			return req.err
		}
		reqID = req.id
	}

	//
	defer func() {
		select {
		case <-p.doneCh:
		case p.opCh <- &endRequest{reqID}:
			<-p.opAckCh
		}
	}()

	// wait for response
	select {
	case <-p.doneCh:
		return errPeerDisconnected
	case <-ctx.Done():
		return ctx.Err()
	case err := <-respCh:
		return err
	}
}

type msgData struct {
	ID         uint32
	IsResponse bool
	Payload    interface{}
}

type localRequest struct {
	msgCode        uint64
	payload        interface{}
	handleResponse func(*p2p.Msg) error

	id  uint32
	err error
}

type remoteResponse struct {
	id  uint32
	msg *p2p.Msg
}

type remoteRequest remoteResponse

type endRequest struct{ id uint32 }

type peerStats struct {
	nRequest     int
	nBadRequest  int
	nResponse    int
	nBadResponse int
	duration     time.Duration
	badPeer      bool
}

func (ps *peerStats) weight() float64 {
	if ps.badPeer {
		return -1
	}
	n := ps.nRequest - ps.nBadRequest + ps.nResponse - ps.nBadResponse
	return float64(ps.duration/time.Minute) + float64(n)
}
