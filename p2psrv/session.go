package p2psrv

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
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
type HandleRequest func(session *Session, msg *p2p.Msg) (resp interface{}, err error)

// Session p2p session which conforms request-response manner.
type Session struct {
	peer    *p2p.Peer
	proto   *Protocol
	opCh    chan interface{}
	opAckCh chan struct{}
	doneCh  chan struct{}

	stats sessionStats
}

func newSession(peer *p2p.Peer, proto *Protocol) *Session {
	return &Session{
		peer:    peer,
		proto:   proto,
		opCh:    make(chan interface{}),
		opAckCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Protocol returns protocol.
func (s *Session) Protocol() *Protocol {
	return s.proto
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
	startTime := mclock.Now()
	defer func() {
		s.stats.duration = time.Duration((mclock.Now() - startTime))
	}()

	var goes co.Goes
	defer goes.Wait()
	defer close(s.doneCh)

	goes.Go(func() { s.opLoop(rw, handleRequest) })

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

	if msg.Size > s.proto.MaxMsgSize {
		return errors.New("msg too large")
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
		s.opCh <- &remoteResponse{reqID, &msg}
	} else {
		s.opCh <- &remoteRequest{reqID, &msg}
	}
	<-s.opAckCh
	return nil
}

func (s *Session) opLoop(rw p2p.MsgReadWriter, handleRequest HandleRequest) {
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
			s.stats.nRequest++
			resp, err := handleRequest(s, val.msg)
			if err != nil {
				s.stats.nBadRequest++
				// TODO log
				break
			}
			if err := p2p.Send(rw, val.msg.Code, &msgData{val.id, true, resp}); err != nil {
				// TODO log
				break
			}
		case *remoteResponse:
			s.stats.nResponse++
			req, ok := pendingReqs[val.id]
			if !ok {
				break
			}
			if val.msg.Code != req.msgCode {
				s.stats.nBadResponse++
				break
			}
			delete(pendingReqs, val.id)
			if req.handleResponse(val.msg) != nil {
				s.stats.nBadResponse++
			}
		}
	}

	// op loop
	for {
		select {
		case <-s.doneCh:
			return
		case val := <-s.opCh:
			process(val)
			s.opAckCh <- struct{}{}
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
	case <-s.doneCh:
		return errSessionClosed
	case <-ctx.Done():
		return ctx.Err()
	case s.opCh <- &req:
		<-s.opAckCh
		if req.err != nil {
			return req.err
		}
		reqID = req.id
	}

	//
	defer func() {
		select {
		case <-s.doneCh:
		case s.opCh <- &endRequest{reqID}:
			<-s.opAckCh
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

type sessionStats struct {
	nRequest     int
	nBadRequest  int
	nResponse    int
	nBadResponse int
	duration     time.Duration
}

func (ss *sessionStats) weight() float64 {
	n := ss.nRequest - ss.nBadRequest + ss.nResponse - ss.nBadResponse
	return float64(ss.duration/time.Minute) + float64(n)
}
