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
	peer        *p2p.Peer
	msgRW       p2p.MsgReadWriter
	protoVer    uint32
	pendingReqs map[uint32]*request
	opCh        chan func() error
	opDoneCh    chan error
	doneCh      chan interface{}
}

func newSession(peer *p2p.Peer, msgRW p2p.MsgReadWriter, protoVer uint32) *Session {
	return &Session{
		peer,
		msgRW,
		protoVer,
		make(map[uint32]*request),
		make(chan func() error),
		make(chan error),
		make(chan interface{}),
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
func (s *Session) serve(handleRequest HandleRequest) error {
	var runner co.Runner
	defer runner.Wait()
	defer func() { close(s.doneCh) }()

	runner.Go(s.opLoop)

	// read msg loop
	for {
		msg, err := s.msgRW.ReadMsg()

		if err != nil {
			return err
		}
		defer msg.Discard()

		var (
			reqID      uint32
			isResponse bool
		)

		// parse firt two elements, which are reqID and isResponse
		stream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := stream.List(); err != nil {
			return err
		}
		if err := stream.Decode(&reqID); err != nil {
			return err
		}
		if err := stream.Decode(&isResponse); err != nil {
			return err
		}

		if isResponse {
			s.handleResponse(reqID, &msg)
		} else {
			resp := handleRequest(s, &msg)
			if resp != nil {
				if err := p2p.Send(s.msgRW, msg.Code, &msgData{reqID, true, resp}); err != nil {
					return err
				}
			}
		}
	}
}

func (s *Session) opLoop() {
	for {
		select {
		case <-s.doneCh:
			return
		case op := <-s.opCh:
			s.opDoneCh <- op()
		}
	}
}

func (s *Session) handleResponse(reqID uint32, msg *p2p.Msg) {
	select {
	case s.opCh <- func() error {
		req, ok := s.pendingReqs[reqID]
		if !ok {
			return nil
		}
		if msg.Code != req.msgCode {
			return nil
		}
		delete(s.pendingReqs, reqID)
		req.onResponse(msg)
		return nil
	}:
		<-s.opDoneCh
	case <-s.doneCh:
	}
}

// Request send request to remote peer and wait for response.
func (s *Session) Request(ctx context.Context, msgCode uint64, reqPayload interface{}, respPayload interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var id uint32
	respCh := make(chan error)

	// send request
	select {
	case <-s.doneCh:
		return errSessionClosed
	case <-ctx.Done():
		return ctx.Err()
	case s.opCh <- func() error {
		id = rand.Uint32()
		for {
			if _, ok := s.pendingReqs[id]; !ok {
				break
			}
		}
		if err := p2p.Send(s.msgRW, msgCode, &msgData{id, false, reqPayload}); err != nil {
			return err
		}
		s.pendingReqs[id] = &request{
			msgCode,
			reqPayload,
			func(msg *p2p.Msg) {
				respCh <- msg.Decode(respPayload)
			},
		}
		return nil
	}:
		if err := <-s.opDoneCh; err != nil {
			return err
		}
	}
	defer func() {
		s.syncRequest(id)
	}()

	// wait for response
	select {
	case err := <-respCh:
		return err
	case <-s.doneCh:
		return errSessionClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) syncRequest(id uint32) {
	select {
	case <-s.doneCh:
	case s.opCh <- func() error {
		delete(s.pendingReqs, id)
		return nil
	}:
		<-s.opDoneCh
	}
}

type msgData struct {
	ID         uint32
	IsResponse bool
	Payload    interface{}
}

type request struct {
	msgCode    uint64
	payload    interface{}
	onResponse func(*p2p.Msg)
}
