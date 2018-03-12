package p2psrv

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/co"
)

func init() {
	// required when generate request id
	rand.Seed(time.Now().UnixNano())
}

var (
	errSessionClosed = errors.New("session closed")
)

// HandleRequest handles incoming request message, acts like a server.
// The returned resp can be nil if don't want to response to incoming request.
type HandleRequest func(session *Session, msg *p2p.Msg) (resp interface{})

// Session p2p session which conforms request-response manner.
type Session struct {
	peer            *p2p.Peer
	protoVer        uint32
	localReqCh      chan *localRequest
	localReqAckCh   chan *localRequestAck
	localReqDoneCh  chan uint32
	remoteRespCh    chan *remoteResponse
	remoteRespAckCh chan struct{}
	remoteReqCh     chan *remoteRequest
	remoteReqAckCh  chan struct{}
	doneCh          chan struct{}
}

func newSession(peer *p2p.Peer, protoVer uint32) *Session {
	return &Session{
		peer,
		protoVer,
		make(chan *localRequest),
		make(chan *localRequestAck),
		make(chan uint32),
		make(chan *remoteResponse),
		make(chan struct{}),
		make(chan *remoteRequest),
		make(chan struct{}),
		make(chan struct{}),
	}
}

// ProtocolVersion returns protocol version.
func (s *Session) ProtocolVersion() uint32 {
	return s.protoVer
}

// Peer returns remote peer of this session.
func (s *Session) Peer() *p2p.Peer {
	return s.peer
}

// Alive returns whether session is alive.
func (s *Session) Alive() bool {
	select {
	case <-s.doneCh:
		return false
	default:
		return true
	}
}

// serve handles p2p message.
func (s *Session) serve(rw p2p.MsgReadWriter, handleRequest HandleRequest) error {

	var runner co.Runner
	defer runner.Wait()
	defer close(s.doneCh)

	runner.Go(func() { s.rrLoop(rw, handleRequest) })

	// msg read loop
	for {
		if err := s.handleMsg(rw); err != nil {
			return err
		}
	}
}

func (s *Session) handleMsg(rw p2p.MsgReadWriter) error {
	msg, err := rw.ReadMsg()
	if err != nil {
		return err
	}
	// ensure msg.Payload consumed
	defer msg.Discard()

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
		s.remoteRespCh <- &remoteResponse{reqID, &msg}
		<-s.remoteRespAckCh
	} else {
		s.remoteReqCh <- &remoteRequest{reqID, &msg}
		<-s.remoteReqAckCh
	}
	return nil
}

func (s *Session) rrLoop(rw p2p.MsgReadWriter, handleRequest HandleRequest) {
	pendingReqs := make(map[uint32]*localRequest)
	// process loop
	for {
		select {
		case <-s.doneCh:
			return
		case resp := <-s.remoteRespCh:
			req, ok := pendingReqs[resp.id]
			if !ok {
				s.remoteRespAckCh <- struct{}{}
				continue
			}
			if resp.msg.Code != req.msgCode {
				s.remoteRespAckCh <- struct{}{}
				continue
			}
			delete(pendingReqs, resp.id)
			req.onResponse(resp.msg)
			s.remoteRespAckCh <- struct{}{}
		case req := <-s.localReqCh:
			id := rand.Uint32()
			for {
				if _, ok := pendingReqs[id]; !ok {
					break
				}
			}
			if err := p2p.Send(rw, req.msgCode, &msgData{id, false, req.payload}); err != nil {
				// TODO log
				s.localReqAckCh <- &localRequestAck{0, err}
				continue
			}
			pendingReqs[id] = req
			s.localReqAckCh <- &localRequestAck{id, nil}
		case reqID := <-s.localReqDoneCh:
			delete(pendingReqs, reqID)
		case req := <-s.remoteReqCh:
			resp := handleRequest(s, req.msg)
			if resp != nil {
				if err := p2p.Send(rw, req.msg.Code, &msgData{req.id, true, resp}); err != nil {
					// TODO log
					s.remoteReqAckCh <- struct{}{}
					continue
				}
			}
			s.remoteReqAckCh <- struct{}{}
		}
	}
}

// Request send request to remote peer and wait for response.
// reqPayload must be rlp encodable
// respPayload must be rlp decodable
func (s *Session) Request(ctx context.Context, msgCode uint64, reqPayload interface{}, respPayload interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	respCh := make(chan error, 1)
	req := localRequest{
		msgCode,
		reqPayload,
		func(msg *p2p.Msg) {
			// should consume msg here, or msg will be discarded
			respCh <- msg.Decode(respPayload)
		},
	}
	var reqID uint32
	// send request
	select {
	case <-s.doneCh:
		return errSessionClosed
	case <-ctx.Done():
		return ctx.Err()
	case s.localReqCh <- &req:
		ack := <-s.localReqAckCh
		if ack.err != nil {
			return ack.err
		}
		reqID = ack.id
	}

	//
	defer func() {
		select {
		case <-s.doneCh:
		case s.localReqDoneCh <- reqID:
		}
	}()

	// wait for response
	select {
	case <-s.doneCh:
		return errSessionClosed
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
	msgCode    uint64
	payload    interface{}
	onResponse func(*p2p.Msg)
}

type localRequestAck struct {
	id  uint32
	err error
}

type remoteResponse struct {
	id  uint32
	msg *p2p.Msg
}

type remoteRequest remoteResponse
