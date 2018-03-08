package session

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/co"
)

type Session struct {
	peer       *p2p.Peer
	nextReqSeq uint32
	reqCh      chan *request
	closed     chan interface{}
}

func New(peer *p2p.Peer) *Session {
	return &Session{
		peer,
		0,
		make(chan *request),
		nil,
	}
}

func (s *Session) Serve(ws p2p.MsgReadWriter) (rerr error) {
	s.closed = make(chan interface{})
	defer func() { close(s.closed) }()

	msgCh := make(chan *p2p.Msg)
	var runner co.Runner

	runner.Go(func() {
		pendings := make(map[uint32]*request)
		// tx loop
		for {
			select {
			case req := <-s.reqCh:
				seq := req.code.Sequence()
				if _, loaded := pendings[seq]; loaded {
					req.respCh <- &response{err: errors.New("duplicated sequence number")}
					continue
				}

				pendings[seq] = req
				if err := p2p.Send(ws, req.code.Uint64(), req.data); err != nil {
					delete(pendings, seq)
					req.respCh <- &response{err: err}
					continue
				}
			case msg := <-msgCh:
				if msg == nil {
					return
				}
				msgCode := MsgCode(msg.Code)
				seq := msgCode.Sequence()
				req, loaded := pendings[seq]
				if !loaded {
					msg.Discard()
					continue
				}
				if msgCode.Code() != req.code.Code() || !msgCode.IsResponse() {
					msg.Discard()
					continue
				}
				delete(pendings, seq)
				req.respCh <- &response{msg: msg}
			}
		}
	})
	runner.Go(func() {
		defer close(msgCh)
		// rx loop
		for {
			msg, err := ws.ReadMsg()
			if err != nil {
				rerr = err
				return
			}
			msgCh <- &msg
		}
	})
	runner.Wait()
	return
}

type Request struct {
	code uint8
	data interface{}
}

func NewRequest(code uint8, data interface{}) *Request {
	return &Request{code, data}
}

func (r *Request) Do(ctx context.Context, session *Session, respData interface{}) error {
	if session.closed == nil {
		return errors.New("session not started")
	}

	req := &request{
		make(chan *response, 1),
		NewMsgCode(r.code, atomic.AddUint32(&session.nextReqSeq, 1), false),
		r.data,
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	select {
	case session.reqCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-session.closed:
		return errors.New("session closed")
	}

	select {
	case resp := <-req.respCh:
		if resp.err != nil {
			return resp.err
		}
		return resp.msg.Decode(respData)
	case <-ctx.Done():
		return ctx.Err()
	case <-session.closed:
		return errors.New("session closed")
	}
}

type request struct {
	respCh chan *response
	code   MsgCode
	data   interface{}
}

type response struct {
	err error
	msg *p2p.Msg
}
